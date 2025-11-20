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

// FileConfig represents settings loaded from config.json
type FileConfig struct {
	IncludeExtensions []string `json:"include_extensions"`
	ExcludeExtensions []string `json:"exclude_extensions"`
}

// AppConfig holds our runtime configuration
type AppConfig struct {
	RootPath          string
	OutputPath        string
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
	configFile := flag.String("c", "config.json", "Path to configuration file")
	flag.Parse()

	// 2. Load Config File (if it exists)
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
	// We don't defer Close() here because we want to close it before counting words at the end
	
	// Use a buffered writer for better performance
	writer := bufio.NewWriter(outFile)

	// 5. Initialize GitIgnore matcher
	ignoreMatcher := getIgnoreMatcher(absRoot)

	// 6. Start the recursive walk
	fmt.Printf("Textifying %s -> %s\n", absRoot, *outputFile)

	// Determine absolute path of output file to ensure we don't include it in itself
	absOutPath, _ := filepath.Abs(*outputFile)

	config := AppConfig{
		RootPath:          absRoot,
		OutputPath:        absOutPath,
		Matcher:           ignoreMatcher,
		IncludeExtensions: fileConfig.IncludeExtensions,
		ExcludeExtensions: fileConfig.ExcludeExtensions,
	}

	err = walk(absRoot, config, writer)
	if err != nil {
		fmt.Printf("Error walking tree: %v\n", err)
	}

	// Flush and Close to ensure all data is on disk before counting
	writer.Flush()
	outFile.Close()

	// 7. Calculate Word Count of the generated file
	totalWords, err := countWordsInFile(*outputFile)
	if err != nil {
		fmt.Printf("Done! (Could not calculate word count: %v)\n", err)
	} else {
		fmt.Printf("\n--------------------------------------------------\n")
		fmt.Printf("Done! Total Word Count: %d\n", totalWords)
		fmt.Printf("--------------------------------------------------\n")
	}
}

// loadConfigFile attempts to read config.json
func loadConfigFile(path string) FileConfig {
	var config FileConfig
	
	file, err := os.Open(path)
	if err != nil {
		// Config file is optional, return empty defaults
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

		// 1. Skip the .git directory
		if entry.Name() == ".git" {
			continue
		}

		// 2. Skip the output file itself if it's inside the directory
		if entryPath == config.OutputPath {
			continue
		}

		// 3. Check .gitignore
		if config.Matcher.Match(entryPath, entry.IsDir()) {
			continue
		}

		if entry.IsDir() {
			// Recurse
			if err := walk(entryPath, config, writer); err != nil {
				return err
			}
		} else {
			// 4. Check Extensions (Include/Exclude)
			if shouldSkipExtension(entry.Name(), config) {
				continue
			}

			// Process File
			if err := appendFileContent(entryPath, config.RootPath, writer); err != nil {
				fmt.Printf("Skipping %s: %v\n", entry.Name(), err)
			}
		}
	}
	return nil
}

// shouldSkipExtension determines if a file should be ignored based on config
func shouldSkipExtension(filename string, config AppConfig) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	// Check Excludes
	for _, exclude := range config.ExcludeExtensions {
		if strings.ToLower(exclude) == ext {
			return true
		}
	}

	// Check Includes (if defined, strictly enforce them)
	if len(config.IncludeExtensions) > 0 {
		found := false
		for _, include := range config.IncludeExtensions {
			if strings.ToLower(include) == ext {
				found = true
				break
			}
		}
		// If we have an include list, and this file isn't in it, skip it
		if !found {
			return true
		}
	}

	return false
}

// appendFileContent reads the file and writes it to the output writer with headers
func appendFileContent(absPath, rootPath string, writer *bufio.Writer) error {
	// Calculate relative path for cleaner display in the text file
	relPath, err := filepath.Rel(rootPath, absPath)
	if err != nil {
		relPath = absPath
	}

	// Open the file
	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Check if binary
	if isBinary(file) {
		return nil 
	}

	// Reset file pointer to beginning after binary check
	file.Seek(0, 0)

	// Write Header
	separator := strings.Repeat("-", 50)
	fmt.Fprintf(writer, "%s\n", separator)
	fmt.Fprintf(writer, "FILE: %s\n", relPath)
	fmt.Fprintf(writer, "%s\n\n", separator)

	// Copy Content
	_, err = io.Copy(writer, file)
	if err != nil {
		return err
	}

	// Write Footer/Padding
	fmt.Fprintf(writer, "\n\n")
	
	// Flush occasionally
	writer.Flush()
	
	fmt.Printf("Added: %s\n", relPath)
	return nil
}

// isBinary reads the first 512 bytes to determine if the file is likely binary
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

// countWordsInFile counts the number of words in a file
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