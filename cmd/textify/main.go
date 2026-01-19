// Package main is the entry point for the Textify CLI tool.
// It handles command-line arguments to initialize configuration
// or start the codebase scanning process.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/scanner"
)

const configFile = "textify.toml"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "init":
		runInit()
	case "start":
		runStart()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

// runInit scans the current directory and generates a default textify.toml file.
func runInit() {
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("Error: %s already exists in this directory.\n", configFile)
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current working directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Scanning directory structure to generate configuration...")

	// Create base config
	cfg := config.DefaultConfig()

	// Scan top-level directories to pre-populate the TOML with suggested defaults
	entries, _ := os.ReadDir(cwd)
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != ".git" {
			// Add a default entry for subdirectories.
			// Empty Extensions implies inheritance or custom setup by the user.
			cfg.Dirs[entry.Name()] = config.DirRule{
				Extensions: []string{"go", "md", "txt", "js", "ts", "json"},
				Include:    []string{},
			}
		}
	}

	if err := cfg.Save(configFile); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✔ Initialization complete. Created %s\n", configFile)
	fmt.Println("  1. Edit the file to configure extensions and inclusions.")
	fmt.Println("  2. Run 'textify start' to generate your output file.")
}

// runStart reads the configuration and executes the scan.
func runStart() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Load Config
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configFile, err)
		fmt.Println("Hint: Did you run 'textify init'?")
		os.Exit(1)
	}

	// Resolve output path
	outPath := cfg.OutputFile
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(cwd, outPath)
	}

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	fmt.Printf("Textifying project using %s...\n", configFile)
	
	if err := scanner.Scan(cwd, cfg, f); err != nil {
		fmt.Printf("Scan error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✔ Done! Output saved to: %s\n", cfg.OutputFile)
}

func printHelp() {
	fmt.Println("Textify - Turn your codebase into AI-ready text")
	fmt.Println("\nUsage:")
	fmt.Println("  textify init   Scans current folder and generates textify.toml")
	fmt.Println("  textify start  Reads textify.toml and generates the output file")
}
