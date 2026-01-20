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

		// 1. Determine the Rule for this specific entry
		//    We check if there is an explicit override in the YAML.
		activeRule := currentRule // Default to inheriting parent settings
		explicitRule, hasExplicit := dirRules[relEntryPath]
		
		if hasExplicit {
			activeRule = explicitRule
		}

		// 2. Check Force Includes
		//    (If it's force included, we skip gitignore and extension checks)
		isForced := checkInclude(entry.Name(), relEntryPath, activeRule.Include)

		if entry.IsDir() {
			// A. Explicit Disabled Check
			//    If YAML explicitly says 'enabled: false', skip it regardless of gitignore
			if hasExplicit && !activeRule.Enabled {
				continue
			}

			// B. Gitignore Check
			//    If NOT forced, check if git ignores this folder.
			if !isForced && matcher.Match(entryPath, true) {
				continue
			}

			// Recurse
			if err := walk(entryPath, rootPath, dirRules, activeRule, matcher, writer); err != nil {
				return err
			}
			continue
		}

		// 3. File Processing
		
		// A. Gitignore Check
		if !isForced && matcher.Match(entryPath, false) {
			continue
		}

		// B. Extension Check
		//    If extensions are defined, the file must match.
		if !isForced && len(activeRule.Extensions) > 0 {
			ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			if !contains(activeRule.Extensions, ext) {
				continue
			}
		}

		// C. Write
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
