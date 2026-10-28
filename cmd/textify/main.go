package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
	"github.com/JohnEsleyer/textify/internal/scanner"
)

const version = "0.4.0"
const configFile = "textify.yaml"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "init":
		runInit()
	case "scan":
		runScan()
	case "start":
		runStart(args, false, false)
	case "signatures":
		runStart(args, true, false)
	case "diff":
		runStart(args, false, true)
	case "version", "-v", "--version":
		fmt.Printf("textify %s\n", version)
	case "help", "-h", "--help":
		printHelp()
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

	cfg, err := config.Discover(cwd, nil)
	if err != nil {
		fmt.Printf("Error scanning directories: %v\n", err)
		os.Exit(1)
	}

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

	existingCfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("Error loading %s: %v\n", configFile, err)
		return
	}

	fmt.Println("Rescanning project for new directories...")

	newCfg, err := config.Discover(cwd, existingCfg)
	if err != nil {
		fmt.Printf("Error scanning: %v\n", err)
		os.Exit(1)
	}

	if err := newCfg.Save(configFile); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✔ Updated %s. Total rules: %d\n", configFile, len(newCfg.Dirs))
}

func runStart(args []string, signaturesOnly, diffOnly bool) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	formatFlag := fs.String("format", "", "Output format: raw, markdown, or xml")
	clipFlag := fs.Bool("clip", false, "Copy output directly to clipboard")
	clipShortFlag := fs.Bool("c", false, "Copy output directly to clipboard (shorthand)")
	stdoutFlag := fs.Bool("stdout", false, "Stream output directly to standard output")
	noTreeFlag := fs.Bool("no-tree", false, "Disable prepending the directory tree")
	noRedactFlag := fs.Bool("no-redact", false, "Do not auto-redact secrets")
	diffFlag := fs.Bool("diff", diffOnly, "Only include files modified in git status")
	sigFlag := fs.Bool("signatures", signaturesOnly, "Extract signatures only")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	copyToClip := *clipFlag || *clipShortFlag
	signatures := *sigFlag
	diff := *diffFlag

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

	opts := scanner.ScanOptions{
		SignaturesOnly: signatures,
		DiffOnly:       diff,
		Format:         *formatFlag,
		RedactSecrets:  !*noRedactFlag,
		Tree:           !*noTreeFlag,
		Stdout:         *stdoutFlag,
	}

	var outputBuf bytes.Buffer

	if !*stdoutFlag {
		modeDesc := "full codebase"
		if signatures {
			modeDesc = "signatures & types"
		}
		if diff {
			modeDesc += " (git changed only)"
		}
		fmt.Printf("Textifying project [%s] using %s...\n", modeDesc, configFile)
	}

	result, err := scanner.Scan(cwd, cfg, &outputBuf, opts)
	if err != nil {
		fmt.Printf("Scan error: %v\n", err)
		os.Exit(1)
	}

	if *stdoutFlag {
		os.Stdout.Write(outputBuf.Bytes())
		return
	}

	outPath := cfg.OutputFile
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(cwd, outPath)
	}

	if err := os.WriteFile(outPath, outputBuf.Bytes(), 0644); err != nil {
		fmt.Printf("Error saving output file: %v\n", err)
		os.Exit(1)
	}

	if copyToClip {
		if clipErr := fileutil.CopyToClipboard(outputBuf.String()); clipErr == nil {
			fmt.Println("✔ Output copied to system clipboard!")
		} else {
			fmt.Printf("Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}

	fmt.Printf("\n✔ Done! Output saved to: %s\n", cfg.OutputFile)
	fmt.Printf("--------------------------------------------------\n")
	fmt.Printf("Files included:  %d\n", result.FilesCount)
	fmt.Printf("Total size:      %.2f KB\n", float64(result.TotalBytes)/1024.0)
	fmt.Printf("Approx. tokens:  ~%d tokens\n", result.ApproxTokens)
	if result.RedactedSecrets > 0 {
		fmt.Printf("⚠️ Redacted secrets: %d sensitive token(s) sanitized\n", result.RedactedSecrets)
	}
	fmt.Printf("--------------------------------------------------\n")
}

func printHelp() {
	fmt.Println("Textify - Turn your codebase into AI-ready text")
	fmt.Println("\nUsage:")
	fmt.Println("  textify init                Scans folders and generates textify.yaml")
	fmt.Println("  textify scan                Detects new folders and updates textify.yaml")
	fmt.Println("  textify start [flags]       Generates output file based on configuration")
	fmt.Println("  textify signatures [flags]  Extracts structs, types, interfaces, and function signatures")
	fmt.Println("  textify diff [flags]        Only includes modified and untracked git files")
	fmt.Println("  textify version             Shows current version")
	fmt.Println("\nFlags for start, signatures, and diff:")
	fmt.Println("  --clip, -c                  Copy output directly to clipboard")
	fmt.Println("  --format <raw|markdown|xml> Output format structure (default raw)")
	fmt.Println("  --stdout                    Stream output to stdout (useful for piping)")
	fmt.Println("  --no-tree                   Omit ASCII directory tree from the header")
	fmt.Println("  --no-redact                 Disable automatic sensitive secret scrubbing")
}
