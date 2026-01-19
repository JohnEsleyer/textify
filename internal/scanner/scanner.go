package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"textify/internal/fileutil"

	"github.com/monochromegane/go-gitignore"
)

// Config holds the configuration for the Scanner
type Config struct {
	RootPath        string
	OutputFilePath  string   // Absolute path to the output file
	IncludePatterns []string // Glob patterns to force include
}

// Scan walks the directory tree and writes content to the writer
func Scan(config Config, writer io.Writer) error {
	// Initialize GitIgnore matcher
	matcher := getIgnoreMatcher(config.RootPath)

	// Buffered writer for performance
	bufWriter := bufio.NewWriter(writer)
	defer bufWriter.Flush()

	return walk(config.RootPath, config, matcher, bufWriter)
}

// getIgnoreMatcher attempts to load .gitignore from the root path
func getIgnoreMatcher(root string) gitignore.IgnoreMatcher {
	gitignorePath := filepath.Join(root, ".gitignore")
	matcher, err := gitignore.NewGitIgnore(gitignorePath)
	if err != nil {
		// Return empty matcher if no .gitignore found
		return gitignore.NewGitIgnoreFromReader(root, strings.NewReader(""))
	}
	return matcher
}

func walk(fullPath string, config Config, matcher gitignore.IgnoreMatcher, writer *bufio.Writer) error {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())

		// 1. Mandatory Exclusions
		if shouldAlwaysExclude(entry.Name(), entryPath, config.OutputFilePath) {
			continue
		}

		// 2. Check Force Includes (Overrides .gitignore)
		isForced := false
		if len(config.IncludePatterns) > 0 {
			relPath, _ := filepath.Rel(config.RootPath, entryPath)
			if shouldInclude(entry.Name(), relPath, config.IncludePatterns) {
				isForced = true
			}
		}

		// 3. Check .gitignore (Only if not forced)
		if !isForced {
			if matcher.Match(entryPath, entry.IsDir()) {
				continue
			}
		}

		// 4. Process Entry
		if entry.IsDir() {
			if err := walk(entryPath, config, matcher, writer); err != nil {
				return err
			}
		} else {
			if err := appendFileContent(entryPath, config.RootPath, writer); err != nil {
				// Log error but continue scanning
				fmt.Fprintf(os.Stderr, "Skipping %s: %v\n", entry.Name(), err)
			}
		}
	}
	return nil
}

// shouldAlwaysExclude handles hardcoded exclusions like .git and the output file itself
func shouldAlwaysExclude(name, absPath, outputAbsPath string) bool {
	if name == ".git" {
		return true
	}
	// Avoid recursive loop if output file is inside the scanned directory
	if absPath == outputAbsPath {
		return true
	}
	// Hardcoded exclusions to prevent polluting the codebase dump
	if name == "codebase.txt" || name == "textify.txt" {
		return true
	}
	return false
}

// shouldInclude checks if the file matches any of the force-include patterns
func shouldInclude(name, relPath string, patterns []string) bool {
	for _, p := range patterns {
		// Match filename
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		// Match relative path
		if matched, _ := filepath.Match(p, relPath); matched {
			return true
		}
	}
	return false
}

func appendFileContent(absPath, rootPath string, writer *bufio.Writer) error {
	relPath, err := filepath.Rel(rootPath, absPath)
	if err != nil {
		relPath = absPath
	}

	// Check for binary
	isBin, err := fileutil.IsBinary(absPath)
	if err != nil {
		return err
	}
	if isBin {
		return nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write Header
	separator := strings.Repeat("-", 50)
	fmt.Fprintf(writer, "%s\n", separator)
	fmt.Fprintf(writer, "FILE: %s\n", relPath)
	fmt.Fprintf(writer, "%s\n\n", separator)

	// Copy Content
	if _, err = io.Copy(writer, file); err != nil {
		return err
	}

	// Write Footer
	fmt.Fprintf(writer, "\n\n")
	writer.Flush()
	
	fmt.Printf("Added: %s\n", relPath)
	return nil
}
