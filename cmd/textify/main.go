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
	case "load":
		runLoad(args)
	case "prompt":
		printPrompt()
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

func runLoad(args []string) {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	formatFlag := fs.String("format", "", "Output format: raw, markdown, or xml")
	clipFlag := fs.Bool("clip", false, "Copy output directly to clipboard")
	clipShortFlag := fs.Bool("c", false, "Copy output directly to clipboard (shorthand)")
	clipInFlag := fs.Bool("clip-in", false, "Read request directly from system clipboard")
	clipInShortFlag := fs.Bool("ci", false, "Read request directly from system clipboard (shorthand)")
	stdoutFlag := fs.Bool("stdout", false, "Stream output directly to standard output")
	noRedactFlag := fs.Bool("no-redact", false, "Disable sensitive secret scrubbing")
	outputFlag := fs.String("o", "", "Override output file name")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	copyToClip := *clipFlag || *clipShortFlag
	fromClip := *clipInFlag || *clipInShortFlag

	var reqData []byte
	positional := fs.Args()

	if fromClip {
		clipText, err := fileutil.ReadFromClipboard()
		if err != nil {
			fmt.Printf("Error reading from clipboard: %v\n", err)
			os.Exit(1)
		}
		reqData = []byte(clipText)
	} else if len(positional) > 0 && positional[0] != "-" {
		requestFile := positional[0]
		data, err := os.ReadFile(requestFile)
		if err != nil {
			fmt.Printf("Error reading request file %s: %v\n", requestFile, err)
			os.Exit(1)
		}
		reqData = data
	} else if len(positional) > 0 && positional[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Printf("Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
		reqData = data
	} else {
		// Auto-detection fallback
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			if data, err := io.ReadAll(os.Stdin); err == nil && len(data) > 0 {
				reqData = data
			}
		}
		if len(reqData) == 0 {
			if data, err := os.ReadFile("request.txt"); err == nil && len(data) > 0 {
				reqData = data
				fmt.Println("Auto-detected request from request.txt")
			}
		}
		if len(reqData) == 0 {
			if clipText, err := fileutil.ReadFromClipboard(); err == nil {
				if strings.Contains(clipText, "<file") || strings.Contains(clipText, "<request") || strings.Contains(clipText, "<slice") {
					reqData = []byte(clipText)
					fmt.Println("Auto-detected request from system clipboard")
				}
			}
		}
		if len(reqData) == 0 {
			fmt.Println("Error: No request source provided.")
			fmt.Println("Usage: textify load [request.txt] [flags]")
			fmt.Println("       textify load -ci -c   (instant clipboard-to-clipboard)")
			fmt.Println("       cat request.txt | textify load - -c")
			os.Exit(1)
		}
	}

	requests := scanner.ParseRequest(string(reqData))
	if len(requests) == 0 {
		fmt.Println("No valid file requests found in the input.")
		fmt.Println("Ensure the request contains <request><file path=\"...\" /></request> or bullet lists.")
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		cfg = &config.Config{OutputFile: "codebase.txt"}
	}

	if *outputFlag != "" {
		cfg.OutputFile = *outputFlag
	}

	opts := scanner.ScanOptions{
		Format:        *formatFlag,
		RedactSecrets: !*noRedactFlag,
		Stdout:        *stdoutFlag,
	}

	var outputBuf bytes.Buffer
	if !*stdoutFlag {
		fmt.Printf("Loading %d file request(s) from %s...\n", len(requests), requestFile)
	}

	result, err := scanner.LoadRequests(cwd, cfg, requests, &outputBuf, opts)
	if err != nil {
		fmt.Printf("Error processing requests: %v\n", err)
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
		fmt.Printf("Error writing output: %v\n", err)
		os.Exit(1)
	}

	if copyToClip {
		if clipErr := fileutil.CopyToClipboard(outputBuf.String()); clipErr == nil {
			fmt.Println("✔ Output copied to system clipboard!")
		} else {
			fmt.Printf("Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}

	fmt.Printf("\n✔ Done! Detailed sections saved to: %s\n", cfg.OutputFile)
	fmt.Printf("--------------------------------------------------\n")
	fmt.Printf("Sections loaded: %d\n", result.FilesCount)
	fmt.Printf("Total size:      %.2f KB\n", float64(result.TotalBytes)/1024.0)
	fmt.Printf("Approx. tokens:  ~%d tokens\n", result.ApproxTokens)
	if result.RedactedSecrets > 0 {
		fmt.Printf("⚠️ Redacted secrets: %d sensitive token(s) sanitized\n", result.RedactedSecrets)
	}
	fmt.Printf("--------------------------------------------------\n")
}

func printPrompt() {
	fmt.Println(`Copy and paste this prompt to the LLM alongside your signatures:
--------------------------------------------------------------------------------
Here is the project outline and signatures with line numbers.
Analyze them. If you need full implementations of specific files,
line ranges, keyword searches, or symbols, respond using this format:

<request>
  <file path="path/to/file.ext" />
  <file path="path/to/file.ext" lines="50-120" />
  <file path="path/to/file.ext" lines="10-25, 80-100" />
  <file path="path/to/file.ext" lines="100:" />
  <file path="path/to/file.ext" search="function_or_keyword" context="5" />
  <file path="path/to/file.ext" symbols="FunctionName,TypeName" />
  <file glob="pkg/compiler/*_test.go" />
</request>

Markdown lists are also supported:
- path/to/file.ext:50-120
- path/to/file.ext (search: "keyword")

Otherwise, proceed directly with your solution.
--------------------------------------------------------------------------------`)
}

func printHelp() {
	fmt.Println("Textify - Turn your codebase into AI-ready text")
	fmt.Println("\nUsage:")
	fmt.Println("  textify init                 Scans folders and generates textify.yaml")
	fmt.Println("  textify scan                 Detects new folders and updates textify.yaml")
	fmt.Println("  textify start [flags]        Generates output file based on configuration")
	fmt.Println("  textify signatures [flags]   Extracts structs, types, interfaces, and function signatures")
	fmt.Println("  textify diff [flags]         Only includes modified and untracked git files")
	fmt.Println("  textify load [file] [flags]  Extracts requested files, slices, searches, or symbols")
	fmt.Println("  textify prompt               Prints the instruction prompt to paste to the LLM")
	fmt.Println("  textify version              Shows current version")
	fmt.Println("\nFlags for load:")
	fmt.Println("  --clip-in, -ci               Read request directly from system clipboard")
	fmt.Println("  --clip, -c                   Copy output slices directly to system clipboard")
	fmt.Println("  --format <raw|markdown|xml>  Output format structure (default raw)")
	fmt.Println("  --stdout                     Stream output to stdout (useful for piping)")
	fmt.Println("  --no-redact                  Disable sensitive secret scrubbing")
	fmt.Println("  -o <file>                    Override destination filename")
}
