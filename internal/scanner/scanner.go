package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
)

// Scan initiates the directory walk based on the provided configuration.
func Scan(rootPath string, cfg *config.Config, writer io.Writer, signaturesOnly bool) error {
	chain := config.NewGitIgnoreChain(rootPath)
	bufWriter := bufio.NewWriter(writer)
	defer bufWriter.Flush()

	// Initial rule (Root ".")
	rootRule, ok := cfg.Dirs["."]
	if !ok {
		// If root is missing from config, default to enabled but no extensions
		rootRule = config.DirRule{Enabled: true, Extensions: []string{}}
	}

	return walk(rootPath, rootPath, cfg.Dirs, rootRule, chain, bufWriter, signaturesOnly)
}

func walk(
	fullPath string,
	rootPath string,
	dirRules map[string]config.DirRule,
	currentRule config.DirRule,
	ignoreChain config.GitIgnoreChain,
	writer *bufio.Writer,
	signaturesOnly bool,
) error {

	// Check if the directory we are currently IN has a specific rule
	relDir, _ := filepath.Rel(rootPath, fullPath)
	if relDir == "." {
		relDir = "."
	} else {
		relDir = filepath.ToSlash(relDir)
	}

	if specificRule, exists := dirRules[relDir]; exists {
		currentRule = specificRule
	}

	// 1. CHECK ENABLED STATUS
	// If the directory is explicitly disabled in config, stop everything here.
	if !currentRule.Enabled {
		return nil // Skip this directory and its children
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())
		relEntryPath, _ := filepath.Rel(rootPath, entryPath)
		relEntryPath = filepath.ToSlash(relEntryPath)
		ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")

		// -----------------------------
		// 1. SYSTEM EXCLUDES (Hardcoded)
		// -----------------------------
		if shouldAlwaysExclude(entry.Name()) {
			continue
		}

		// -----------------------------
		// 2. USER EXCLUDES (Specific Files/Patterns)
		// Priority: High. If excluded here, it is skipped regardless of include rules.
		// -----------------------------
		if checkPatternMatch(entry.Name(), relEntryPath, currentRule.Exclude) {
			continue
		}

		// -----------------------------
		// 3. FORCE INCLUDE (Specific Files/Patterns)
		// Priority: Overrides .gitignore and extension rules
		// -----------------------------
		isForced := checkPatternMatch(entry.Name(), relEntryPath, currentRule.Include)

		if entry.IsDir() {
			// Check if this specific SUBDIRECTORY has a rule that disables it
			if subRule, ok := dirRules[relEntryPath]; ok {
				if !subRule.Enabled {
					continue
				}
			}

			// If not forced, respect gitignore for directories
			if !isForced && ignoreChain.Match(entryPath, true) {
				continue
			}

			subChain := ignoreChain.AddIfExists(entryPath)
			if err := walk(entryPath, rootPath, dirRules, currentRule, subChain, writer, signaturesOnly); err != nil {
				return err
			}
			continue
		}

		// -----------------------------
		// FILE PROCESSING LOGIC
		// -----------------------------

		// 4. GITIGNORE CHECK
		// If not forced, check if ignored by git
		if !isForced && ignoreChain.Match(entryPath, false) {
			continue
		}

		// 5. EXTENSION EXCLUDES (Blocklist)
		if !isForced && len(currentRule.ExcludeExtensions) > 0 {
			if contains(currentRule.ExcludeExtensions, ext) {
				continue
			}
		}

		// 6. EXTENSION INCLUDES (Allowlist)
		// If Extensions list is provided, file MUST match one of them (unless forced)
		if !isForced && len(currentRule.Extensions) > 0 {
			if !contains(currentRule.Extensions, ext) {
				continue
			}
		}

		// Write content
		if err := appendFileContent(entryPath, relEntryPath, currentRule, writer, signaturesOnly); err != nil {
			continue
		}
	}
	return nil
}

// shouldAlwaysExclude handles hardcoded exclusions for tool integrity.
func shouldAlwaysExclude(name string) bool {
	return name == ".git" || name == "textify.yaml" || name == "codebase.txt"
}

// checkPatternMatch checks if the file matches any of the glob patterns.
func checkPatternMatch(name, relPath string, patterns []string) bool {
	for _, p := range patterns {
		// Match against filename
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		// Match against relative path
		if matched, _ := filepath.Match(p, relPath); matched {
			return true
		}
		// Direct folder/file path match
		if p == relPath {
			return true
		}
	}
	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// appendFileContent writes the file header and content to the buffer.
func appendFileContent(absPath, relPath string, rule config.DirRule, writer *bufio.Writer, signaturesOnly bool) error {
	// Check for binary content
	isBin, err := fileutil.IsBinary(absPath)
	if err != nil || isBin {
		return nil // Skip binaries silently
	}

	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	separator := strings.Repeat("-", 50)
	fmt.Fprintf(writer, "%s\n", separator)
	fmt.Fprintf(writer, "FILE: %s\n", relPath)
	fmt.Fprintf(writer, "%s\n\n", separator)

	if signaturesOnly || rule.Compact {
		scanner := bufio.NewScanner(file)
		inBody := false
		braceCount := 0

		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if signaturesOnly {
				// Heuristic filter for structs, enums, interfaces, classes, and function signatures
				isSigStart := strings.HasPrefix(trimmed, "func ") ||
					strings.HasPrefix(trimmed, "fn ") ||
					strings.HasPrefix(trimmed, "type ") ||
					strings.HasPrefix(trimmed, "struct ") ||
					strings.HasPrefix(trimmed, "enum ") ||
					strings.HasPrefix(trimmed, "class ") ||
					strings.HasPrefix(trimmed, "interface ") ||
					strings.HasPrefix(trimmed, "trait ") ||
					strings.Contains(trimmed, "func(") ||
					strings.Contains(trimmed, "interface{") ||
					strings.Contains(trimmed, "struct{")

				if isSigStart {
					inBody = true
					braceCount = 0
				}

				if inBody {
					fmt.Fprintln(writer, line)
					for _, ch := range trimmed {
						if ch == '{' {
							braceCount++
						} else if ch == '}' {
							braceCount--
						}
					}
					if braceCount <= 0 && strings.Contains(trimmed, "}") {
						inBody = false
						fmt.Fprintln(writer, "")
					}
				}
			} else {
				if trimmed != "" {
					fmt.Fprintln(writer, line)
				}
			}
		}
	} else {
		if _, err = io.Copy(writer, file); err != nil {
			return err
		}
	}

	fmt.Fprintf(writer, "\n\n")

	modeSuffix := ""
	if signaturesOnly {
		modeSuffix = " (signatures only)"
	} else if rule.Compact {
		modeSuffix = " (compacted)"
	}

	fmt.Printf("Added: %s%s\n", relPath, modeSuffix)
	return nil
}
