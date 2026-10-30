package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
	"github.com/JohnEsleyer/textify/internal/parser"
	"github.com/JohnEsleyer/textify/internal/patcher"
	"github.com/JohnEsleyer/textify/internal/scanner"
)

const version = "0.7.0"
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
	case "patch", "apply", "patcher":
		runPatch(args)
	case "grep", "search", "find":
		runGrep(args)
	case "def", "definition":
		runDef(args)
	case "trace", "flow":
		runTrace(args)
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

func runGrep(args []string) {
	fs := flag.NewFlagSet("grep", flag.ExitOnError)
	contextFlag := fs.Int("C", 2, "Lines of context around matching lines")
	clipFlag := fs.Bool("clip", false, "Copy output directly to clipboard")
	clipShortFlag := fs.Bool("c", false, "Copy output directly to clipboard (shorthand)")
	globFlag := fs.String("glob", "", "Filter files by glob pattern (e.g. *.go)")
	stdoutFlag := fs.Bool("stdout", false, "Stream output directly to standard output")
	noRedactFlag := fs.Bool("no-redact", false, "Disable sensitive secret scrubbing")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	positional := fs.Args()
	if len(positional) == 0 {
		fmt.Println("Error: Missing search pattern.")
		fmt.Println("Usage: textify grep <pattern> [-C context] [-c] [--glob pattern]")
		os.Exit(1)
	}

	pattern := positional[0]
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		cfg = &config.Config{OutputFile: "codebase.txt"}
	}

	req := scanner.FileRequest{
		Query:   pattern,
		Context: *contextFlag,
		Glob:    *globFlag,
	}

	var outputBuf bytes.Buffer
	if !*stdoutFlag {
		fmt.Printf("Searching for %q across project...\n", pattern)
	}

	opts := scanner.ScanOptions{
		Format:        "raw",
		RedactSecrets: !*noRedactFlag,
		Stdout:        *stdoutFlag,
	}

	result, err := scanner.LoadRequests(cwd, cfg, []scanner.FileRequest{req}, &outputBuf, opts)
	if err != nil {
		fmt.Printf("Grep error: %v\n", err)
		os.Exit(1)
	}

	if *stdoutFlag || (!*clipFlag && !*clipShortFlag) {
		os.Stdout.Write(outputBuf.Bytes())
	}

	if *clipFlag || *clipShortFlag {
		if clipErr := fileutil.CopyToClipboard(outputBuf.String()); clipErr == nil {
			fmt.Println("\n✔ Grep results copied to system clipboard!")
		} else {
			fmt.Printf("Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}

	if !*stdoutFlag {
		fmt.Printf("\nMatched in %d file(s) (~%d tokens).\n", result.FilesCount, result.ApproxTokens)
	}
}

func runDef(args []string) {
	fs := flag.NewFlagSet("def", flag.ExitOnError)
	langFlag := fs.String("lang", "all", "Language filter (go, ts, or all)")
	depthFlag := fs.Int("depth", 5, "Maximum trace depth when --trace is used")
	traceFlag := fs.Bool("trace", false, "Also trace execution call flow")
	clipFlag := fs.Bool("clip", false, "Copy definition directly to clipboard")
	clipShortFlag := fs.Bool("c", false, "Copy definition directly to clipboard (shorthand)")

	valueFlags := map[string]bool{"--lang": true, "-lang": true, "--depth": true, "-depth": true}
	if err := fs.Parse(normalizeFlagArgs(args, valueFlags)); err != nil {
		os.Exit(1)
	}

	if fs.NArg() == 0 {
		fmt.Println("Usage: textify def <symbol_or_struct> [--lang go,ts] [--trace] [--depth 5] [-c]")
		os.Exit(1)
	}

	symbol := fs.Arg(0)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Indexing codebase and parsing AST for symbol %q...\n", symbol)
	idx, err := parser.BuildIndex(cwd, parser.ParseLanguages(*langFlag))
	if err != nil {
		fmt.Printf("Error indexing: %v\n", err)
		os.Exit(1)
	}

	var output strings.Builder
	if *traceFlag {
		output.WriteString(idx.TraceFlow(symbol, *depthFlag))
		output.WriteString("\n")
	}

	syms := idx.FindDefinition(symbol, "")
	if len(syms) == 0 {
		fmt.Fprintf(os.Stderr, "Symbol %q not found in indexed files.\n", symbol)
		os.Exit(1)
	}

	for _, s := range syms {
		output.WriteString(fmt.Sprintf("--- %s %s [%s:%d-%d] ---\n", s.Kind, s.FullName(), s.File, s.StartLine, s.EndLine))
		for offset, line := range strings.Split(s.Body, "\n") {
			output.WriteString(fmt.Sprintf("%5d | %s\n", s.StartLine+offset, line))
		}
		output.WriteString("\n")
	}

	fmt.Print(output.String())

	if *clipFlag || *clipShortFlag {
		if clipErr := fileutil.CopyToClipboard(output.String()); clipErr == nil {
			fmt.Fprintln(os.Stderr, "✔ Definition copied to clipboard!")
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}
}

func runTrace(args []string) {
	fs := flag.NewFlagSet("trace", flag.ExitOnError)
	langFlag := fs.String("lang", "all", "Language filter (go, ts, or all)")
	depthFlag := fs.Int("depth", 5, "Maximum call chain depth")
	clipFlag := fs.Bool("clip", false, "Copy trace directly to clipboard")
	clipShortFlag := fs.Bool("c", false, "Copy trace directly to clipboard (shorthand)")

	valueFlags := map[string]bool{"--lang": true, "-lang": true, "--depth": true, "-depth": true}
	if err := fs.Parse(normalizeFlagArgs(args, valueFlags)); err != nil {
		os.Exit(1)
	}

	if fs.NArg() == 0 {
		fmt.Println("Usage: textify trace <function_or_struct> [--lang go,ts] [--depth 5] [-c]")
		os.Exit(1)
	}

	symbol := fs.Arg(0)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Building AST execution flow for %q...\n", symbol)
	idx, err := parser.BuildIndex(cwd, parser.ParseLanguages(*langFlag))
	if err != nil {
		fmt.Printf("Error building AST index: %v\n", err)
		os.Exit(1)
	}

	matches := idx.FindDefinition(symbol, "")
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "Symbol %q not found in indexed files.\n", symbol)
		os.Exit(1)
	}

	trace := idx.TraceFlow(symbol, *depthFlag)
	fmt.Print(trace)

	if *clipFlag || *clipShortFlag {
		if clipErr := fileutil.CopyToClipboard(trace); clipErr == nil {
			fmt.Fprintln(os.Stderr, "✔ Execution trace copied to clipboard!")
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}
}

func normalizeFlagArgs(args []string, valueFlags map[string]bool) []string {
	var flags []string
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if valueFlags[arg] && i+1 < len(args) {
				flags = append(flags, arg+"="+args[i+1])
				i++
			} else {
				flags = append(flags, arg)
			}
			continue
		}
		positional = append(positional, arg)
	}

	return append(flags, positional...)
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
				if strings.Contains(clipText, "<file") || strings.Contains(clipText, "<request") || strings.Contains(clipText, "<grep") || strings.Contains(clipText, "<def") || strings.Contains(clipText, "<definition") {
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
		fmt.Println("No valid file or grep requests found in the input.")
		fmt.Println("Ensure the request contains <request><file path=\"...\" /></request>, <grep ... />, <def ... />, or bullet lists.")
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
		fmt.Printf("Loading %d file/grep request(s)...\n", len(requests))
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
	fmt.Printf("Files included:  %d\n", result.FilesCount)
	fmt.Printf("Total size:      %.2f KB\n", float64(result.TotalBytes)/1024.0)
	fmt.Printf("Approx. tokens:  ~%d tokens\n", result.ApproxTokens)
	if result.RedactedSecrets > 0 {
		fmt.Printf("⚠️ Redacted secrets: %d sensitive token(s) sanitized\n", result.RedactedSecrets)
	}
	fmt.Printf("--------------------------------------------------\n")
}

func runPatch(args []string) {
	fs := flag.NewFlagSet("patch", flag.ExitOnError)
	dryRunFlag := fs.Bool("dry-run", false, "Preview changes without modifying files on disk")
	dryRunShortFlag := fs.Bool("n", false, "Preview changes without modifying files on disk (shorthand)")
	clipInFlag := fs.Bool("clip-in", false, "Read patch directly from system clipboard")
	clipInShortFlag := fs.Bool("ci", false, "Read patch directly from system clipboard (shorthand)")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	dryRun := *dryRunFlag || *dryRunShortFlag
	fromClip := *clipInFlag || *clipInShortFlag

	var patchData []byte
	positional := fs.Args()

	if fromClip {
		clipText, err := fileutil.ReadFromClipboard()
		if err != nil {
			fmt.Printf("Error reading from clipboard: %v\n", err)
			os.Exit(1)
		}
		patchData = []byte(clipText)
	} else if len(positional) > 0 && positional[0] != "-" {
		patchFile := positional[0]
		data, err := os.ReadFile(patchFile)
		if err != nil {
			fmt.Printf("Error reading patch file %s: %v\n", patchFile, err)
			os.Exit(1)
		}
		patchData = data
	} else if len(positional) > 0 && positional[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Printf("Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
		patchData = data
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			if data, err := io.ReadAll(os.Stdin); err == nil && len(data) > 0 {
				patchData = data
			}
		}
		if len(patchData) == 0 {
			for _, candidate := range []string{"patch.txt", "changes.txt", "patcher.txt"} {
				if data, err := os.ReadFile(candidate); err == nil && len(data) > 0 {
					patchData = data
					fmt.Printf("Auto-detected patch from %s\n", candidate)
					break
				}
			}
		}
		if len(patchData) == 0 {
			if clipText, err := fileutil.ReadFromClipboard(); err == nil {
				if strings.Contains(clipText, "<file") || strings.Contains(clipText, "<replace") || strings.Contains(clipText, "<delete") {
					patchData = []byte(clipText)
					fmt.Println("Auto-detected patch from system clipboard")
				}
			}
		}
		if len(patchData) == 0 {
			fmt.Println("Error: No patch source provided.")
			fmt.Println("Usage: textify patch [patch-file.txt] [--dry-run]")
			fmt.Println("       textify patch -ci              (apply patch directly from clipboard)")
			fmt.Println("       cat changes.txt | textify patch -")
			os.Exit(1)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	result, err := patcher.Apply(cwd, string(patchData), patcher.PatchOptions{
		DryRun: dryRun,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Patch validation failed. Zero files were modified.\n")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(result.Changes) == 0 {
		fmt.Println("Notice: No patch directives found in input.")
		return
	}

	if dryRun {
		fmt.Println("✔ [DRY RUN] All patch directives validated successfully.")
		fmt.Println("The following changes would be applied:")
		for _, ch := range result.Changes {
			fmt.Printf("  %s %-6s %s (%s)\n", changeIcon(ch.Type), ch.Type, ch.Path, ch.Description)
		}
		fmt.Printf("\nTotal: %d change(s) verified. Zero files modified on disk.\n", len(result.Changes))
		return
	}

	fmt.Println("✔ Patch applied successfully!")
	for _, ch := range result.Changes {
		fmt.Printf("  %s %-6s %s (%s)\n", changeIcon(ch.Type), ch.Type, ch.Path, ch.Description)
	}
	fmt.Printf("\nTotal: %d change(s) applied.\n", len(result.Changes))
}

func changeIcon(t patcher.FileChangeType) string {
	switch t {
	case patcher.ChangeCreate:
		return "+"
	case patcher.ChangeUpdate:
		return "~"
	case patcher.ChangeDelete:
		return "-"
	}
	return "•"
}

func printPrompt() {
	fmt.Println(`Copy and paste this prompt to the LLM alongside your signatures:
--------------------------------------------------------------------------------
Here is the project outline and signatures with line numbers.
Analyze them. If you need implementations of specific logic, DO NOT GUESS LINE NUMBERS.
Use targeted requests:

<request>
  <!-- Search keyword across all files with context -->
  <grep pattern="search_term" context="10" />

  <!-- Find code inside a file by keyword or anchor -->
  <file path="path/to/file.ext" find="keyword_or_statement" context="15" />

  <!-- Extract a block from start pattern to end pattern -->
  <file path="path/to/file.ext" from="case *MyType:" to="case *" />

  <!-- Extract an entire function or struct by symbol name -->
  <file path="path/to/file.ext" symbol="function_or_struct_name" />

  <!-- AST-exact definition (Go/TypeScript) with precise line numbers -->
  <def name="handleRequest" />
  <def name="UserConfig" />

  <!-- AST-exact definition plus execution / dependency flow -->
  <def name="handleRequest" trace="true" />
  <file path="path/to/file.ts" def="UserService" trace="true" />

  <!-- Explicit line range (only if already known from signatures above) -->
  <file path="path/to/file.ext" lines="50-120" />
</request>

When providing solutions or code edits, output patch directives using <file>, <replace>, or <delete>:

1. Create or overwrite a file:
<file path="path/to/file.ext">
... full content ...
</file>

2. Targeted region update (PREFER symbol or <search> with ...):
<replace path="path/to/file.ext" symbol="functionOrStructName">
<with>
... replacement declaration ...
</with>
</replace>

<replace path="path/to/file.ext">
<search>
prefix code
...
lines to replace
...
suffix code
</search>
<with>
prefix code
...
new replacement lines
...
suffix code
</with>
</replace>

3. Anchor insertions or pattern ranges:
<replace path="path/to/file.ext" after="anchor line">
<with>
new code
</with>
</replace>

<replace path="path/to/file.ext" from="start_pattern" to="end_pattern">
<with>
replacement block
</with>
</replace>

4. Delete a file:
<delete path="path/to/obsolete.ext" />

CRITICAL RULES FOR PATCH DIRECTIVES:
- NEVER put '<' or '>' inside XML attributes (use <search> block instead).
- Prefer symbol="..." over multiple partial replacements in the same function.
- Every <replace> MUST have a target (symbol, search block, from/to, after, before, or lines).
- Use '...' wildcards in <search> blocks to match surrounding unchanged boilerplate.
--------------------------------------------------------------------------------`)
}

func printHelp() {
	fmt.Println("Textify - Turn your codebase into AI-ready text & safely apply patches")
	fmt.Println("\nUsage:")
	fmt.Println("  textify init                 Scans folders and generates textify.yaml")
	fmt.Println("  textify scan                 Detects new folders and updates textify.yaml")
	fmt.Println("  textify start [flags]        Generates output file based on configuration")
	fmt.Println("  textify signatures [flags]   Extracts structs, types, interfaces, and function signatures")
	fmt.Println("  textify diff [flags]         Only includes modified and untracked git files")
	fmt.Println("  textify grep <pattern> [opts]Searches project for pattern and outputs matching context")
	fmt.Println("  textify def <symbol> [flags] Extracts an AST-exact function/struct definition (Go/TypeScript)")
	fmt.Println("  textify trace <symbol> [flags] Traces a function or struct dependency flow")
	fmt.Println("  textify load [file] [flags]  Extracts requested files, slices, searches, or symbols")
	fmt.Println("  textify patch [file] [flags] Safely applies multi-file LLM patches (atomic VFS)")
	fmt.Println("  textify prompt               Prints the instruction prompt to paste to the LLM")
	fmt.Println("  textify version              Shows current version")
	fmt.Println("\nFlags for patch:")
	fmt.Println("  --dry-run, -n                Validate and preview changes without modifying files")
	fmt.Println("  --clip-in, -ci               Read patch directly from system clipboard")
	fmt.Println("\nFlags for def/trace:")
	fmt.Println("  --lang <go,ts,all>           Languages to index (default all)")
	fmt.Println("  --depth <num>                Maximum trace depth (default 5)")
	fmt.Println("  --trace                      Include execution flow with def")
	fmt.Println("  --clip, -c                   Copy output to clipboard")
	fmt.Println("\nFlags for grep:")
	fmt.Println("  -C <num>                     Lines of context around match (default 2)")
	fmt.Println("  --clip, -c                   Copy grep results to clipboard")
	fmt.Println("  --glob <pattern>             Filter files by glob (e.g. *.go)")
	fmt.Println("\nFlags for load:")
	fmt.Println("  --clip-in, -ci               Read request directly from system clipboard")
	fmt.Println("  --clip, -c                   Copy output slices directly to system clipboard")
	fmt.Println("  --format <raw|markdown|xml>  Output format structure (default raw)")
	fmt.Println("  --stdout                     Stream output to stdout (useful for piping)")
	fmt.Println("  --no-redact                  Disable sensitive secret scrubbing")
	fmt.Println("  -o <file>                    Override destination filename")
}
