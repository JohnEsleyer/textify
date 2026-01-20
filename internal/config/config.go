package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// configHeader is the comment block added to the top of textify.yaml
const configHeader = `# Textify Configuration
#
# output_file: Path where the merged codebase text will be saved.
# dirs:        Directory-specific configurations. Keys are paths relative to root.
#
# Rule Options:
#   enabled:    (bool)   If false, this directory and its children are skipped.
#   extensions: ([list]) File extensions to include (e.g., [go, js, md]).
#   include:    ([list]) Specific files to force-include (overrides .gitignore).
#
# Usage:
#   - Run 'textify scan' to detect new folders and update this file.
#   - Run 'textify start' to generate the output file.

`

// DirRule defines filtering rules for a specific directory.
type DirRule struct {
	// Enabled determines if this directory is scanned at all.
	Enabled bool `yaml:"enabled"`

	// Extensions is a list of file extensions to include (e.g., ["go", "md"]).
	Extensions []string `yaml:"extensions,omitempty"`
	
	// Include is a list of specific files or patterns to force-include
	// regardless of extension or gitignore rules.
	Include []string `yaml:"include,omitempty"`
}

// Config represents the top-level structure of the textify.yaml file.
type Config struct {
	OutputFile string             `yaml:"output_file"`
	Dirs       map[string]DirRule `yaml:"dirs"`
}

// DefaultConfig returns a barebones config.
func DefaultConfig() Config {
	return Config{
		OutputFile: "codebase.txt",
		Dirs:       make(map[string]DirRule),
	}
}

// Load reads and parses the configuration file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// Ensure map is initialized
	if cfg.Dirs == nil {
		cfg.Dirs = make(map[string]DirRule)
	}
	return &cfg, nil
}

// Save marshals the configuration and writes it to the given path with a header.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	// Combine the header comments with the generated YAML
	content := append([]byte(configHeader), data...)

	return os.WriteFile(path, content, 0644)
}
