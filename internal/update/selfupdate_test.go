// Package update provides self-update detection and installation method logic.
package update

import (
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
