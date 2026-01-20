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
func Scan(rootPath string, cfg *config.Config, writer io.Writer) error {
	matcher := getIgnoreMatcher(rootPath)
	bufWriter := bufio.NewWriter(writer)
	defer bufWriter.Flush()

	// Initial rule (Root ".")
	rootRule, ok := cfg.Dirs["."]
	if !ok {
		// If root is missing from config, default to enabled but no extensions
		rootRule = config.DirRule{Enabled: true, Extensions: []string{}}
	}

	return walk(rootPath, rootPath, cfg.Dirs, rootRule, matcher, bufWriter)
}

func walk(
	fullPath string,
	rootPath string,
	dirRules map[string]config.DirRule,
	currentRule config.DirRule,
	matcher gitignore.IgnoreMatcher,
	writer *bufio.Writer,
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

		if shouldAlwaysExclude(entry.Name()) {
			continue
		}

		// Priority Check: Force Include
		isForced := checkInclude(entry.Name(), relEntryPath, currentRule.Include)

		if entry.IsDir() {
            // Check if this specific SUBDIRECTORY has a rule that disables it
            // before we even recurse or check gitignore.
            if subRule, ok := dirRules[relEntryPath]; ok {
                if !subRule.Enabled {
                    continue 
                }
            }

			if !isForced && matcher.Match(entryPath, true) {
				continue
			}
			
			if err := walk(entryPath, rootPath, dirRules, currentRule, matcher, writer); err != nil {
				return err
			}
			continue
		}

		// Process File
		if !isForced && matcher.Match(entryPath, false) {
			continue
		}

		if !isForced && len(currentRule.Extensions) > 0 {
			ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			if !contains(currentRule.Extensions, ext) {
				continue
			}
		}

		if err := appendFileContent(entryPath, relEntryPath, writer); err != nil {
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
	return name == ".git" || name == "textify.yaml" || name == "codebase.txt"
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
