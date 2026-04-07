package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGlobalConfig(t *testing.T, configDir, content string) {
	t.Helper()
	dir := filepath.Join(configDir, "contextception")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadGlobalMissing(t *testing.T) {
	configDir := t.TempDir()
	cfg, err := LoadGlobal(configDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Update.Check {
		t.Error("Update.Check = false, want true (default)")
	}
}

func TestLoadGlobalCheckDisabled(t *testing.T) {
	configDir := t.TempDir()
	writeGlobalConfig(t, configDir, "update:\n  check: false\n")

	cfg, err := LoadGlobal(configDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Update.Check {
		t.Error("Update.Check = true, want false")
	}
}

func TestLoadGlobalMalformed(t *testing.T) {
	configDir := t.TempDir()
	writeGlobalConfig(t, configDir, "update:\n  check: [invalid yaml\n")

	cfg, err := LoadGlobal(configDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Update.Check {
		t.Error("Update.Check = false, want true (default for malformed YAML)")
	}
}

func TestDefaultGlobal(t *testing.T) {
	cfg := DefaultGlobal()
	if !cfg.Update.Check {
		t.Error("DefaultGlobal().Update.Check = false, want true")
	}
}
