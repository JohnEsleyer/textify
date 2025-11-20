package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/monochromegane/go-gitignore"
)

// Config holds our runtime configuration
type Config struct {
	RootPath   string
	OutputPath string
	Matcher    gitignore.IgnoreMatcher
	// Whitelist is a map of allowed extensions (e.g. ".go" -> true).
	// If nil, all extensions are allowed.
	Whitelist map[string]bool
}

func main() {
	// 1. Parse Flags
	outputFile := flag.String("o", "codebase.txt", "The output text file path")
	dirPath := flag.String("d", ".", "The root directory to scan")
	flag.Parse()

	// 2. Resolve absolute path for accurate matching
	absRoot, err := filepath.Abs(*dirPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// 3. Create the output file
	outFile, err := os.Create(*outputFile)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	// Use a buffered writer for better performance
	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	fmt.Printf("Textifying %s -> %s\n", absRoot, *outputFile)

	// 4. Run lstree and write to top of file
	if err := runLstree(absRoot, writer); err != nil {
		fmt.Printf("Warning: could not run 'lstree': %v\n", err)
		fmt.Fprintln(writer, "[Tree view unavailable - verify lstree is installed]")
	}
	
	// Add separation between tree and content
	fmt.Fprintln(writer, "\n"+strings.Repeat("=", 50))
	fmt.Fprintln(writer, "FILE CONTENTS")
	fmt.Fprintln(writer, strings.Repeat("=", 50)+"\n")
	writer.Flush()

	// 5. Load .textify-config if it exists
	whitelist, err := loadExtensionWhitelist(absRoot)
	if err != nil {
		// Non-critical error (e.g. file permission), just log
		if !os.IsNotExist(err) {
			fmt.Printf("Warning reading .textify-config: %v\n", err)
		}
		// If error or not exist, whitelist remains nil (allow all)
	} else {
		fmt.Println("Extension whitelist applied via .textify-config")
	}

	// 6. Initialize GitIgnore matcher
	ignoreMatcher := getIgnoreMatcher(absRoot)

	// Determine absolute path of output file to ensure we don't include it in itself
	absOutPath, _ := filepath.Abs(*outputFile)

	config := Config{
		RootPath:   absRoot,
		OutputPath: absOutPath,
		Matcher:    ignoreMatcher,
		Whitelist:  whitelist,
	}

	// 7. Start the recursive walk
	err = walk(absRoot, config, writer)
	if err != nil {
		fmt.Printf("Error walking tree: %v\n", err)
	}

	fmt.Println("Done!")
}

// runLstree executes the lstree command and pipes output to the writer
func runLstree(root string, w io.Writer) error {
	cmd := exec.Command("lstree", root)
	cmd.Stdout = w
	// Capture stderr to print to console if needed, or ignore
	cmd.Stderr = os.Stderr 
	
	fmt.Println("Generating project tree...")
	return cmd.Run()
}

// loadExtensionWhitelist reads .textify-config and returns a map of allowed extensions.
// Returns nil if the file does not exist.
func loadExtensionWhitelist(root string) (map[string]bool, error) {
	configPath := filepath.Join(root, ".textify-config")
	
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	whitelist := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Normalize extension: ensure it has a dot prefix
		// e.g., "go" -> ".go", ".txt" -> ".txt"
		if !strings.HasPrefix(line, ".") {
			line = "." + line
		}
		whitelist[line] = true
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return whitelist, nil
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
func walk(fullPath string, config Config, writer *bufio.Writer) error {
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

		// 2. Skip the output file itself
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
			// 4. Check Whitelist (if active)
			if config.Whitelist != nil {
				ext := filepath.Ext(entry.Name())
				// If extension is not in whitelist, skip
				// Note: If file has no extension (e.g. Dockerfile, Makefile), ext is empty string.
				// The user must explicitly add "." or "" to config to include extensionless files 
				// depending on how they want to handle them, or specific names.
				// For this implementation, we strictly check the extension.
				if !config.Whitelist[ext] {
					continue
				}
			}

			// Process File
			if err := appendFileContent(entryPath, config.RootPath, writer); err != nil {
				fmt.Printf("Skipping %s: %v\n", entry.Name(), err)
			}
		}
	}
	return nil
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