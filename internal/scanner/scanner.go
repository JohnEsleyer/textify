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

	"github.com/monochromegane/go-gitignore"
)

// Scan initiates the directory walk based on the provided configuration.
// It writes the formatted content of valid files to the provided writer.
func Scan(rootPath string, cfg *config.Config, writer io.Writer) error {
	matcher := getIgnoreMatcher(rootPath)
	bufWriter := bufio.NewWriter(writer)
	defer bufWriter.Flush()

	// Initial rule (Root ".")
	rootRule, ok := cfg.Dirs["."]
	if !ok {
		// Fallback: If "." is missing, assume strict mode or empty defaults.
		rootRule = config.DirRule{Extensions: []string{}}
	}

	return walk(rootPath, rootPath, cfg.Dirs, rootRule, matcher, bufWriter)
}

// walk recursively scans directories.
// currentRule represents the active rule set (Extensions/Includes) for the current directory.
func walk(
	fullPath string,
	rootPath string,
	dirRules map[string]config.DirRule,
	currentRule config.DirRule,
	matcher gitignore.IgnoreMatcher,
	writer *bufio.Writer,
) error {

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	// Calculate relative path for the directory we are currently IN
	relDir, _ := filepath.Rel(rootPath, fullPath)
	if relDir == "." {
		relDir = "."
	} else {
		relDir = filepath.ToSlash(relDir) // Ensure standard forward slashes for map keys
	}

	// Context Switch:
	// If the TOML has a specific entry for this folder, switch to that rule.
	// Otherwise, continue using 'currentRule' (inherited from parent).
	if specificRule, exists := dirRules[relDir]; exists {
		currentRule = specificRule
	}

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())
		relEntryPath, _ := filepath.Rel(rootPath, entryPath)
		relEntryPath = filepath.ToSlash(relEntryPath)

		// 1. Mandatory Exclusions (e.g. .git, output file, config file)
		if shouldAlwaysExclude(entry.Name()) {
			continue
		}

		// 2. Priority Check: Force Include
		// If it's in the 'include' list, we skip extension checks and gitignore.
		isForced := checkInclude(entry.Name(), relEntryPath, currentRule.Include)

		// 3. Process Directory
		if entry.IsDir() {
			// If not forced, respect gitignore
			if !isForced && matcher.Match(entryPath, true) {
				continue
			}
			// Recurse
			if err := walk(entryPath, rootPath, dirRules, currentRule, matcher, writer); err != nil {
				return err
			}
			continue
		}

		// 4. Process File
		// A. Check gitignore (if not forced)
		if !isForced && matcher.Match(entryPath, false) {
			continue
		}

		// B. Check Extensions (if not forced and extensions are defined)
		// If Extensions list is present, the file MUST match one of them.
		if !isForced && len(currentRule.Extensions) > 0 {
			ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			if !contains(currentRule.Extensions, ext) {
				continue
			}
		}

		// C. Write Content
		if err := appendFileContent(entryPath, relEntryPath, writer); err != nil {
			// Silently skip files we cannot read (e.g., permissions errors)
			// to avoid halting the entire scan.
			continue
		}
	}
	return nil
}

// getIgnoreMatcher attempts to load .gitignore from the root path.
func getIgnoreMatcher(root string) gitignore.IgnoreMatcher {
	gitignorePath := filepath.Join(root, ".gitignore")
	matcher, err := gitignore.NewGitIgnore(gitignorePath)
	if err != nil {
		return gitignore.NewGitIgnoreFromReader(root, strings.NewReader(""))
	}
	return matcher
}

// shouldAlwaysExclude handles hardcoded exclusions for tool integrity.
func shouldAlwaysExclude(name string) bool {
	return name == ".git" || name == "textify.toml" || name == "codebase.txt"
}

// checkInclude checks if the file matches any of the force-include patterns.
func checkInclude(name, relPath string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		if matched, _ := filepath.Match(p, relPath); matched {
			return true
		}
		// Direct folder match
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
func appendFileContent(absPath, relPath string, writer *bufio.Writer) error {
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

	if _, err = io.Copy(writer, file); err != nil {
		return err
	}
	fmt.Fprintf(writer, "\n\n")

	fmt.Printf("Added: %s\n", relPath)
	return nil
}
