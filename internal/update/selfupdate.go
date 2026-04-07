// Package update provides self-update detection and installation method logic.
package update

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

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
