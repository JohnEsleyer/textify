package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/scanner"
)

const configFile = "textify.yaml"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "init":
		runInit()
	case "scan":
		runScan() // New Command
	case "start":
		runStart()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func runInit() {
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("Error: %s already exists. Use 'textify scan' to update it.\n", configFile)
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	fmt.Println("Initializing and scanning project structure...")

	// Run Discovery with no existing config
	cfg, err := config.Discover(cwd, nil)
	if err != nil {
		fmt.Printf("Error scanning directories: %v\n", err)
		os.Exit(1)
	}

	// Save
	if err := cfg.Save(configFile); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✔ Generated %s with %d directory rules.\n", configFile, len(cfg.Dirs))
}

func runScan() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	// 1. Load existing
	existingCfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configFile, err)
		return
	}

	fmt.Println("Rescanning project for new directories...")

	// 2. Run Discovery (Merging into existing)
	newCfg, err := config.Discover(cwd, existingCfg)
	if err != nil {
		fmt.Printf("Error scanning: %v\n", err)
		os.Exit(1)
	}

	// 3. Save
	if err := newCfg.Save(configFile); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✔ Updated %s. Total rules: %d\n", configFile, len(newCfg.Dirs))
}

func runStart() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configFile, err)
		os.Exit(1)
	}

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
	fmt.Println("  textify init   Scans folders and generates textify.yaml")
	fmt.Println("  textify scan   Detects new folders and updates textify.yaml")
	fmt.Println("  textify start  Generates the output file based on config")
}
