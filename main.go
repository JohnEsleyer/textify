
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/monochromegane/go-gitignore"
)

// FileConfig represents settings loaded from textify.json
type FileConfig struct {
	IncludeExtensions []string `json:"include_extensions"`
	ExcludeExtensions []string `json:"exclude_extensions"`
}

// AppConfig holds our runtime configuration
type AppConfig struct {
	RootPath          string
	OutputPath        string
	DocsPath          string // Absolute path to the optional 'docs' folder
	Matcher           gitignore.IgnoreMatcher
	IncludeExtensions []string
	ExcludeExtensions []string
}

func main() {
	// 0. Check for Subcommands (e.g., "count")
	if len(os.Args) > 1 && os.Args[1] == "count" {
		targetFile := "codebase.txt"
		if len(os.Args) > 2 {
			targetFile = os.Args[2]
		}
		count, err := countWordsInFile(targetFile)
		if err != nil {
			fmt.Printf("Error counting words: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("File: %s\nWord Count: %d\n", targetFile, count)
		return
	}

	// 1. Parse Flags
	outputFile := flag.String("o", "codebase.txt", "The output text file path")
	dirPath := flag.String("d", ".", "The root directory to scan")
	// UPDATED: Default config name is now textify.json
	configFile := flag.String("c", "textify.json", "Path to configuration file")
	flag.Parse()

	// 2. Load Config File (or create default if missing)
	fileConfig := loadConfigFile(*configFile)

	// 3. Resolve absolute path for accurate matching
	absRoot, err := filepath.Abs(*dirPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// 4. Create the output file
	outFile, err := os.Create(*outputFile)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	
	writer := bufio.NewWriter(outFile)

	// 5. Initialize GitIgnore matcher
	ignoreMatcher := getIgnoreMatcher(absRoot)

	// 6. Start the recursive walk
	fmt.Printf("Textifying %s -> %s\n", absRoot, *outputFile)

	absOutPath, _ := filepath.Abs(*outputFile)
	
	// Calculate absolute path for the docs folder
	absDocsPath := filepath.Join(absRoot, "docs")

	config := AppConfig{
		RootPath:          absRoot,
		OutputPath:        absOutPath,
		DocsPath:          absDocsPath,
		Matcher:           ignoreMatcher,
		IncludeExtensions: fileConfig.IncludeExtensions,
		ExcludeExtensions: fileConfig.ExcludeExtensions,
	}

	err = walk(absRoot, config, writer)
	if err != nil {
		fmt.Printf("Error walking tree: %v\n", err)
	}

	// Flush and Close
	writer.Flush()
	outFile.Close()

	// 7. Calculate Word Count
	totalWords, err := countWordsInFile(*outputFile)
	if err != nil {
		fmt.Printf("Done! (Could not calculate word count: %v)\n", err)
	} else {
		fmt.Printf("\n--------------------------------------------------\n")
		fmt.Printf("Done! Total Word Count: %d\n", totalWords)
		fmt.Printf("--------------------------------------------------\n")
	}
}

// loadConfigFile attempts to read textify.json, or creates it with defaults if missing
func loadConfigFile(path string) FileConfig {
	var config FileConfig
	
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		// File doesn't exist, create default settings
		config.IncludeExtensions = []string{}
		config.ExcludeExtensions = []string{".exe", ".dll", ".so", ".test", ".jpg", ".png", ".gif", ".sum"}
		
		data, _ := json.MarshalIndent(config, "", "  ")
		
		// Write to disk
		if wErr := os.WriteFile(path, data, 0644); wErr == nil {
			fmt.Printf("Created default configuration file: %s\n", path)
		}
		return config
	} else if err != nil {
		return config
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		fmt.Printf("Warning: Could not parse config file: %v\n", err)
	}
	return config
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

// walk recursively processes the tree
func walk(fullPath string, config AppConfig, writer *bufio.Writer) error {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())

		if entry.Name() == ".git" {
			continue
		}

		if entryPath == config.OutputPath {
			continue
		}

		// --- FEATURE: Docs Exception ---
		// We determine if we should skip gitignore checks for this entry.
		// 1. If this specific directory IS the root 'docs' folder.
		// 2. If we are currently walking INSIDE the 'docs' folder (fullPath has prefix DocsPath).
		
		isDocsRoot := (fullPath == config.RootPath && entry.Name() == "docs" && entry.IsDir())
		isInsideDocs := strings.HasPrefix(fullPath, config.DocsPath)
		shouldIgnoreGitRule := isDocsRoot || isInsideDocs

		// Only check gitignore if we are NOT in the special docs context
		if !shouldIgnoreGitRule {
			if config.Matcher.Match(entryPath, entry.IsDir()) {
				continue
			}
		}
		// -------------------------------

		if entry.IsDir() {
			if err := walk(entryPath, config, writer); err != nil {
				return err
			}
		} else {
			if shouldSkipExtension(entry.Name(), config) {
				continue
			}

			if err := appendFileContent(entryPath, config.RootPath, writer); err != nil {
				fmt.Printf("Skipping %s: %v\n", entry.Name(), err)
			}
		}
	}
	return nil
}

func shouldSkipExtension(filename string, config AppConfig) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	for _, exclude := range config.ExcludeExtensions {
		if strings.ToLower(exclude) == ext {
			return true
		}
	}

	if len(config.IncludeExtensions) > 0 {
		found := false
		for _, include := range config.IncludeExtensions {
			if strings.ToLower(include) == ext {
				found = true
				break
			}
		}
		if !found {
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

	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if isBinary(file) {
		return nil 
	}

	file.Seek(0, 0)

	separator := strings.Repeat("-", 50)
	fmt.Fprintf(writer, "%s\n", separator)
	fmt.Fprintf(writer, "FILE: %s\n", relPath)
	fmt.Fprintf(writer, "%s\n\n", separator)

	_, err = io.Copy(writer, file)
	if err != nil {
		return err
	}

	fmt.Fprintf(writer, "\n\n")
	writer.Flush()
	
	fmt.Printf("Added: %s\n", relPath)
	return nil
}

func isBinary(file *os.File) bool {
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false
	}
	
	if n == 0 {
		return false
	}

	content := buffer[:n]
	for _, b := range content {
		if b == 0 {
			return true
		}
	}

	if !utf8.Valid(content) {
		return true
	}

	return false
}

func countWordsInFile(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanWords)

	count := 0
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return count, err
	}

	return count, nil
}