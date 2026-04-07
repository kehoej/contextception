package config

import (
	"bytes"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// UpdateConfig holds update-check settings.
type UpdateConfig struct {
	Check bool `yaml:"check"`
}

// GlobalConfig represents the user's global contextception configuration,
// stored at {configDir}/contextception/config.yaml.
type GlobalConfig struct {
	Update UpdateConfig `yaml:"update"`
}

// DefaultGlobal returns a GlobalConfig with all default values applied.
// Update.Check defaults to true.
func DefaultGlobal() *GlobalConfig {
	return &GlobalConfig{
		Update: UpdateConfig{Check: true},
	}
}

// LoadGlobal reads {configDir}/contextception/config.yaml and returns a
// GlobalConfig. It returns defaults if the file is missing or malformed —
// global config is tolerant by design since it may evolve over time.
func LoadGlobal(configDir string) (*GlobalConfig, error) {
	path := filepath.Join(configDir, "contextception", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		// Missing file is fine — return defaults.
		return DefaultGlobal(), nil
	}

	cfg := DefaultGlobal()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(cfg); err != nil {
		// Malformed YAML — return defaults, not an error.
		return DefaultGlobal(), nil
	}

	return cfg, nil
}
