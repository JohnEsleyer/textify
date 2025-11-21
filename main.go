
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
	ExcludePaths      []string `json:"exclude_paths"` // New field for specific files/folders
}

// AppConfig holds our runtime configuration
type AppConfig struct {
	RootPath          string
	OutputPath        string
	DocsPath          string
	Matcher           gitignore.IgnoreMatcher
	IncludeExtensions []string
	ExcludeExtensions []string
	ExcludePaths      []string
}

func main() {
	// 0. Check for Subcommands
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
	configFile := flag.String("c", "textify.json", "Path to configuration file")
	flag.Parse()

	// 2. Load Config File
	fileConfig := loadConfigFile(*configFile)

	// 3. Resolve absolute path
	absRoot, err := filepath.Abs(*dirPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// 4. Create output file
	outFile, err := os.Create(*outputFile)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	
	writer := bufio.NewWriter(outFile)

	// 5. Initialize GitIgnore matcher
	ignoreMatcher := getIgnoreMatcher(absRoot)

	// 6. Start recursive walk
	fmt.Printf("Textifying %s -> %s\n", absRoot, *outputFile)

	absOutPath, _ := filepath.Abs(*outputFile)
	absDocsPath := filepath.Join(absRoot, "docs")

	config := AppConfig{
		RootPath:          absRoot,
		OutputPath:        absOutPath,
		DocsPath:          absDocsPath,
		Matcher:           ignoreMatcher,
		IncludeExtensions: fileConfig.IncludeExtensions,
		ExcludeExtensions: fileConfig.ExcludeExtensions,
		ExcludePaths:      fileConfig.ExcludePaths,
	}

	err = walk(absRoot, config, writer)
	if err != nil {
		fmt.Printf("Error walking tree: %v\n", err)
	}

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

func loadConfigFile(path string) FileConfig {
	var config FileConfig
	
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		// Create default settings
		config.IncludeExtensions = []string{}
		config.ExcludeExtensions = []string{".exe", ".dll", ".so", ".test", ".jpg", ".png", ".gif", ".sum"}
		config.ExcludePaths = []string{} // Default empty list
		
		data, _ := json.MarshalIndent(config, "", "  ")
		
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

func getIgnoreMatcher(root string) gitignore.IgnoreMatcher {
	gitignorePath := filepath.Join(root, ".gitignore")
	matcher, err := gitignore.NewGitIgnore(gitignorePath)
	if err != nil {
		return gitignore.NewGitIgnoreFromReader(root, strings.NewReader(""))
	}
	return matcher
}

func walk(fullPath string, config AppConfig, writer *bufio.Writer) error {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())

		// 1. Skip .git and output file
		if entry.Name() == ".git" {
			continue
		}
		if entryPath == config.OutputPath {
			continue
		}

		// 2. Check Manual Exclusions (NEW)
		// Calculate path relative to root to match config settings
		relPath, err := filepath.Rel(config.RootPath, entryPath)
		if err == nil {
			if shouldExcludePath(relPath, config.ExcludePaths) {
				continue
			}
		}

		// 3. Docs Exception Logic
		isDocsRoot := (fullPath == config.RootPath && entry.Name() == "docs" && entry.IsDir())
		isInsideDocs := strings.HasPrefix(fullPath, config.DocsPath)
		shouldIgnoreGitRule := isDocsRoot || isInsideDocs

		// 4. Check GitIgnore (unless in docs)
		if !shouldIgnoreGitRule {
			if config.Matcher.Match(entryPath, entry.IsDir()) {
				continue
			}
		}

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

// shouldExcludePath checks if the current relative path matches the blocklist
func shouldExcludePath(relPath string, excludes []string) bool {
	relPath = filepath.Clean(relPath)
	for _, exclude := range excludes {
		if relPath == filepath.Clean(exclude) {
			return true
		}
	}
	return false
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

