// Package update provides self-update detection and installation method logic.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
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

	// Build checksums.txt content.
	checksumsTxt := fmt.Sprintf("%s  %s\n", archiveChecksum, archiveName)

	// Start mock HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
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

	dir := t.TempDir()
	oldBinary := filepath.Join(dir, "contextception")
	if err := os.WriteFile(oldBinary, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	archiveData := createTestTarGz(t, "contextception", []byte("new binary"))
	archiveName := archiveNameForPlatform()
	wrongChecksum := strings.Repeat("a", 64)
	checksumsTxt := fmt.Sprintf("%s  %s\n", wrongChecksum, archiveName)

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

	err := SelfUpdate(oldBinary, "v1.0.5", srv.URL+"/{tag}/{name}")
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
