package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// configHeader is the comment block added to the top of textify.yaml
const configHeader = `# Textify Configuration
#
# output_file: Path where the merged codebase text will be saved.
# dirs:        Directory-specific configurations. Keys are paths relative to root.
#
# Rule Options:
#   enabled:            (bool)   If false, this directory and its children are skipped.
#   compact:            (bool)   If true, strips extra whitespace and condenses formatting to save LLM tokens.
#   group:              (string) Optional group name to categorize this directory.
#   include:            ([list]) Specific files/globs to Force Include (overrides gitignore & extensions).
#   exclude:            ([list]) Specific files/globs to Force Exclude (highest priority).
#   extensions:         ([list]) Allow-list of extensions (e.g., [go, js]). If empty, all text files are allowed.
#   exclude_extensions: ([list]) Block-list of extensions (e.g., [log, tmp]).
#
# Group Options:
#   signatures_groups:  ([list]) Groups that should be extracted as signatures only.
#   full_code_groups:   ([list]) Groups that should be extracted as full code.
#   groups:             (map)    Map of group name to mode ("signatures" or "full").
#
# Usage:
#   - Run 'textify scan' to detect new folders and update this file.
#   - Run 'textify start' to generate the output file.

`

// DirRule defines filtering rules for a specific directory.
type DirRule struct {
	// Enabled determines if this directory is scanned at all.
	Enabled bool `yaml:"enabled"`

	// Compact strips extra whitespace and non-essential lines to save LLM tokens.
	Compact bool `yaml:"compact,omitempty"`

	// Group specifies an optional group name for this directory.
	Group string `yaml:"group,omitempty"`

	// Extensions is a list of file extensions to include (e.g., ["go", "md"]).
	// If empty, all text files are considered (subject to exclusions).
	Extensions []string `yaml:"extensions,omitempty"`

	// ExcludeExtensions is a list of file extensions to specifically ignore.
	ExcludeExtensions []string `yaml:"exclude_extensions,omitempty"`

	// Include is a list of specific files or patterns to force-include
	// regardless of extension or gitignore rules.
	Include []string `yaml:"include,omitempty"`

	// Exclude is a list of specific files or patterns to force-exclude.
	// This takes precedence over Include.
	Exclude []string `yaml:"exclude,omitempty"`
}

// GroupRule specifies mode settings for a group.
type GroupRule struct {
	Mode           string `yaml:"mode,omitempty"` // "signatures" or "full"
	SignaturesOnly bool   `yaml:"signatures_only,omitempty"`
	FullCode       bool   `yaml:"full_code,omitempty"`
}

func (g *GroupRule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		g.Mode = value.Value
		return nil
	}
	type rawGroupRule GroupRule
	var raw rawGroupRule
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*g = GroupRule(raw)
	return nil
}

// Config represents the top-level structure of the textify.yaml file.
type Config struct {
	OutputFile       string               `yaml:"output_file"`
	Format           string               `yaml:"format,omitempty"`        // "raw" (default), "markdown", "xml"
	Tree             *bool                `yaml:"tree,omitempty"`          // prepend directory tree (default true)
	MaxFileSize      int64                `yaml:"max_file_size,omitempty"` // file size limit in bytes (default 500KB)
	SignaturesGroups []string             `yaml:"signatures_groups,omitempty"`
	SignatureGroups  []string             `yaml:"signature_groups,omitempty"` // alias
	FullCodeGroups   []string             `yaml:"full_code_groups,omitempty"`
	Groups           map[string]GroupRule `yaml:"groups,omitempty"`
	Dirs             map[string]DirRule   `yaml:"dirs"`
}

// IsGroupSignaturesOnly reports whether the group is designated for signatures only.
func (c *Config) IsGroupSignaturesOnly(group string) bool {
	if group == "" {
		return false
	}
	for _, g := range c.SignaturesGroups {
		if strings.EqualFold(g, group) {
			return true
		}
	}
	for _, g := range c.SignatureGroups {
		if strings.EqualFold(g, group) {
			return true
		}
	}
	if gr, ok := c.Groups[group]; ok {
		if gr.SignaturesOnly {
			return true
		}
		mode := strings.ToLower(strings.TrimSpace(gr.Mode))
		if mode == "signatures" || mode == "signatures_only" || mode == "sigs" || mode == "sig" || mode == "signature" {
			return true
		}
	}
	return false
}

// IsGroupFullCode reports whether the group is designated for full code.
func (c *Config) IsGroupFullCode(group string) bool {
	if group == "" {
		return false
	}
	for _, g := range c.FullCodeGroups {
		if strings.EqualFold(g, group) {
			return true
		}
	}
	if gr, ok := c.Groups[group]; ok {
		if gr.FullCode {
			return true
		}
		mode := strings.ToLower(strings.TrimSpace(gr.Mode))
		if mode == "full" || mode == "full_code" || mode == "fullcode" || mode == "full-code" {
			return true
		}
	}
	return false
}

// DefaultConfig returns a barebones config.
func DefaultConfig() Config {
	treeDefault := true
	return Config{
		OutputFile:  "codebase.txt",
		Format:      "raw",
		Tree:        &treeDefault,
		MaxFileSize: 500 * 1024, // 500 KB
		Dirs:        make(map[string]DirRule),
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
