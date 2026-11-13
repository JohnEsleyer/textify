package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// configHeader is the comment block added to the top of textify.yaml
const configHeader = `# ==============================================================================
# Textify Configuration (textify.yaml)
# ==============================================================================
#
# output_file:   Path where merged codebase text is saved (default: codebase.txt).
# format:        Output format: "tags" (recommended for LLMs), "raw", or "markdown".
# tree:          Prepend ASCII directory structure tree (default: true).
# max_file_size: Max file size in bytes to include (default: 512000 / 500KB).
#
# ------------------------------------------------------------------------------
# HOW TO CONFIGURE DIRECTORIES (dirs:)
# ------------------------------------------------------------------------------
# Every key under 'dirs:' is a directory path relative to project root (use "." for root).
# Do NOT use leading slashes in dir keys (e.g., use "cmd", NOT "/cmd").
#
# RULE OPTIONS PER DIRECTORY:
#   enabled:            (bool)   true to scan this folder, false to skip it and its children.
#                                NOTE: 'enabled' only takes true/false. Do NOT put file lists here.
#   include:            ([list]) Specific files or subfolders to Force Include.
#                                Overrides .gitignore and extension filters!
#                                Examples: [config, targets/golang, errors.go]
#   exclude:            ([list]) Specific files or subfolders to Force Exclude (highest priority).
#                                Examples: [targets/html, targets/jvm, obsolete.go]
#   extensions:         ([list]) Allow-list of extensions (e.g., [go, md, json]).
#                                If omitted, inherits from parent or allows text files.
#   exclude_extensions: ([list]) Block-list of extensions (e.g., [log, tmp, lock]).
#   group:              (string) Group name to categorize this folder (e.g., api, core, models).
#   compact:            (bool)   true to strip non-essential whitespace to save LLM tokens.
#
# RULE EVALUATION PRECEDENCE:
#   1. System exclusions (.git, textify.yaml, output_file)
#   2. Force Exclude (exclude list) - highest priority
#   3. Force Include (include list) - overrides gitignore and extension filters
#   4. Gitignore (.gitignore at root and subfolders)
#   5. Extension block-list (exclude_extensions)
#   6. Extension allow-list (extensions)
#
# GROUP EXTRACTION MODES (Signatures vs Full Code):
#   signatures_groups:  ([list]) Groups that should be extracted as signatures only.
#   full_code_groups:   ([list]) Groups that should be extracted as full code.
#   groups:             (map)    Map of group name to mode ("signatures" or "full").
#
# COMMON CLI COMMANDS:
#   textify check              Validate textify.yaml and inspect active directory rules.
#   textify scan               Auto-detect new folders and update textify.yaml.
#   textify start              Bundle full project codebase into context file.
#   textify signatures         Export symbol signatures (functions, structs, classes).
#   textify tokens             List folders and estimated token sizes.
# ==============================================================================

`

// StringList is a helper slice that unmarshals flexibly from either a single
// scalar string or a list of strings in YAML.
type StringList []string

func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		val := strings.TrimSpace(value.Value)
		if val != "" {
			*s = []string{val}
		} else {
			*s = nil
		}
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		var list []string
		for _, item := range value.Content {
			val := strings.TrimSpace(item.Value)
			if val != "" {
				list = append(list, val)
			}
		}
		*s = list
		return nil
	}
	return nil
}

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
	Extensions StringList `yaml:"extensions,omitempty"`

	// ExcludeExtensions is a list of file extensions to specifically ignore.
	ExcludeExtensions StringList `yaml:"exclude_extensions,omitempty"`

	// Include is a list of specific files or patterns to force-include
	// regardless of extension or gitignore rules.
	Include StringList `yaml:"include,omitempty"`

	// Exclude is a list of specific files or patterns to force-exclude.
	// This takes precedence over Include.
	Exclude StringList `yaml:"exclude,omitempty"`
}

// UnmarshalYAML provides resilient unmarshaling for DirRule, tolerating scalars
// and sequence inputs.
func (d *DirRule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node for directory rule, got kind %d", value.Kind)
	}

	d.Enabled = true // default to true if unspecified

	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := strings.ToLower(strings.TrimSpace(keyNode.Value))

		switch key {
		case "enabled":
			if valNode.Kind == yaml.ScalarNode {
				val := strings.ToLower(strings.TrimSpace(valNode.Value))
				if len(strings.Fields(val)) > 1 || strings.Contains(valNode.Value, "\n") {
					return fmt.Errorf("line %d: cannot unmarshal list into enabled: 'enabled' only takes a boolean (true or false)", keyNode.Line)
				}
				d.Enabled = val == "true" || val == "yes" || val == "1" || val == "on"
			} else if valNode.Kind == yaml.SequenceNode {
				// User provided a list directly under enabled; tolerate and treat items as include
				d.Enabled = true
				for _, item := range valNode.Content {
					trimmed := strings.TrimSpace(item.Value)
					if trimmed != "" {
						d.Include = append(d.Include, trimmed)
					}
				}
			}
		case "compact":
			if valNode.Kind == yaml.ScalarNode {
				val := strings.ToLower(strings.TrimSpace(valNode.Value))
				d.Compact = val == "true" || val == "yes" || val == "1"
			}
		case "group":
			d.Group = strings.TrimSpace(valNode.Value)
		case "extensions", "ext", "extension":
			var list StringList
			if err := valNode.Decode(&list); err == nil {
				d.Extensions = list
			}
		case "exclude_extensions", "exclude_ext":
			var list StringList
			if err := valNode.Decode(&list); err == nil {
				d.ExcludeExtensions = list
			}
		case "include", "includes":
			var list StringList
			if err := valNode.Decode(&list); err == nil {
				d.Include = append(d.Include, list...)
			}
		case "exclude", "excludes":
			var list StringList
			if err := valNode.Decode(&list); err == nil {
				d.Exclude = append(d.Exclude, list...)
			}
		}
	}

	// Normalize list values
	d.Extensions = cleanStringList(d.Extensions, true)
	d.ExcludeExtensions = cleanStringList(d.ExcludeExtensions, true)
	d.Include = cleanPathList(d.Include)
	d.Exclude = cleanPathList(d.Exclude)

	return nil
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
	Format           string               `yaml:"format,omitempty"`        // "raw" (default), "markdown", "xml", "tags"
	Tree             *bool                `yaml:"tree,omitempty"`          // prepend directory tree (default true)
	MaxFileSize      int64                `yaml:"max_file_size,omitempty"` // file size limit in bytes (default 500KB)
	SignaturesGroups StringList           `yaml:"signatures_groups,omitempty"`
	SignatureGroups  StringList           `yaml:"signature_groups,omitempty"` // alias
	FullCodeGroups   StringList           `yaml:"full_code_groups,omitempty"`
	Groups           map[string]GroupRule `yaml:"groups,omitempty"`
	Dirs             map[string]DirRule   `yaml:"dirs"`
}

// Normalize sanitizes directory keys, file paths, extensions, and default values.
func (c *Config) Normalize() {
	if c.OutputFile == "" {
		c.OutputFile = "codebase.txt"
	}

	if c.Dirs == nil {
		c.Dirs = make(map[string]DirRule)
		return
	}

	cleanedDirs := make(map[string]DirRule, len(c.Dirs))
	for dirKey, rule := range c.Dirs {
		cleanKey := normalizeDirKey(dirKey)
		rule.Include = cleanPathList(rule.Include)
		rule.Exclude = cleanPathList(rule.Exclude)
		rule.Extensions = cleanStringList(rule.Extensions, true)
		rule.ExcludeExtensions = cleanStringList(rule.ExcludeExtensions, true)
		cleanedDirs[cleanKey] = rule
	}
	c.Dirs = cleanedDirs
}

// Validate checks directory rules for common configuration mistakes and non-existent paths.
func (c *Config) Validate(root string) []string {
	var warnings []string

	if c.Format != "" {
		f := strings.ToLower(strings.TrimSpace(c.Format))
		switch f {
		case "tags", "raw", "markdown", "md", "xml":
		default:
			warnings = append(warnings, fmt.Sprintf("Unrecognized format %q (expected 'tags', 'raw', or 'markdown')", c.Format))
		}
	}

	for dirKey := range c.Dirs {
		if dirKey == "." {
			continue
		}
		absDir := filepath.Join(root, filepath.FromSlash(dirKey))
		info, err := os.Stat(absDir)
		if err != nil || !info.IsDir() {
			warnings = append(warnings, fmt.Sprintf("Directory %q in 'dirs' does not exist in workspace", dirKey))
		}
	}

	return warnings
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
		Format:      "tags",
		Tree:        &treeDefault,
		MaxFileSize: 500 * 1024, // 500 KB
		Dirs:        make(map[string]DirRule),
	}
}

// Load reads, parses, and validates the configuration file with diagnostics.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s", formatYamlError(path, string(data), err))
	}

	cfg.Normalize()
	return &cfg, nil
}

func formatYamlError(path, content string, err error) string {
	errStr := err.Error()
	lines := strings.Split(content, "\n")

	lineNum := 0
	reLine := regexp.MustCompile(`line\s+(\d+)`)
	if m := reLine.FindStringSubmatch(errStr); len(m) > 1 {
		lineNum, _ = strconv.Atoi(m[1])
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("❌ Error parsing %s", filepath.Base(path)))
	if lineNum > 0 {
		sb.WriteString(fmt.Sprintf(" (line %d)", lineNum))
	}
	sb.WriteString(":\n")

	if lineNum > 0 && lineNum <= len(lines) {
		start := lineNum - 3
		if start < 0 {
			start = 0
		}
		end := lineNum + 2
		if end > len(lines) {
			end = len(lines)
		}

		for i := start; i < end; i++ {
			prefix := "   "
			if i+1 == lineNum {
				prefix = " > "
			}
			sb.WriteString(fmt.Sprintf("%s%4d | %s\n", prefix, i+1, lines[i]))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Reason: %s\n", errStr))

	// Provide targeted guidance for common YAML mistakes in textify.yaml
	lowerContent := strings.ToLower(content)
	if strings.Contains(errStr, "mapping values are not allowed") ||
		strings.Contains(errStr, "did not find expected key") ||
		strings.Contains(errStr, "cannot unmarshal") {

		// Check if user indented a list directly under enabled: true
		hasEnabled := false
		if lineNum > 0 && lineNum <= len(lines) {
			for i := lineNum - 2; i <= lineNum; i++ {
				if i >= 0 && i < len(lines) && strings.Contains(lines[i], "enabled:") {
					hasEnabled = true
					break
				}
			}
		}
		if !hasEnabled && strings.Contains(lowerContent, "enabled:") {
			hasEnabled = true
		}
		if hasEnabled {
			sb.WriteString("\n💡 Guidance: 'enabled' only takes a boolean (true or false).\n")
			sb.WriteString("   To include specific files or subdirectories, use the 'include:' list:\n\n")
			sb.WriteString("   dirs:\n")
			sb.WriteString("     internal:\n")
			sb.WriteString("       enabled: true\n")
			sb.WriteString("       include:\n")
			sb.WriteString("         - config\n")
			sb.WriteString("         - fileutil\n")
			return sb.String()
		}
	}

	if strings.Contains(lowerContent, "\t") {
		sb.WriteString("\n💡 Guidance: Tab characters were found in textify.yaml. YAML strictly requires spaces for indentation (use 2 spaces).\n")
	} else {
		sb.WriteString("\n💡 Guidance: Ensure 2-space indentation and check that every key is followed by a space (e.g., 'enabled: true').\n")
	}

	return sb.String()
}

func normalizeDirKey(k string) string {
	k = strings.TrimSpace(filepath.ToSlash(k))
	k = strings.TrimPrefix(k, "./")
	k = strings.TrimPrefix(k, "/")
	k = strings.TrimSuffix(k, "/")
	if k == "" {
		return "."
	}
	return k
}

func cleanPathList(paths []string) []string {
	var clean []string
	seen := make(map[string]bool)
	for _, p := range paths {
		p = strings.TrimSpace(filepath.ToSlash(p))
		if p == "." {
			if !seen["."] {
				seen["."] = true
				clean = append(clean, ".")
			}
			continue
		}
		p = strings.TrimPrefix(p, "./")
		p = strings.TrimPrefix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if p != "" && !seen[p] {
			seen[p] = true
			clean = append(clean, p)
		}
	}
	return clean
}

func cleanStringList(items []string, removeDot bool) []string {
	var clean []string
	seen := make(map[string]bool)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if removeDot {
			item = strings.TrimPrefix(item, ".")
		}
		if item != "" && !seen[item] {
			seen[item] = true
			clean = append(clean, item)
		}
	}
	return clean
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
