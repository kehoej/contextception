// Package config handles loading and validating .contextception/config.yaml.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the user's repository configuration.
type Config struct {
	Version     int      `yaml:"version"`
	Entrypoints []string `yaml:"entrypoints,omitempty"`
	Ignore      []string `yaml:"ignore,omitempty"`
	Generated   []string `yaml:"generated,omitempty"`
}

// Load reads .contextception/config.yaml from repoRoot.
// Returns Empty() if the file does not exist.
// Returns an error if the file exists but is malformed or contains unknown keys.
func Load(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, ".contextception", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Empty(), nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Normalize ignore/generated directory paths to end with "/".
	normalizePaths(cfg.Ignore)
	normalizePaths(cfg.Generated)

	return &cfg, nil
}

// Empty returns a zero-value config where all predicates return false.
func Empty() *Config {
	return &Config{}
}

// Validate checks config for correctness.
// Returns warnings for non-fatal issues (e.g. non-existent paths) and
// an error for fatal issues (e.g. wrong version, absolute paths).
func (c *Config) Validate(repoRoot string) (warnings []string, err error) {
	if c.Version != 1 {
		return nil, fmt.Errorf("unsupported config version %d (expected 1)", c.Version)
	}

	allPaths := make([]string, 0, len(c.Entrypoints)+len(c.Ignore)+len(c.Generated))
	allPaths = append(allPaths, c.Entrypoints...)
	allPaths = append(allPaths, c.Ignore...)
	allPaths = append(allPaths, c.Generated...)

	for _, p := range allPaths {
		if filepath.IsAbs(p) {
			return nil, fmt.Errorf("absolute path not allowed in config: %s", p)
		}
	}

	// Warn on non-existent entrypoint files.
	for _, p := range c.Entrypoints {
		abs := filepath.Join(repoRoot, p)
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("entrypoint path does not exist: %s", p))
		}
	}

	// Warn on non-existent ignore/generated directories.
	for _, pair := range []struct {
		label string
		paths []string
	}{
		{"ignore", c.Ignore},
		{"generated", c.Generated},
	} {
		for _, p := range pair.paths {
			dir := strings.TrimSuffix(p, "/")
			abs := filepath.Join(repoRoot, dir)
			if _, err := os.Stat(abs); os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("%s path does not exist: %s", pair.label, dir))
			}
		}
	}

	return warnings, nil
}

// IsIgnored returns true if path matches any config ignore prefix.
func (c *Config) IsIgnored(path string) bool {
	for _, prefix := range c.Ignore {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// IsGenerated returns true if path matches any config generated prefix.
func (c *Config) IsGenerated(path string) bool {
	for _, prefix := range c.Generated {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// IsConfigEntrypoint returns true if path exactly matches a config entrypoint.
func (c *Config) IsConfigEntrypoint(path string) bool {
	for _, ep := range c.Entrypoints {
		if ep == path {
			return true
		}
	}
	return false
}

// RepoProfile represents an auto-detected repository type.
type RepoProfile struct {
	Type        string   // monorepo, python-package, go-module, ts-project, mixed, unknown
	Signals     []string // detection signals found
	Entrypoints []string // auto-detected entrypoints
}

// DetectRepoProfile detects the repository type from file structure.
// Returns a profile with type and suggested entrypoints.
func DetectRepoProfile(repoRoot string) RepoProfile {
	profile := RepoProfile{Type: "unknown"}

	// Check for language config files.
	hasGoMod := fileExists(repoRoot, "go.mod")
	hasGoWork := fileExists(repoRoot, "go.work")
	hasPyproject := fileExists(repoRoot, "pyproject.toml")
	hasSetupPy := fileExists(repoRoot, "setup.py")
	hasTSConfig := fileExists(repoRoot, "tsconfig.json")
	hasPackageJSON := fileExists(repoRoot, "package.json")
	hasPnpmWorkspace := fileExists(repoRoot, "pnpm-workspace.yaml")
	hasCargoToml := fileExists(repoRoot, "Cargo.toml")
	hasPomXML := fileExists(repoRoot, "pom.xml")
	hasBuildGradle := fileExists(repoRoot, "build.gradle") || fileExists(repoRoot, "build.gradle.kts")

	// Count language types present.
	var langCount int
	if hasGoMod || hasGoWork {
		langCount++
	}
	if hasPyproject || hasSetupPy {
		langCount++
	}
	if hasTSConfig || hasPackageJSON {
		langCount++
	}
	if hasCargoToml {
		langCount++
	}
	if hasPomXML || hasBuildGradle {
		langCount++
	}

	// Detect monorepo signals.
	isMonorepo := hasPnpmWorkspace || hasGoWork
	if hasPackageJSON && !isMonorepo {
		// Check for workspaces field in package.json.
		if data, err := os.ReadFile(filepath.Join(repoRoot, "package.json")); err == nil {
			if strings.Contains(string(data), `"workspaces"`) {
				isMonorepo = true
				profile.Signals = append(profile.Signals, "package.json workspaces")
			}
		}
	}
	// Multiple go.mod files indicate a monorepo.
	if hasGoMod && !isMonorepo {
		if entries, err := os.ReadDir(repoRoot); err == nil {
			goModCount := 0
			for _, e := range entries {
				if e.IsDir() {
					if fileExists(filepath.Join(repoRoot, e.Name()), "go.mod") {
						goModCount++
					}
				}
			}
			if goModCount >= 2 {
				isMonorepo = true
				profile.Signals = append(profile.Signals, "multiple go.mod")
			}
		}
	}

	if isMonorepo {
		profile.Type = "monorepo"
		if hasPnpmWorkspace {
			profile.Signals = append(profile.Signals, "pnpm-workspace.yaml")
		}
		if hasGoWork {
			profile.Signals = append(profile.Signals, "go.work")
		}
		return profile
	}

	if langCount >= 2 {
		profile.Type = "mixed"
		profile.Signals = append(profile.Signals, "multiple language configs")
		return profile
	}

	// Single-language profiles.
	switch {
	case hasGoMod:
		profile.Type = "go-module"
		profile.Signals = append(profile.Signals, "go.mod")
		// Auto-detect cmd/*/main.go entrypoints.
		if entries, err := os.ReadDir(filepath.Join(repoRoot, "cmd")); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					mainPath := filepath.Join("cmd", e.Name(), "main.go")
					if fileExists(repoRoot, mainPath) {
						profile.Entrypoints = append(profile.Entrypoints, mainPath)
					}
				}
			}
		}

	case hasPyproject || hasSetupPy:
		profile.Type = "python-package"
		if hasPyproject {
			profile.Signals = append(profile.Signals, "pyproject.toml")
		}
		if hasSetupPy {
			profile.Signals = append(profile.Signals, "setup.py")
		}
		// Auto-detect src/ layout entrypoint.
		if info, err := os.Stat(filepath.Join(repoRoot, "src")); err == nil && info.IsDir() {
			profile.Signals = append(profile.Signals, "src/ layout")
		}

	case hasTSConfig:
		profile.Type = "ts-project"
		profile.Signals = append(profile.Signals, "tsconfig.json")

	case hasCargoToml:
		profile.Type = "rust-crate"
		profile.Signals = append(profile.Signals, "Cargo.toml")
		if fileExists(repoRoot, "src/main.rs") {
			profile.Entrypoints = append(profile.Entrypoints, "src/main.rs")
		}
		if fileExists(repoRoot, "src/lib.rs") {
			profile.Entrypoints = append(profile.Entrypoints, "src/lib.rs")
		}

	case hasPomXML || hasBuildGradle:
		profile.Type = "java-project"
		if hasPomXML {
			profile.Signals = append(profile.Signals, "pom.xml")
		}
		if hasBuildGradle {
			profile.Signals = append(profile.Signals, "build.gradle")
		}
	}

	return profile
}

// fileExists checks if a file exists at repoRoot/name.
func fileExists(base, name string) bool {
	_, err := os.Stat(filepath.Join(base, name))
	return err == nil
}

// normalizePaths ensures each path in the slice ends with "/".
func normalizePaths(paths []string) {
	for i, p := range paths {
		if p != "" && !strings.HasSuffix(p, "/") {
			paths[i] = p + "/"
		}
	}
}
