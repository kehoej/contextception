// Package update provides self-update detection and installation method logic.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jedisct1/go-minisign"
	"golang.org/x/mod/semver"
)

// minisignPubKey is the minisign public key used to verify release signatures.
// Override in tests. Generate with: minisign -G
var minisignPubKey = `untrusted comment: minisign public key 9610BE1C90F8AD9E
RWSerfiQHL4QlsgaAA47PzkzNCuWhCtXyAbpB3nqAt+0nsBTApWNoXx0`

// InstallMethod describes how the binary was installed.
type InstallMethod int

const (
	// DirectDownload means the binary was downloaded directly (e.g. from GitHub Releases).
	DirectDownload InstallMethod = iota
	// Homebrew means the binary was installed via Homebrew.
	Homebrew
	// GoInstall means the binary was installed via go install.
	GoInstall
)

// String returns the human-readable name of the install method.
func (m InstallMethod) String() string {
	switch m {
	case Homebrew:
		return "homebrew"
	case GoInstall:
		return "go-install"
	default:
		return "direct"
	}
}

// homebrewPrefixes are path prefixes that indicate a Homebrew installation.
var homebrewPrefixes = []string{
	"/opt/homebrew/",
	"/usr/local/Cellar/",
	"/home/linuxbrew/",
}

// DetectInstallMethod returns the InstallMethod for the given binary path.
// Use filepath.ToSlash for cross-platform path comparison.
func DetectInstallMethod(binaryPath string) InstallMethod {
	slashed := filepath.ToSlash(binaryPath)

	// Check Homebrew prefixes.
	for _, prefix := range homebrewPrefixes {
		if strings.HasPrefix(slashed, prefix) {
			return Homebrew
		}
	}

	// Check GOBIN environment variable.
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		gobinSlashed := filepath.ToSlash(gobin)
		if !strings.HasSuffix(gobinSlashed, "/") {
			gobinSlashed += "/"
		}
		if strings.HasPrefix(slashed, gobinSlashed) {
			return GoInstall
		}
	}

	// Check GOPATH/bin.
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Default GOPATH is $HOME/go.
		home, err := os.UserHomeDir()
		if err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath != "" {
		gopathBin := filepath.ToSlash(filepath.Join(gopath, "bin")) + "/"
		if strings.HasPrefix(slashed, gopathBin) {
			return GoInstall
		}
	}

	return DirectDownload
}

// CurrentBinaryPath returns the absolute path to the running executable,
// resolving any symlinks.
func CurrentBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("os.Executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("filepath.EvalSymlinks: %w", err)
	}
	return resolved, nil
}

// archiveNameForPlatform returns the release archive filename for the current
// OS and architecture. Windows uses .zip; all other platforms use .tar.gz.
func archiveNameForPlatform() string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("contextception_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// downloadTimeout is the HTTP timeout for downloading release archives.
const downloadTimeout = 30 * time.Second

// httpGet performs a GET request with a 30-second timeout and returns the body.
func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: downloadTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "contextception-selfupdate")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	// Limit response size to 100MB to prevent OOM from malicious servers.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 100<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

// errNotFound is returned by httpGetOptional when the server returns 404.
var errNotFound = errors.New("not found")

// httpGetOptional is like httpGet but returns errNotFound for 404 responses
// instead of a generic error. This allows callers to distinguish "file does
// not exist" from other failures.
func httpGetOptional(url string) ([]byte, error) {
	client := &http.Client{Timeout: downloadTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "contextception-selfupdate")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

// verifyChecksums verifies the minisign signature over checksumsData.
func verifyChecksums(checksumsData, signatureData []byte) error {
	pk, err := minisign.DecodePublicKey(minisignPubKey)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	sig, err := minisign.DecodeSignature(string(signatureData))
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}
	valid, err := pk.Verify(checksumsData, sig)
	if err != nil {
		return fmt.Errorf("signature verification: %w", err)
	}
	if !valid {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// parseChecksum parses sha256sum-format data (lines of "hexhash  filename") and
// returns the hex hash for the given filename. Returns an error if not found.
func parseChecksum(data []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "hexhash  filename" (two spaces) or "hexhash *filename" (binary mode).
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		// The filename field may have a leading '*' (binary mode indicator).
		name := strings.TrimPrefix(parts[1], "*")
		if name == filename {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %q", filename)
}

// extractFromTarGz searches a tar.gz archive (provided as raw bytes) for a
// file whose base name matches targetName. Returns the file's contents.
func extractFromTarGz(data []byte, targetName string) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar read: %w", err)
		}
		if filepath.Base(hdr.Name) == targetName {
			// Reject symlinks and other non-regular files to prevent archive attacks.
			if hdr.Typeflag != tar.TypeReg {
				return nil, fmt.Errorf("archive entry %q is not a regular file (type %d)", hdr.Name, hdr.Typeflag)
			}
			// Limit extracted size to 200MB to prevent decompression bombs.
			lr := io.LimitReader(tr, 200<<20)
			content, err := io.ReadAll(lr)
			if err != nil {
				return nil, fmt.Errorf("read tar entry: %w", err)
			}
			// Verify the entry was fully read (not silently truncated).
			if _, err := tr.Read(make([]byte, 1)); err != io.EOF {
				return nil, fmt.Errorf("archive entry %q exceeds 200MB size limit", hdr.Name)
			}
			return content, nil
		}
	}
	return nil, fmt.Errorf("file %q not found in archive", targetName)
}

// extractFromZip searches a zip archive (provided as raw bytes) for a file
// whose base name matches targetName. Returns the file's contents.
func extractFromZip(data []byte, targetName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip reader: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == targetName {
			// Reject symlinks and other non-regular files to prevent archive attacks.
			if !f.FileInfo().Mode().IsRegular() {
				return nil, fmt.Errorf("archive entry %q is not a regular file (mode %s)", f.Name, f.FileInfo().Mode())
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open zip entry: %w", err)
			}
			// Limit extracted size to 200MB to prevent decompression bombs.
			content, err := io.ReadAll(io.LimitReader(rc, 200<<20))
			if err != nil {
				rc.Close()
				return nil, fmt.Errorf("read zip entry: %w", err)
			}
			// Verify the entry was fully read (not silently truncated).
			if _, err := rc.Read(make([]byte, 1)); err != io.EOF {
				rc.Close()
				return nil, fmt.Errorf("archive entry %q exceeds 200MB size limit", f.Name)
			}
			rc.Close()
			return content, nil
		}
	}
	return nil, fmt.Errorf("file %q not found in zip archive", targetName)
}

// replaceBinary atomically replaces the binary at path with newContent.
// On Unix: writes to a sibling .new file, then renames over the original.
// On Windows: renames original to .bak, then renames .new to original.
// If the Windows rename fails, it attempts to restore from .bak.
func replaceBinary(path string, newContent []byte) error {
	newPath := path + ".new"

	// Write the new binary to a temp file next to the original.
	if err := os.WriteFile(newPath, newContent, 0o755); err != nil {
		return fmt.Errorf("write new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		bakPath := path + ".bak"
		if err := os.Rename(path, bakPath); err != nil {
			_ = os.Remove(newPath)
			return fmt.Errorf("rename original to .bak: %w", err)
		}
		if err := os.Rename(newPath, path); err != nil {
			// Try to restore from backup.
			_ = os.Rename(bakPath, path)
			return fmt.Errorf("rename .new to original: %w", err)
		}
		_ = os.Remove(bakPath)
		return nil
	}

	// Unix: atomic rename.
	if err := os.Rename(newPath, path); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("rename new binary over original: %w", err)
	}
	return nil
}

// defaultBaseURL is the template for GitHub release asset URLs.
const defaultBaseURL = "https://github.com/kehoej/contextception/releases/download/{tag}/{name}"

// SelfUpdate downloads the release archive for newVersion, verifies its
// checksum, extracts the binary, and atomically replaces binaryPath.
//
// baseURL is a template with {tag} and {name} placeholders. Pass an empty
// string to use the default GitHub releases URL.
func SelfUpdate(binaryPath, newVersion, baseURL string) error {
	// Validate version string to prevent URL injection.
	v := ensureVPrefix(newVersion)
	if !semver.IsValid(v) {
		return fmt.Errorf("invalid version: %s", newVersion)
	}

	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// Build URL helper.
	makeURL := func(tag, name string) string {
		u := strings.ReplaceAll(baseURL, "{tag}", tag)
		u = strings.ReplaceAll(u, "{name}", name)
		return u
	}

	// 1. Download checksums.txt.
	checksumsURL := makeURL(v, "checksums.txt")
	checksumsData, err := httpGet(checksumsURL)
	if err != nil {
		return fmt.Errorf("download checksums.txt: %w", err)
	}

	// 2. Verify checksums.txt signature if available.
	sigURL := makeURL(v, "checksums.txt.minisig")
	sigData, sigErr := httpGetOptional(sigURL)
	switch {
	case sigErr == nil:
		if err := verifyChecksums(checksumsData, sigData); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	case errors.Is(sigErr, errNotFound):
		return fmt.Errorf("release not signed: signature file not found at %s", sigURL)
	default:
		return fmt.Errorf("download signature: %w", sigErr)
	}

	// 3. Determine archive name and expected checksum.
	archiveName := archiveNameForPlatform()
	expectedChecksum, err := parseChecksum(checksumsData, archiveName)
	if err != nil {
		return fmt.Errorf("parse checksum for %s: %w", archiveName, err)
	}

	// 4. Download archive.
	archiveURL := makeURL(v, archiveName)
	archiveData, err := httpGet(archiveURL)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	// 5. Verify checksum of downloaded archive data in memory.
	actualSum := sha256.Sum256(archiveData)
	if hex.EncodeToString(actualSum[:]) != expectedChecksum {
		return fmt.Errorf("checksum verification failed: got %s, want %s",
			hex.EncodeToString(actualSum[:]), expectedChecksum)
	}

	// 6. Extract the binary from the archive.
	var binaryName string
	var newBinaryContent []byte
	if runtime.GOOS == "windows" {
		binaryName = "contextception.exe"
		newBinaryContent, err = extractFromZip(archiveData, binaryName)
	} else {
		binaryName = "contextception"
		newBinaryContent, err = extractFromTarGz(archiveData, binaryName)
	}
	if err != nil {
		return fmt.Errorf("extract binary from archive: %w", err)
	}

	// 7. Atomically replace the binary.
	if err := replaceBinary(binaryPath, newBinaryContent); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}
