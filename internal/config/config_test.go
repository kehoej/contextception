package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".contextception")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissing(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 0 {
		t.Errorf("Version = %d, want 0", cfg.Version)
	}
	if len(cfg.Entrypoints) != 0 {
		t.Errorf("Entrypoints = %v, want empty", cfg.Entrypoints)
	}
}

func TestLoadValid(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
version: 1
entrypoints:
  - cmd/server/main.go
  - cmd/cli/main.go
ignore:
  - vendor
  - third_party/
generated:
  - gen/
  - proto/generated
`)

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if len(cfg.Entrypoints) != 2 {
		t.Errorf("Entrypoints count = %d, want 2", len(cfg.Entrypoints))
	}
	if len(cfg.Ignore) != 2 {
		t.Errorf("Ignore count = %d, want 2", len(cfg.Ignore))
	}
	if len(cfg.Generated) != 2 {
		t.Errorf("Generated count = %d, want 2", len(cfg.Generated))
	}

	// Verify trailing slash normalization.
	if cfg.Ignore[0] != "vendor/" {
		t.Errorf("Ignore[0] = %q, want %q", cfg.Ignore[0], "vendor/")
	}
	if cfg.Ignore[1] != "third_party/" {
		t.Errorf("Ignore[1] = %q, want %q", cfg.Ignore[1], "third_party/")
	}
	if cfg.Generated[0] != "gen/" {
		t.Errorf("Generated[0] = %q, want %q", cfg.Generated[0], "gen/")
	}
	if cfg.Generated[1] != "proto/generated/" {
		t.Errorf("Generated[1] = %q, want %q", cfg.Generated[1], "proto/generated/")
	}
}

func TestLoadUnknownKey(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
version: 1
entrypoints: []
unknown_key: something
`)

	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestLoadInvalidVersion(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
version: 99
entrypoints: []
`)

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = cfg.Validate(root)
	if err == nil {
		t.Fatal("expected validation error for version 99")
	}
}

func TestLoadMissingVersion(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
entrypoints:
  - main.py
`)

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	// version defaults to 0, which should fail validation.
	_, err = cfg.Validate(root)
	if err == nil {
		t.Fatal("expected validation error for missing version (0)")
	}
}

func TestIsIgnored(t *testing.T) {
	cfg := &Config{
		Ignore: []string{"vendor/", "third_party/"},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"vendor/lib/foo.py", true},
		{"vendor/bar.py", true},
		{"third_party/proto.py", true},
		{"src/main.py", false},
		{"vendored/lib.py", false},
	}

	for _, tt := range tests {
		if got := cfg.IsIgnored(tt.path); got != tt.want {
			t.Errorf("IsIgnored(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsGenerated(t *testing.T) {
	cfg := &Config{
		Generated: []string{"gen/", "proto/generated/"},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"gen/models.py", true},
		{"proto/generated/api.py", true},
		{"proto/manual.py", false},
		{"src/gen.py", false},
	}

	for _, tt := range tests {
		if got := cfg.IsGenerated(tt.path); got != tt.want {
			t.Errorf("IsGenerated(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsConfigEntrypoint(t *testing.T) {
	cfg := &Config{
		Entrypoints: []string{"cmd/server/main.go", "cmd/cli/main.go"},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"cmd/server/main.go", true},
		{"cmd/cli/main.go", true},
		{"cmd/other/main.go", false},
		{"main.go", false},
	}

	for _, tt := range tests {
		if got := cfg.IsConfigEntrypoint(tt.path); got != tt.want {
			t.Errorf("IsConfigEntrypoint(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestValidateAbsolutePath(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{
		Version:     1,
		Entrypoints: []string{"/absolute/path/main.py"},
	}

	_, err := cfg.Validate(root)
	if err == nil {
		t.Fatal("expected error for absolute path in config")
	}
}

func TestValidateNonexistentPath(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{
		Version:     1,
		Entrypoints: []string{"nonexistent/main.py"},
		Ignore:      []string{"missing_dir/"},
		Generated:   []string{"missing_gen/"},
	}

	warnings, err := cfg.Validate(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 3 {
		t.Errorf("expected 3 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestEmptyConfig(t *testing.T) {
	cfg := Empty()

	if cfg.IsIgnored("anything/foo.py") {
		t.Error("Empty config should not ignore any path")
	}
	if cfg.IsGenerated("anything/foo.py") {
		t.Error("Empty config should not mark any path as generated")
	}
	if cfg.IsConfigEntrypoint("anything/foo.py") {
		t.Error("Empty config should not mark any path as entrypoint")
	}
}
