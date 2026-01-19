package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

// DirRule defines filtering rules for a specific directory.
type DirRule struct {
	// Extensions is a list of file extensions to include (e.g., ["go", "md"]).
	// If empty, the behavior depends on the scanner implementation (usually inherits or includes all).
	Extensions []string `toml:"extensions,omitempty"`
	
	// Include is a list of specific files or patterns to force-include
	// regardless of extension or gitignore rules.
	Include []string `toml:"include,omitempty"`
}

// Config represents the top-level structure of the textify.toml file.
type Config struct {
	OutputFile string             `toml:"output_file"`
	Dirs       map[string]DirRule `toml:"dirs"`
}

// DefaultConfig returns a Config struct populated with sensible defaults for the root directory.
func DefaultConfig() Config {
	cfg := Config{
		OutputFile: "codebase.txt",
		Dirs:       make(map[string]DirRule),
	}

	// Default rules for the root directory
	cfg.Dirs["."] = DirRule{
		Extensions: []string{"go", "md", "txt", "json", "js", "ts", "yml", "yaml", "toml"},
		Include:    []string{".env.example"},
	}

	return cfg
}

// Load reads and parses the configuration file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save marshals the configuration and writes it to the given path.
func Save(cfg *Config, path string) error {
	return cfg.Save(path) // Proxy to method
}

// Save is a method on Config to write itself to disk.
func (c *Config) Save(path string) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
