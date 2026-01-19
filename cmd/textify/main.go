package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"textify/internal/scanner"
)

func main() {
	// 1. Parse Flags
	outputFile := flag.String("o", "codebase.txt", "The output text file path")
	dirPath := flag.String("d", ".", "The root directory to scan")
	include := flag.String("i", "", "Comma-separated list of patterns to force include (e.g. '*.env,secret.conf')")
	flag.Parse()

	// 2. Resolve absolute paths
	absRoot, err := filepath.Abs(*dirPath)
	if err != nil {
		fmt.Printf("Error resolving root path: %v\n", err)
		os.Exit(1)
	}

	absOut, err := filepath.Abs(*outputFile)
	if err != nil {
		fmt.Printf("Error resolving output path: %v\n", err)
		os.Exit(1)
	}

	// 3. Create output file
	f, err := os.Create(absOut)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// 4. Parse include patterns
	var includePatterns []string
	if *include != "" {
		parts := strings.Split(*include, ",")
		for _, p := range parts {
			includePatterns = append(includePatterns, strings.TrimSpace(p))
		}
	}

	// 5. Configure and Run Scanner
	config := scanner.Config{
		RootPath:        absRoot,
		OutputFilePath:  absOut,
		IncludePatterns: includePatterns,
	}

	fmt.Printf("Textifying %s -> %s\n", absRoot, *outputFile)
	if len(includePatterns) > 0 {
		fmt.Printf("Force including: %v\n", includePatterns)
	}

	if err := scanner.Scan(config, f); err != nil {
		fmt.Printf("Error during scan: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Done!")
}
