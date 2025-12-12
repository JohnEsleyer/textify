package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/monochromegane/go-gitignore"
)

// FileConfig represents settings loaded from textify.json
type FileConfig struct {
	IncludeExtensions []string `json:"include_extensions"`
	ExcludePaths      []string `json:"exclude_paths"`
	IncludeFolders    []string `json:"include_folders"`
}

// AppConfig holds our runtime configuration
type AppConfig struct {
	RootPath          string
	OutputPath        string
	DocsPath          string
	Matcher           gitignore.IgnoreMatcher
	IncludeExtensions []string
	ExcludePaths      []string
	IncludeFolders    []string
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

	// 3. Resolve absolute paths
	absRoot, err := filepath.Abs(*dirPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	absOutPath, _ := filepath.Abs(*outputFile)
	absDocsPath := filepath.Join(absRoot, "docs")

	// 4. Initialize GitIgnore matcher
	ignoreMatcher := getIgnoreMatcher(absRoot)

	// 5. Setup Config Object
	config := AppConfig{
		RootPath:          absRoot,
		OutputPath:        absOutPath,
		DocsPath:          absDocsPath,
		Matcher:           ignoreMatcher,
		IncludeExtensions: fileConfig.IncludeExtensions,
		ExcludePaths:      fileConfig.ExcludePaths,
		IncludeFolders:    fileConfig.IncludeFolders,
	}

	fmt.Printf("Textifying %s -> %s\n", absRoot, *outputFile)

	// 6. Create output file
	outFile, err := os.Create(*outputFile)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	writer := bufio.NewWriter(outFile)

	// 7. Generate and Write Directory Tree
	fmt.Println("Generating Project Tree...")
	treeStr, err := generateDirectoryTree(absRoot, "", config)
	if err != nil {
		fmt.Printf("Warning: Could not generate tree: %v\n", err)
	} else {
		fmt.Fprintf(writer, "PROJECT STRUCTURE:\n")
		fmt.Fprintf(writer, "==================\n")
		fmt.Fprintf(writer, "%s\n", treeStr)
		fmt.Fprintf(writer, "==================\n\n")
		fmt.Fprintf(writer, "FILE CONTENTS:\n\n")
	}

	// 8. Walk and Append Content
	fmt.Println("Processing Files...")
	err = walkAndAppend(absRoot, config, writer)
	if err != nil {
		fmt.Printf("Error walking tree: %v\n", err)
	}

	writer.Flush()

	// 9. Calculate Word Count
	totalWords, err := countWordsInFile(*outputFile)
	if err != nil {
		fmt.Printf("Done! (Could not calculate word count: %v)\n", err)
	} else {
		fmt.Printf("\n--------------------------------------------------\n")
		fmt.Printf("Done! Total Word Count: %d\n", totalWords)
		fmt.Printf("--------------------------------------------------\n")
	}
}

// --- Logic ---

// shouldSkip determines if a file or folder should be ignored based on all rules.
func shouldSkip(path string, info os.DirEntry, config AppConfig) bool {
	name := info.Name()
    relPath, err := filepath.Rel(config.RootPath, path)
    if err != nil {
        relPath = path 
    }
    
	// 1. Skip .git and the output file itself
	if name == ".git" {
		return true
	}
	if path == config.OutputPath {
		return true
	}

	// 2. Check Manual Exclusions (exclude_paths in json)
	if err == nil {
		if shouldExcludePath(relPath, config.ExcludePaths) {
			return true
		}
	}

	// 3. Docs Exception Logic
	isDocsRoot := (config.RootPath == filepath.Dir(path) && name == "docs" && info.IsDir())
	isInsideDocs := strings.HasPrefix(path, config.DocsPath)
	shouldIgnoreGitRule := isDocsRoot || isInsideDocs

	// 4. Check GitIgnore
	if !shouldIgnoreGitRule {
		if config.Matcher.Match(path, info.IsDir()) {
			return true
		}
	}

    // 5. Check Folder Inclusion (Applies to both directories and files within them)
    if len(config.IncludeFolders) > 0 {
        if !shouldIncludeFolder(relPath, config.IncludeFolders) {
            // If folder whitelist is active and the path doesn't match an inclusion rule, skip it.
            // Exclude root and special docs folder from this skip check.
            if relPath != "." && !isDocsRoot && !isInsideDocs {
                return true
            }
        }
    }


	// 6. Check Extensions (Files only)
	if !info.IsDir() {
		if shouldSkipExtension(name, config) {
			return true
		}
	}

	return false
}

func generateDirectoryTree(currentPath string, prefix string, config AppConfig) (string, error) {
	var sb strings.Builder
	
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return "", err
	}

	// Filter entries first to know count for tree formatting
	var validEntries []os.DirEntry
	for _, entry := range entries {
		entryPath := filepath.Join(currentPath, entry.Name())
		if !shouldSkip(entryPath, entry, config) {
			validEntries = append(validEntries, entry)
		}
	}

	// Sort: Directories first, then files, both alphabetical
	sort.Slice(validEntries, func(i, j int) bool {
		d1, d2 := validEntries[i], validEntries[j]
		if d1.IsDir() != d2.IsDir() {
			return d1.IsDir() // Directories first
		}
		return d1.Name() < d2.Name()
	})

	for i, entry := range validEntries {
		isLast := i == len(validEntries)-1
		entryPath := filepath.Join(currentPath, entry.Name())

		connector := "├── "
		if isLast {
			connector = "└── "
		}

		sb.WriteString(prefix + connector + entry.Name() + "\n")

		if entry.IsDir() {
			newPrefix := prefix
			if isLast {
				newPrefix += "    "
			} else {
				newPrefix += "│   "
			}
			subTree, _ := generateDirectoryTree(entryPath, newPrefix, config)
			sb.WriteString(subTree)
		}
	}

	return sb.String(), nil
}

func walkAndAppend(fullPath string, config AppConfig, writer *bufio.Writer) error {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())

		// Use the centralized skip logic
		if shouldSkip(entryPath, entry, config) {
			continue
		}

		if entry.IsDir() {
			if err := walkAndAppend(entryPath, config, writer); err != nil {
				return err
			}
		} else {
			if err := appendFileContent(entryPath, config.RootPath, writer); err != nil {
				// Only report hard errors, suppressing "binary file detected" message
				if !strings.Contains(err.Error(), "binary file detected") {
					fmt.Printf("Skipping reading %s: %v\n", entry.Name(), err)
				}
			}
		}
	}
	return nil
}

// --- Helpers ---

func loadConfigFile(path string) FileConfig {
	var config FileConfig

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		// Create default settings (pure whitelists, empty means include all)
		config.IncludeExtensions = []string{}
		config.ExcludePaths = []string{}
		config.IncludeFolders = []string{}

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

func shouldExcludePath(relPath string, excludes []string) bool {
	relPath = filepath.Clean(relPath)
	for _, exclude := range excludes {
		cleanExclude := filepath.Clean(exclude)
		// Check for exact file/folder match OR if the path is inside the excluded folder
		if relPath == cleanExclude || strings.HasPrefix(relPath, cleanExclude+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// shouldIncludeFolder checks if the relative path falls under any whitelisted folder.
func shouldIncludeFolder(relPath string, includes []string) bool {
    if len(includes) == 0 {
        return true 
    }
    
    // Check if the path itself or its parent folder is included
    for _, include := range includes {
        cleanInclude := filepath.Clean(include)
        
        // Exact match for the directory itself
        if relPath == cleanInclude {
            return true
        }
        
        // Path is inside the included folder: e.g., include="src", path="src/main.go"
        if strings.HasPrefix(relPath, cleanInclude + string(filepath.Separator)) {
            return true
        }
    }
    
    // Also explicitly allow the root directory itself to start the scan
    if relPath == "." {
        return true
    }
    
    return false
}


func shouldSkipExtension(filename string, config AppConfig) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	// 1. Check Explicit Includes (Whitelist)
	if len(config.IncludeExtensions) > 0 {
		found := false
		for _, include := range config.IncludeExtensions {
			if strings.ToLower(include) == ext {
				found = true
				break
			}
		}
		// If whitelist is active and no match was found, skip it.
		if !found {
			return true 
		}
	}
	
	// If whitelist is empty, or if we found a match, DO NOT skip.
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
		return fmt.Errorf("binary file detected")
	}

	// Reset file pointer after binary check
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