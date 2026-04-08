// Package update provides self-update detection and installation method logic.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectInstallMethod(t *testing.T) {
	tests := []struct {
		path    string
		want    InstallMethod
		homeDir string // if set, overrides HOME and clears GOPATH/GOBIN
	}{
		{path: "/opt/homebrew/bin/contextception", want: Homebrew},
		{path: "/usr/local/Cellar/contextception/1.0.0/bin/contextception", want: Homebrew},
		{path: "/home/linuxbrew/.linuxbrew/bin/contextception", want: Homebrew},
		// /home/user/go/bin is the default GOPATH/bin when HOME=/home/user
		{path: "/home/user/go/bin/contextception", want: GoInstall, homeDir: "/home/user"},
		{path: "/usr/local/bin/contextception", want: DirectDownload},
		{path: `C:\Users\me\bin\contextception.exe`, want: DirectDownload},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if tt.homeDir != "" {
				t.Setenv("HOME", tt.homeDir)
				t.Setenv("GOPATH", "")
				t.Setenv("GOBIN", "")
			}
			got := DetectInstallMethod(tt.path)
			if got != tt.want {
				t.Errorf("DetectInstallMethod(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDetectInstallMethodGopath(t *testing.T) {
	t.Setenv("GOPATH", "/custom/gopath")
	got := DetectInstallMethod("/custom/gopath/bin/contextception")
	if got != GoInstall {
		t.Errorf("DetectInstallMethod with custom GOPATH = %v, want GoInstall", got)
	}
}

func TestDetectInstallMethodGobin(t *testing.T) {
	t.Setenv("GOBIN", "/custom/gobin")
	got := DetectInstallMethod("/custom/gobin/contextception")
	if got != GoInstall {
		t.Errorf("DetectInstallMethod with GOBIN = %v, want GoInstall", got)
	}
}

func TestInstallMethodString(t *testing.T) {
	tests := []struct {
		method InstallMethod
		want   string
	}{
		{Homebrew, "homebrew"},
		{GoInstall, "go-install"},
		{DirectDownload, "direct"},
	}
	for _, tt := range tests {
		if got := tt.method.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.method, got, tt.want)
		}
	}
}

func TestCurrentBinaryPath(t *testing.T) {
	path, err := CurrentBinaryPath()
	if err != nil {
		t.Fatalf("CurrentBinaryPath() error = %v", err)
	}
	if path == "" {
		t.Error("CurrentBinaryPath() returned empty string")
	}
}

// createTestTarGz creates an in-memory tar.gz archive containing a single file
// with the given name and content.
func createTestTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return buf.Bytes()
}

// TestParseChecksum verifies parsing of sha256sum-format lines.
func TestParseChecksum(t *testing.T) {
	data := []byte("abc123  file1.tar.gz\ndef456  file2.tar.gz\n")

	sum, err := parseChecksum(data, "file1.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksum: %v", err)
	}
	if sum != "abc123" {
		t.Errorf("got %q, want %q", sum, "abc123")
	}

	sum, err = parseChecksum(data, "file2.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksum: %v", err)
	}
	if sum != "def456" {
		t.Errorf("got %q, want %q", sum, "def456")
	}

	_, err = parseChecksum(data, "missing.tar.gz")
	if err == nil {
		t.Error("expected error for missing filename, got nil")
	}
}

// TestExtractFromTarGz verifies that extractFromTarGz finds a file by base name.
func TestExtractFromTarGz(t *testing.T) {
	wantContent := []byte("binary content here")
	archiveData := createTestTarGz(t, "contextception", wantContent)

	got, err := extractFromTarGz(archiveData, "contextception")
	if err != nil {
		t.Fatalf("extractFromTarGz: %v", err)
	}
	if !bytes.Equal(got, wantContent) {
		t.Errorf("content mismatch: got %q, want %q", got, wantContent)
	}

	_, err = extractFromTarGz(archiveData, "notexist")
	if err == nil {
		t.Error("expected error for missing file in archive, got nil")
	}
}

// TestSelfUpdate is a full integration test using a mock HTTP server.
// It skips on Windows because the binary replacement path differs.
func TestSelfUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: binary replacement tested separately")
	}

	// Generate a test key pair and override the package-level public key.
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	origKey := minisignPubKey
	minisignPubKey = formatMinisignPublicKey(pub)
	t.Cleanup(func() { minisignPubKey = origKey })

	// Create fake "old" binary.
	dir := t.TempDir()
	oldBinary := dir + "/contextception"
	if err := os.WriteFile(oldBinary, []byte("old binary content"), 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}

	// Create test archive containing the "new" binary.
	newContent := []byte("new binary content v1.0.5")
	archiveData := createTestTarGz(t, "contextception", newContent)

	// Compute checksum of the archive.
	sum := sha256.Sum256(archiveData)
	archiveChecksum := hex.EncodeToString(sum[:])

	// Determine the archive name that SelfUpdate will request.
	archiveName := archiveNameForPlatform()

	// Build checksums.txt content and sign it.
	checksumsTxt := fmt.Sprintf("%s  %s\n", archiveChecksum, archiveName)
	minisig := signMinisign(t, priv, []byte(checksumsTxt))

	// Start mock HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt.minisig"):
			_, _ = w.Write([]byte(minisig))
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(checksumsTxt))
		case strings.HasSuffix(r.URL.Path, archiveName):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archiveData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Use mock server URL as baseURL pattern.
	baseURL := srv.URL + "/{tag}/{name}"

	if err := SelfUpdate(oldBinary, "v1.0.5", baseURL); err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}

	// Verify the binary now contains the new content.
	got, err := os.ReadFile(oldBinary)
	if err != nil {
		t.Fatalf("read updated binary: %v", err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("updated binary content = %q, want %q", got, newContent)
	}
}

func TestArchiveNameForPlatform(t *testing.T) {
	name := archiveNameForPlatform()
	if !strings.HasPrefix(name, "contextception_") {
		t.Errorf("archiveNameForPlatform() = %q, want prefix contextception_", name)
	}
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(name, ".zip") {
			t.Errorf("archiveNameForPlatform() on windows = %q, want .zip suffix", name)
		}
	} else {
		if !strings.HasSuffix(name, ".tar.gz") {
			t.Errorf("archiveNameForPlatform() = %q, want .tar.gz suffix", name)
		}
	}
	// Should contain OS and arch
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("archiveNameForPlatform() = %q, want to contain GOOS %q", name, runtime.GOOS)
	}
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("archiveNameForPlatform() = %q, want to contain GOARCH %q", name, runtime.GOARCH)
	}
}

// TestSelfUpdateChecksumMismatch verifies that a tampered archive is rejected.
func TestSelfUpdateChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	// Generate a test key pair and override the package-level public key.
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	origKey := minisignPubKey
	minisignPubKey = formatMinisignPublicKey(pub)
	t.Cleanup(func() { minisignPubKey = origKey })

	dir := t.TempDir()
	oldBinary := filepath.Join(dir, "contextception")
	if err := os.WriteFile(oldBinary, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	archiveData := createTestTarGz(t, "contextception", []byte("new binary"))
	archiveName := archiveNameForPlatform()
	wrongChecksum := strings.Repeat("a", 64)
	checksumsTxt := fmt.Sprintf("%s  %s\n", wrongChecksum, archiveName)
	minisig := signMinisign(t, priv, []byte(checksumsTxt))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt.minisig"):
			_, _ = w.Write([]byte(minisig))
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			_, _ = w.Write([]byte(checksumsTxt))
		case strings.HasSuffix(r.URL.Path, archiveName):
			_, _ = w.Write(archiveData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err = SelfUpdate(oldBinary, "v1.0.5", srv.URL+"/{tag}/{name}")
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum verification failed") {
		t.Errorf("expected checksum error, got: %v", err)
	}
}

// TestSelfUpdateInvalidVersion verifies that malformed version strings are rejected.
func TestSelfUpdateInvalidVersion(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "contextception")
	if err := os.WriteFile(binary, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	invalidVersions := []string{"not-a-version", "../../../etc/passwd", ""}
	for _, v := range invalidVersions {
		err := SelfUpdate(binary, v, "http://example.com/{tag}/{name}")
		if err == nil {
			t.Errorf("SelfUpdate(%q) should fail with invalid version", v)
			continue
		}
		if !strings.Contains(err.Error(), "invalid version") {
			t.Errorf("SelfUpdate(%q) error = %v, want 'invalid version'", v, err)
		}
	}
}

// TestReplaceBinary verifies atomic binary replacement and cleanup.
func TestReplaceBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: rename-dance tested separately")
	}

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "binary")
	if err := os.WriteFile(oldPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("new content")
	if err := replaceBinary(oldPath, newContent); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}

	got, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("got %q, want %q", got, newContent)
	}

	// .new file should be cleaned up.
	if _, err := os.Stat(oldPath + ".new"); !os.IsNotExist(err) {
		t.Error(".new file should not exist after successful replacement")
	}
}

// TestReplaceBinaryFailure verifies that replaceBinary fails gracefully for a
// nonexistent parent directory.
func TestReplaceBinaryFailure(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "nonexistent", "binary")
	err := replaceBinary(badPath, []byte("new"))
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

// TestExtractFromZip verifies zip extraction by base name.
func TestExtractFromZip(t *testing.T) {
	wantContent := []byte("zip binary content")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("contextception.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(wantContent); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromZip(buf.Bytes(), "contextception.exe")
	if err != nil {
		t.Fatalf("extractFromZip: %v", err)
	}
	if !bytes.Equal(got, wantContent) {
		t.Errorf("got %q, want %q", got, wantContent)
	}

	_, err = extractFromZip(buf.Bytes(), "notexist")
	if err == nil {
		t.Error("expected error for missing file in zip archive")
	}
}

// --- Symlink rejection tests ---

func TestExtractFromTarGzRejectsSymlink(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     "contextception",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := extractFromTarGz(buf.Bytes(), "contextception")
	if err == nil {
		t.Fatal("expected error for symlink entry, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error = %v, want to contain 'not a regular file'", err)
	}
}

func TestExtractFromZipRejectsNonRegular(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Create a file header with symlink mode bits.
	fh := &zip.FileHeader{Name: "contextception"}
	fh.SetMode(os.ModeSymlink | 0o755)
	w, err := zw.CreateHeader(fh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("/etc/passwd")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = extractFromZip(buf.Bytes(), "contextception")
	if err == nil {
		t.Fatal("expected error for symlink entry, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error = %v, want to contain 'not a regular file'", err)
	}
}

// --- Minisign signature tests ---

// testMinisignKeyID is an arbitrary 8-byte key ID used in test signatures.
var testMinisignKeyID = [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

// formatMinisignPublicKey formats an ed25519 public key as a minisign public
// key string: "untrusted comment: ...\n" + base64(Ed || keyID || pubkey).
func formatMinisignPublicKey(pub ed25519.PublicKey) string {
	// Minisign public key format: 2-byte algorithm ("Ed") + 8-byte key ID + 32-byte public key.
	var raw [42]byte
	raw[0] = 'E'
	raw[1] = 'd'
	copy(raw[2:10], testMinisignKeyID[:])
	copy(raw[10:42], pub)
	return "untrusted comment: minisign public key\n" + base64.StdEncoding.EncodeToString(raw[:])
}

// signMinisign creates a minisign signature string over message using the given
// ed25519 private key.
func signMinisign(t *testing.T, priv ed25519.PrivateKey, message []byte) string {
	t.Helper()

	// Sign the message.
	sig := ed25519.Sign(priv, message)

	// Build the signature line: 2-byte algorithm ("Ed") + 8-byte key ID + 64-byte signature.
	var sigRaw [74]byte
	sigRaw[0] = 'E'
	sigRaw[1] = 'd'
	copy(sigRaw[2:10], testMinisignKeyID[:])
	copy(sigRaw[10:74], sig)

	sigB64 := base64.StdEncoding.EncodeToString(sigRaw[:])

	// Trusted comment.
	trustedComment := "timestamp:1234567890"

	// Global signature: sign(signature_bytes || trusted_comment_bytes).
	var globalMsg []byte
	globalMsg = append(globalMsg, sig...)
	globalMsg = append(globalMsg, []byte(trustedComment)...)
	globalSig := ed25519.Sign(priv, globalMsg)
	globalSigB64 := base64.StdEncoding.EncodeToString(globalSig)

	return fmt.Sprintf("untrusted comment: signature\n%s\ntrusted comment: %s\n%s",
		sigB64, trustedComment, globalSigB64)
}

func TestVerifyChecksums(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Override the package-level public key for this test.
	origKey := minisignPubKey
	minisignPubKey = formatMinisignPublicKey(pub)
	t.Cleanup(func() { minisignPubKey = origKey })

	message := []byte("abc123  contextception_linux_amd64.tar.gz\n")
	sigStr := signMinisign(t, priv, message)

	if err := verifyChecksums(message, []byte(sigStr)); err != nil {
		t.Fatalf("verifyChecksums with valid signature: %v", err)
	}
}

func TestVerifyChecksumsInvalidSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Generate a different key to sign with (wrong key).
	_, wrongPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	origKey := minisignPubKey
	minisignPubKey = formatMinisignPublicKey(pub)
	t.Cleanup(func() { minisignPubKey = origKey })

	message := []byte("abc123  contextception_linux_amd64.tar.gz\n")
	sigStr := signMinisign(t, wrongPriv, message)

	if err := verifyChecksums(message, []byte(sigStr)); err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}

func TestSelfUpdateWithValidSignature(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	origKey := minisignPubKey
	minisignPubKey = formatMinisignPublicKey(pub)
	t.Cleanup(func() { minisignPubKey = origKey })

	dir := t.TempDir()
	oldBinary := filepath.Join(dir, "contextception")
	if err := os.WriteFile(oldBinary, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("new binary with sig")
	archiveData := createTestTarGz(t, "contextception", newContent)
	sum := sha256.Sum256(archiveData)
	archiveName := archiveNameForPlatform()
	checksumsTxt := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName)
	minisig := signMinisign(t, priv, []byte(checksumsTxt))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt.minisig"):
			_, _ = w.Write([]byte(minisig))
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			_, _ = w.Write([]byte(checksumsTxt))
		case strings.HasSuffix(r.URL.Path, archiveName):
			_, _ = w.Write(archiveData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	if err := SelfUpdate(oldBinary, "v1.0.5", srv.URL+"/{tag}/{name}"); err != nil {
		t.Fatalf("SelfUpdate with valid signature: %v", err)
	}

	got, err := os.ReadFile(oldBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("binary = %q, want %q", got, newContent)
	}
}

func TestSelfUpdateWithInvalidSignature(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, wrongPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	origKey := minisignPubKey
	minisignPubKey = formatMinisignPublicKey(pub)
	t.Cleanup(func() { minisignPubKey = origKey })

	dir := t.TempDir()
	oldBinary := filepath.Join(dir, "contextception")
	if err := os.WriteFile(oldBinary, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	archiveData := createTestTarGz(t, "contextception", []byte("new"))
	sum := sha256.Sum256(archiveData)
	archiveName := archiveNameForPlatform()
	checksumsTxt := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName)
	badSig := signMinisign(t, wrongPriv, []byte(checksumsTxt))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt.minisig"):
			_, _ = w.Write([]byte(badSig))
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			_, _ = w.Write([]byte(checksumsTxt))
		case strings.HasSuffix(r.URL.Path, archiveName):
			_, _ = w.Write(archiveData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err = SelfUpdate(oldBinary, "v1.0.5", srv.URL+"/{tag}/{name}")
	if err == nil {
		t.Fatal("expected signature verification error, got nil")
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Errorf("error = %v, want to contain 'signature verification failed'", err)
	}
}

func TestSelfUpdateWithoutSignature(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	dir := t.TempDir()
	oldBinary := filepath.Join(dir, "contextception")
	if err := os.WriteFile(oldBinary, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("new binary no sig")
	archiveData := createTestTarGz(t, "contextception", newContent)
	sum := sha256.Sum256(archiveData)
	archiveName := archiveNameForPlatform()
	checksumsTxt := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName)

	// Server returns 404 for .minisig — simulates unsigned release.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			_, _ = w.Write([]byte(checksumsTxt))
		case strings.HasSuffix(r.URL.Path, archiveName):
			_, _ = w.Write(archiveData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Should fail — missing signature is now a hard error.
	err := SelfUpdate(oldBinary, "v1.0.5", srv.URL+"/{tag}/{name}")
	if err == nil {
		t.Fatal("expected error for missing signature, got nil")
	}
	if !strings.Contains(err.Error(), "release not signed") {
		t.Errorf("error = %v, want to contain 'release not signed'", err)
	}

	// Verify the original binary was NOT replaced.
	got, err := os.ReadFile(oldBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("old")) {
		t.Error("binary should not have been replaced when signature is missing")
	}
}
