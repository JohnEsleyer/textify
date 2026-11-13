package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
	"github.com/JohnEsleyer/textify/internal/parser"
	"github.com/JohnEsleyer/textify/internal/patcher"
	"github.com/JohnEsleyer/textify/internal/scanner"
)

const version = "0.12.0"

const defaultPrompt = `You are interacting with a codebase managed by Textify.

CRITICAL INSTRUCTION FOR CONTEXT RETRIEVAL:
Minimize consecutive round trips. Aim to finish all inspections in AT MOST 1 request before providing your solution.
Do NOT request symbols or files one by one if they are part of a call chain.

When inspecting code, use structural XML directives:
- Deep Execution Trace: <trace symbol="FuncName" depth="2" />
  (Automatically extracts the root function, bodies of called functions, and referenced structs in 1 shot)
- Switch-case branch: <select path="path/to/file.ext" fn="funcName" case="caseValue" />
- Struct methods: <select struct="StructName" methods="Method1,Method2" />
- Cross-references: <xref symbol="SymbolName" action="read|write|call|all" />
- Composite bundles: <feature-context lang="go"><target-type name="T"/><entrypoint symbol="S" depth="2"/></feature-context>
- Project-wide search: <grep pattern="keyword" [context="10"] [glob="*.go"] />
- Load specific lines: <file path="path/to/file.ext" lines="10-50,80-120" />
- Clean patch-ready anchors: Add for-patch="true" to any directive to strip line numbers

When returning code modifications, format changes using atomic patch directives:
- Create/overwrite file:
  ` + "\x3cfile" + ` path="path/to/file.ext">
  file contents here
  ` + "\x3c/file" + `>

- Replace with search block (supports '...' wildcards):
  ` + "\x3creplace" + ` path="path/to/file.ext" [within="functionName"]>
  ` + "\x3csearch" + `>
  code to find
  ` + "\x3c/search" + `>
  ` + "\x3cwith" + `>
  replacement code
  ` + "\x3c/with" + `>
  ` + "\x3c/replace" + `>

- Replace switch-case block via AST:
  ` + "\x3creplace" + ` path="path/to/file.ext" fn="funcName" case="caseTarget">
  ` + "\x3cwith" + `>
      case target:
          new implementation
  ` + "\x3c/with" + `>
  ` + "\x3c/replace" + `>

- Replace AST symbol definition:
  ` + "\x3creplace" + ` path="path/to/file.ext" symbol="SymbolName">
  ` + "\x3cwith" + `>
  func SymbolName() { ... }
  ` + "\x3c/with" + `>
  ` + "\x3c/replace" + `>

- Anchor insertions:
  ` + "\x3creplace" + ` path="path/to/file.ext" after="anchor line">
  ` + "\x3cwith" + `>
  inserted code
  ` + "\x3c/with" + `>
  ` + "\x3c/replace" + `>

- Delete file:
  ` + "\x3cdelete" + ` path="path/to/file.ext" />
`

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "init":
		runInit(args)
	case "scan":
		runScan(args)
	case "check", "validate":
		runCheck(args)
	case "start":
		runStart(args)
	case "signatures", "sig":
		runSignatures(args)
	case "tokens", "folders", "dirs", "sizes", "size":
		runTokens(args)
	case "diff":
		runDiff(args)
	case "grep":
		runGrep(args)
	case "def":
		runDef(args)
	case "trace":
		runTrace(args)
	case "load":
		runLoad(args)
	case "patch":
		runPatch(args)
	case "prompt":
		runPrompt(args)
	case "version", "--version", "-v":
		fmt.Printf("textify version %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %q\nRun 'textify help' for usage.\n", command)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`Textify ⚡ - High-performance codebase context converter & patch engine (v%s)

Usage:
  textify <command> [arguments...] [flags]

Core Commands:
  init                     Scan directory tree and generate textify.yaml
  scan                     Detect new directories and update textify.yaml
  check, validate          Validate textify.yaml and inspect active directory rules
  start                    Bundle project codebase into context file
  signatures, sig          Export types, structs, interfaces, and function headers
  tokens, dirs             List included folders and estimated token sizes (largest to smallest)
  diff                     Bundle only modified, staged, or untracked Git files
  grep <pattern>           Search codebase for pattern with context lines
  def <symbol>             Extract AST definition of function/struct/class (Go/TS/Java/Kotlin/Rust)
  trace <symbol>           Trace function execution call flow or type dependencies
  load [file]              Extract requested files, slices, searches, or AST symbols
  patch [file]             Safely apply multi-file LLM patches (atomic two-pass VFS)
  prompt                   Print prompt instructions for the LLM
  version                  Print current version
  help                     Show help and command usage

Common Flags:
  -c, --clip               Copy command output directly to system clipboard
  -ci, --clip-in           Read input directly from system clipboard (load, patch)
  -s, --stdout             Write output to stdout instead of default file
  -r, --redact             Redact sensitive secrets (API keys, private keys, tokens)
  -f, --format             Output format: tags, raw, markdown (default "tags")
  --config                 Path to textify config file (default "textify.yaml")

`, version)
}

func normalizeFlagArgs(args []string, valueFlags map[string]bool) []string {
	var flags []string
	var positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Flags with values (e.g. -f tags, --config file.yaml)
		if valueFlags[arg] && i+1 < len(args) {
			flags = append(flags, arg, args[i+1])
			i++
			continue
		}

		if strings.HasPrefix(arg, "-") {
			// Handle bundled boolean flags like -ci, -cs, etc.
			if !strings.HasPrefix(arg, "--") && len(arg) > 2 && !strings.Contains(arg, "=") && arg != "-ci" {
				chars := arg[1:]
				allLetters := true
				for _, ch := range chars {
					if ch < 'a' || ch > 'z' {
						allLetters = false
						break
					}
				}
				if allLetters {
					for _, ch := range chars {
						flags = append(flags, fmt.Sprintf("-%c", ch))
					}
					continue
				}
			}
			flags = append(flags, arg)
		} else {
			positionals = append(positionals, arg)
		}
	}

	return append(flags, positionals...)
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	_ = fs.Parse(args)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(*configPath); err == nil {
		fmt.Fprintf(os.Stderr, "Configuration file %q already exists. Use 'textify scan' to update.\n", *configPath)
		os.Exit(1)
	}

	cfg, err := config.Discover(cwd, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during project discovery: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Save(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	fmt.Printf("✔ Generated %s with %d directories detected.\n", *configPath, len(cfg.Dirs))
}

func runScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	_ = fs.Parse(args)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	var existingCfg *config.Config
	if _, err := os.Stat(*configPath); err == nil {
		loaded, loadErr := config.Load(*configPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", loadErr)
			os.Exit(1)
		}
		existingCfg = loaded
	}

	cfg, err := config.Discover(cwd, existingCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during discovery scan: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Save(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	fmt.Printf("✔ Updated %s (%d directories configured).\n", *configPath, len(cfg.Dirs))
}

func runStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	formatFlag := fs.String("format", "", "Output format (tags, raw, markdown)")
	formatShort := fs.String("f", "", "Output format (shorthand)")
	stdoutFlag := fs.Bool("stdout", false, "Write bundle to stdout")
	stdoutShort := fs.Bool("s", false, "Write bundle to stdout (shorthand)")
	clipFlag := fs.Bool("clip", false, "Copy bundle to clipboard")
	clipShort := fs.Bool("c", false, "Copy bundle to clipboard (shorthand)")
	redactFlag := fs.Bool("redact", false, "Redact sensitive tokens and secrets")
	redactShort := fs.Bool("r", false, "Redact sensitive tokens and secrets (shorthand)")
	treeFlag := fs.Bool("tree", false, "Prepend directory tree")
	fullFlag := fs.Bool("full", false, "Force full code extraction for all files (ignore signatures groups)")
	sigGroupsFlag := fs.String("signatures-groups", "", "Comma-separated group names for signatures only")
	sigGroupsShort := fs.String("sig-groups", "", "Comma-separated group names for signatures only (shorthand)")
	fullGroupsFlag := fs.String("full-groups", "", "Comma-separated group names for full code")
	fullGroupsShort := fs.String("full-code-groups", "", "Comma-separated group names for full code (alias)")
	groupFlag := fs.String("group", "", "Only include directories/files in specific group(s)")
	groupsFlag := fs.String("groups", "", "Only include directories/files in specific group(s) (alias)")

	valueFlags := map[string]bool{
		"--config": true, "-config": true, "--format": true, "-format": true, "-f": true,
		"--signatures-groups": true, "-signatures-groups": true,
		"--sig-groups": true, "-sig-groups": true,
		"--full-groups": true, "-full-groups": true,
		"--full-code-groups": true, "-full-code-groups": true,
		"--group": true, "-group": true, "--groups": true, "-groups": true,
	}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	executeScan(scanCmdParams{
		configPath:       *configPath,
		format:           firstNonEmpty(*formatFlag, *formatShort),
		stdout:           *stdoutFlag || *stdoutShort,
		clip:             *clipFlag || *clipShort,
		redact:           *redactFlag || *redactShort,
		tree:             *treeFlag,
		signaturesOnly:   false,
		diffOnly:         false,
		forceFull:        *fullFlag,
		signaturesGroups: splitCommaList(firstNonEmpty(*sigGroupsFlag, *sigGroupsShort)),
		fullCodeGroups:   splitCommaList(firstNonEmpty(*fullGroupsFlag, *fullGroupsShort)),
		targetGroups:     splitCommaList(firstNonEmpty(*groupFlag, *groupsFlag)),
	})
}

func runSignatures(args []string) {
	fs := flag.NewFlagSet("signatures", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	formatFlag := fs.String("format", "", "Output format (tags, raw, markdown)")
	formatShort := fs.String("f", "", "Output format (shorthand)")
	stdoutFlag := fs.Bool("stdout", false, "Write signatures to stdout")
	stdoutShort := fs.Bool("s", false, "Write signatures to stdout (shorthand)")
	clipFlag := fs.Bool("clip", false, "Copy signatures to clipboard")
	clipShort := fs.Bool("c", false, "Copy signatures to clipboard (shorthand)")
	redactFlag := fs.Bool("redact", false, "Redact sensitive tokens and secrets")
	redactShort := fs.Bool("r", false, "Redact sensitive tokens and secrets (shorthand)")
	astOverview := fs.Bool("ast", false, "Use AST parser index overview instead of line scan")
	fullGroupsFlag := fs.String("full-groups", "", "Comma-separated group names for full code")
	fullGroupsShort := fs.String("full-code-groups", "", "Comma-separated group names for full code (shorthand)")
	groupFlag := fs.String("group", "", "Only include directories/files in specific group(s)")
	groupsFlag := fs.String("groups", "", "Only include directories/files in specific group(s) (alias)")

	valueFlags := map[string]bool{
		"--config": true, "-config": true, "--format": true, "-format": true, "-f": true,
		"--full-groups": true, "-full-groups": true,
		"--full-code-groups": true, "-full-code-groups": true,
		"--group": true, "-group": true, "--groups": true, "-groups": true,
	}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	if *astOverview {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
			os.Exit(1)
		}
		idx, err := parser.BuildIndex(cwd, parser.ParseLanguages("all"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error building AST index: %v\n", err)
			os.Exit(1)
		}
		output := idx.GenerateSignaturesOverview()
		if *redactFlag || *redactShort {
			output, _ = fileutil.RedactSecrets(output)
		}
		handleTextOutput(output, "Signatures", *stdoutFlag || *stdoutShort, *clipFlag || *clipShort)
		return
	}

	executeScan(scanCmdParams{
		configPath:     *configPath,
		format:         firstNonEmpty(*formatFlag, *formatShort),
		stdout:         *stdoutFlag || *stdoutShort,
		clip:           *clipFlag || *clipShort,
		redact:         *redactFlag || *redactShort,
		tree:           false,
		signaturesOnly: true,
		diffOnly:       false,
		fullCodeGroups: splitCommaList(firstNonEmpty(*fullGroupsFlag, *fullGroupsShort)),
		targetGroups:   splitCommaList(firstNonEmpty(*groupFlag, *groupsFlag)),
	})
}

func runDiff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	formatFlag := fs.String("format", "", "Output format (tags, raw, markdown)")
	formatShort := fs.String("f", "", "Output format (shorthand)")
	stdoutFlag := fs.Bool("stdout", false, "Write diff bundle to stdout")
	stdoutShort := fs.Bool("s", false, "Write diff bundle to stdout (shorthand)")
	clipFlag := fs.Bool("clip", false, "Copy diff bundle to clipboard")
	clipShort := fs.Bool("c", false, "Copy diff bundle to clipboard (shorthand)")
	redactFlag := fs.Bool("redact", false, "Redact sensitive tokens and secrets")
	redactShort := fs.Bool("r", false, "Redact sensitive tokens and secrets (shorthand)")
	groupFlag := fs.String("group", "", "Only include directories/files in specific group(s)")
	groupsFlag := fs.String("groups", "", "Only include directories/files in specific group(s) (alias)")

	valueFlags := map[string]bool{
		"--config": true, "-config": true, "--format": true, "-format": true, "-f": true,
		"--group": true, "-group": true, "--groups": true, "-groups": true,
	}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	executeScan(scanCmdParams{
		configPath:     *configPath,
		format:         firstNonEmpty(*formatFlag, *formatShort),
		stdout:         *stdoutFlag || *stdoutShort,
		clip:           *clipFlag || *clipShort,
		redact:         *redactFlag || *redactShort,
		tree:           false,
		signaturesOnly: false,
		diffOnly:       true,
		targetGroups:   splitCommaList(firstNonEmpty(*groupFlag, *groupsFlag)),
	})
}

func runGrep(args []string) {
	fs := flag.NewFlagSet("grep", flag.ExitOnError)
	contextFlag := fs.Int("context", 10, "Context lines surrounding matches")
	contextShort := fs.Int("C", 10, "Context lines surrounding matches (shorthand)")
	globFlag := fs.String("glob", "*", "File glob filter pattern")
	clipFlag := fs.Bool("clip", false, "Copy grep results to clipboard")
	clipShort := fs.Bool("c", false, "Copy grep results to clipboard (shorthand)")
	forPatch := fs.Bool("for-patch", false, "Output clean patch-ready search blocks")

	valueFlags := map[string]bool{"--context": true, "-context": true, "-C": true, "--glob": true, "-glob": true}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	if fs.NArg() == 0 {
		fmt.Println("Usage: textify grep <pattern> [-C 10] [--glob '*.go'] [-c]")
		os.Exit(1)
	}

	pattern := fs.Arg(0)
	ctxLines := *contextFlag
	if ctxLines == 10 && *contextShort != 10 {
		ctxLines = *contextShort
	}

	req := scanner.FileRequest{
		Query:    pattern,
		Context:  ctxLines,
		Glob:     *globFlag,
		ForPatch: *forPatch,
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	cfg := config.DefaultConfig()
	_, err = scanner.LoadRequests(cwd, &cfg, []scanner.FileRequest{req}, &buf, scanner.ScanOptions{
		Format: "tags",
		Stdout: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Grep error: %v\n", err)
		os.Exit(1)
	}

	output := buf.String()
	fmt.Print(output)

	if *clipFlag || *clipShort {
		if clipErr := fileutil.CopyToClipboard(output); clipErr == nil {
			fmt.Fprintln(os.Stderr, "✔ Grep results copied to clipboard!")
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}
}

func runDef(args []string) {
	fs := flag.NewFlagSet("def", flag.ExitOnError)
	langFlag := fs.String("lang", "all", "Language filter (go, ts, java, kt, rust, or all)")
	pathFlag := fs.String("path", "", "Restrict lookup to specific file path")
	clipFlag := fs.Bool("clip", false, "Copy definition directly to clipboard")
	clipShort := fs.Bool("c", false, "Copy definition directly to clipboard (shorthand)")
	forPatch := fs.Bool("for-patch", false, "Output clean patch-ready replacement block")

	valueFlags := map[string]bool{"--lang": true, "-lang": true, "--path": true, "-path": true}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	if fs.NArg() == 0 {
		fmt.Println("Usage: textify def <symbol> [--lang go,ts] [--path file.go] [-c]")
		os.Exit(1)
	}

	symbol := fs.Arg(0)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	idx, err := parser.BuildIndex(cwd, parser.ParseLanguages(*langFlag))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building AST index: %v\n", err)
		os.Exit(1)
	}

	matches := idx.FindDefinition(symbol, *pathFlag)
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "Symbol %q not found in indexed files.\n", symbol)
		os.Exit(1)
	}

	var sb strings.Builder
	for _, sym := range matches {
		if *forPatch {
			sb.WriteString(fmt.Sprintf("<replace path=\"%s\" symbol=\"%s\">\n<with>\n%s\n</with>\n</replace>\n\n", sym.File, sym.FullName(), sym.Body))
		} else {
			sb.WriteString(fmt.Sprintf("/* Definition: %s %s [%s:%d-%d] */\n", sym.Kind, sym.FullName(), sym.File, sym.StartLine, sym.EndLine))
			for i, line := range strings.Split(sym.Body, "\n") {
				sb.WriteString(fmt.Sprintf("%5d | %s\n", sym.StartLine+i, line))
			}
			sb.WriteString("\n")
		}
	}

	output := sb.String()
	fmt.Print(output)

	if *clipFlag || *clipShort {
		if clipErr := fileutil.CopyToClipboard(output); clipErr == nil {
			fmt.Fprintln(os.Stderr, "✔ Definition copied to clipboard!")
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}
}

func runTrace(args []string) {
	fs := flag.NewFlagSet("trace", flag.ExitOnError)
	langFlag := fs.String("lang", "all", "Language filter (go, ts, java, kt, or all)")
	depthFlag := fs.Int("depth", 2, "Call chain and callee body depth (default 2)")
	deepFlag := fs.Bool("deep", true, "Include implementations of called functions and referenced types")
	clipFlag := fs.Bool("clip", false, "Copy trace directly to clipboard")
	clipShortFlag := fs.Bool("c", false, "Copy trace directly to clipboard (shorthand)")

	valueFlags := map[string]bool{"--lang": true, "-lang": true, "--depth": true, "-depth": true}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	if fs.NArg() == 0 {
		fmt.Println("Usage: textify trace <function_or_struct> [--lang go,ts] [--depth 2] [--deep] [-c]")
		os.Exit(1)
	}

	symbol := fs.Arg(0)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Building AST execution flow for %q...\n", symbol)
	idx, err := parser.BuildIndex(cwd, parser.ParseLanguages(*langFlag))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building AST index: %v\n", err)
		os.Exit(1)
	}

	matches := idx.FindDefinition(symbol, "")
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "Symbol %q not found in indexed files.\n", symbol)
		os.Exit(1)
	}

	var trace string
	if *deepFlag {
		deepRes := idx.TraceDeep(symbol, *depthFlag)
		if deepRes != nil {
			trace = deepRes.Render(false)
		} else {
			trace = idx.TraceFlow(symbol, *depthFlag)
		}
	} else {
		trace = idx.TraceFlow(symbol, *depthFlag)
	}

	fmt.Print(trace)

	if *clipFlag || *clipShortFlag {
		if clipErr := fileutil.CopyToClipboard(trace); clipErr == nil {
			fmt.Fprintln(os.Stderr, "✔ Execution trace copied to clipboard!")
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}
}

func runLoad(args []string) {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	formatFlag := fs.String("format", "tags", "Output format (tags, raw, markdown)")
	formatShort := fs.String("f", "tags", "Output format (shorthand)")
	stdoutFlag := fs.Bool("stdout", false, "Write extracted content to stdout")
	stdoutShort := fs.Bool("s", false, "Write extracted content to stdout (shorthand)")
	clipInFlag := fs.Bool("clip-in", false, "Read request directly from system clipboard")
	clipInShort := fs.Bool("ci", false, "Read request directly from system clipboard (shorthand)")
	clipOutFlag := fs.Bool("clip", false, "Copy output directly to system clipboard")
	clipOutShort := fs.Bool("c", false, "Copy output directly to system clipboard (shorthand)")
	redactFlag := fs.Bool("redact", false, "Redact sensitive tokens and secrets")
	redactShort := fs.Bool("r", false, "Redact sensitive tokens and secrets (shorthand)")

	valueFlags := map[string]bool{"--config": true, "-config": true, "--format": true, "-format": true, "-f": true}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	var rawInput string

	if *clipInFlag || *clipInShort {
		clipContent, err := fileutil.ReadFromClipboard()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from clipboard: %v\n", err)
			os.Exit(1)
		}
		rawInput = clipContent
	} else if fs.NArg() > 0 {
		filePath := fs.Arg(0)
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file %q: %v\n", filePath, err)
			os.Exit(1)
		}
		rawInput = string(data)
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(1)
			}
			rawInput = string(data)
		} else {
			fmt.Println("Usage: textify load [file] [-ci] [-c] [-s]")
			os.Exit(1)
		}
	}

	requests := scanner.ParseRequest(rawInput)
	if len(requests) == 0 {
		fmt.Fprintln(os.Stderr, "No valid request directives (<file>, <slice>, <grep>, <def>, etc.) found.")
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	cfg := config.DefaultConfig()
	if _, err := os.Stat(*configPath); err == nil {
		loaded, loadErr := config.Load(*configPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", loadErr)
			os.Exit(1)
		}
		cfg = *loaded
	}

	var buf bytes.Buffer
	toStdout := *stdoutFlag || *stdoutShort
	toClip := *clipOutFlag || *clipOutShort
	fmtChoice := firstNonEmpty(*formatFlag, *formatShort)
	if fmtChoice == "" {
		fmtChoice = "tags"
	}

	scanRes, err := scanner.LoadRequests(cwd, &cfg, requests, &buf, scanner.ScanOptions{
		Format:        fmtChoice,
		RedactSecrets: *redactFlag || *redactShort,
		Stdout:        toStdout || toClip,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading requests: %v\n", err)
		os.Exit(1)
	}

	output := buf.String()

	if toStdout || (!toClip && scanRes.FilesCount == 0) {
		fmt.Print(output)
	}

	if toClip {
		if clipErr := fileutil.CopyToClipboard(output); clipErr == nil {
			fmt.Fprintf(os.Stderr, "✔ Successfully copied requested content (%d sections, ~%d tokens) to clipboard!\n", scanRes.FilesCount, scanRes.ApproxTokens)
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
			if !toStdout {
				fmt.Print(output)
			}
		}
	} else if !toStdout {
		fmt.Printf("\nDone: %d items loaded (~%d tokens).\n", scanRes.FilesCount, scanRes.ApproxTokens)
	}
}

func runPatch(args []string) {
	fs := flag.NewFlagSet("patch", flag.ExitOnError)
	dryRunFlag := fs.Bool("dry-run", false, "Validate and preview changes without modifying files")
	dryRunShort := fs.Bool("n", false, "Validate and preview changes without modifying files (shorthand)")
	clipInFlag := fs.Bool("clip-in", false, "Read patch directly from system clipboard")
	clipInShort := fs.Bool("ci", false, "Read patch directly from system clipboard (shorthand)")
	fixFlag := fs.Bool("fix", false, "Enable fuzzy matching tolerance for near search matches")
	fixShort := fs.Bool("f", false, "Enable fuzzy matching tolerance for near search matches (shorthand)")

	valueFlags := map[string]bool{}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	var rawPatch string

	if *clipInFlag || *clipInShort {
		clipContent, err := fileutil.ReadFromClipboard()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading patch from clipboard: %v\n", err)
			os.Exit(1)
		}
		rawPatch = clipContent
	} else if fs.NArg() > 0 {
		filePath := fs.Arg(0)
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading patch file %q: %v\n", filePath, err)
			os.Exit(1)
		}
		rawPatch = string(data)
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(1)
			}
			rawPatch = string(data)
		} else {
			fmt.Println("Usage: textify patch [patch.xml] [-ci] [-n] [-f]")
			os.Exit(1)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	opts := patcher.PatchOptions{
		DryRun: *dryRunFlag || *dryRunShort,
		Fix:    *fixFlag || *fixShort,
	}

	res, err := patcher.Apply(cwd, rawPatch, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Patch failed: %v\n", err)
		os.Exit(1)
	}

	prefix := "Applied"
	if opts.DryRun {
		prefix = "Dry-run validated"
	}

	for _, ch := range res.Changes {
		fmt.Printf("✔ [%s] %s %s\n", prefix, ch.Type, ch.Path)
	}

	fmt.Printf("\nSuccessfully %s %d file modification(s).\n", strings.ToLower(prefix), len(res.Changes))
}

func runPrompt(args []string) {
	fs := flag.NewFlagSet("prompt", flag.ExitOnError)
	clipFlag := fs.Bool("clip", false, "Copy instructions to clipboard")
	clipShort := fs.Bool("c", false, "Copy instructions to clipboard (shorthand)")
	_ = fs.Parse(normalizeFlagArgs(args, nil))

	fmt.Print(defaultPrompt)

	if *clipFlag || *clipShort {
		if err := fileutil.CopyToClipboard(defaultPrompt); err == nil {
			fmt.Fprintln(os.Stderr, "\n✔ Prompt instructions copied to clipboard!")
		}
	}
}

func runTokens(args []string) {
	fs := flag.NewFlagSet("tokens", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	clipFlag := fs.Bool("clip", false, "Copy output to clipboard")
	clipShort := fs.Bool("c", false, "Copy output to clipboard (shorthand)")
	stdoutFlag := fs.Bool("stdout", false, "Write output to stdout")
	stdoutShort := fs.Bool("s", false, "Write output to stdout (shorthand)")
	fullFlag := fs.Bool("full", false, "Calculate tokens with full code for all files")
	sigFlag := fs.Bool("signatures", false, "Calculate tokens with signatures only for all files")
	sigShort := fs.Bool("sig", false, "Calculate tokens with signatures only (shorthand)")
	groupFlag := fs.String("group", "", "Only include directories/files in specific group(s)")
	groupsFlag := fs.String("groups", "", "Only include directories/files in specific group(s) (alias)")

	valueFlags := map[string]bool{
		"--config": true, "-config": true,
		"--group": true, "-group": true, "--groups": true, "-groups": true,
	}
	_ = fs.Parse(normalizeFlagArgs(args, valueFlags))

	_ = stdoutFlag
	_ = stdoutShort

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	cfg := config.DefaultConfig()
	if _, err := os.Stat(*configPath); err == nil {
		loaded, loadErr := config.Load(*configPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", loadErr)
			os.Exit(1)
		}
		cfg = *loaded
	}

	stats, err := scanner.CalculateFolderTokens(cwd, &cfg, scanner.ScanOptions{
		SignaturesOnly: *sigFlag || *sigShort,
		ForceFull:      *fullFlag,
		TargetGroups:   splitCommaList(firstNonEmpty(*groupFlag, *groupsFlag)),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calculating folder tokens: %v\n", err)
		os.Exit(1)
	}

	if len(stats) == 0 {
		fmt.Println("No included folders or matching files found.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Included Folders by Estimated Token Size:\n\n")
	totalTokens := 0
	totalFiles := 0
	var totalBytes int64

	for i, stat := range stats {
		totalTokens += stat.ApproxTokens
		totalFiles += stat.FilesCount
		totalBytes += stat.TotalBytes
		fileWord := "files"
		if stat.FilesCount == 1 {
			fileWord = "file"
		}
		sb.WriteString(fmt.Sprintf("%2d. %s - ~%d tokens (%s, %d %s)\n", i+1, stat.Path, stat.ApproxTokens, formatBytes(stat.TotalBytes), stat.FilesCount, fileWord))
	}

	sb.WriteString(fmt.Sprintf("\nTotal: ~%d tokens across %d folder(s) (%d files, %s)\n", totalTokens, len(stats), totalFiles, formatBytes(totalBytes)))

	output := sb.String()
	fmt.Print(output)

	if *clipFlag || *clipShort {
		if clipErr := fileutil.CopyToClipboard(output); clipErr == nil {
			fmt.Fprintln(os.Stderr, "✔ Folder token list copied to clipboard!")
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
		}
	}
}

func runCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "textify.yaml", "Configuration file path")
	_ = fs.Parse(normalizeFlagArgs(args, map[string]bool{"--config": true, "-config": true}))

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "❌ Configuration file %q not found.\nRun 'textify init' to generate a textify.yaml file.\n", *configPath)
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	warnings := cfg.Validate(cwd)

	fmt.Printf("✔ Configuration in %q is syntactically valid!\n\n", *configPath)
	fmt.Printf("Output File: %s\n", cfg.OutputFile)
	fmt.Printf("Format:      %s\n", cfg.Format)
	fmt.Printf("Directories configured: %d\n\n", len(cfg.Dirs))

	for dir, rule := range cfg.Dirs {
		status := "enabled"
		if !rule.Enabled {
			status = "disabled"
		}
		fmt.Printf("  • %-24s [%s]", dir, status)
		if len(rule.Extensions) > 0 {
			fmt.Printf(" ext: [%s]", strings.Join(rule.Extensions, ", "))
		}
		if len(rule.Include) > 0 {
			fmt.Printf(" include: %d rule(s)", len(rule.Include))
		}
		if len(rule.Exclude) > 0 {
			fmt.Printf(" exclude: %d rule(s)", len(rule.Exclude))
		}
		fmt.Println()
	}

	if len(warnings) > 0 {
		fmt.Printf("\n⚠️ Warnings / Suggestions (%d):\n", len(warnings))
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
	} else {
		fmt.Println("\nAll directory rules look great! You are ready to run 'textify start'.")
	}
}

type scanCmdParams struct {
	configPath       string
	format           string
	stdout           bool
	clip             bool
	redact           bool
	tree             bool
	signaturesOnly   bool
	diffOnly         bool
	forceFull        bool
	signaturesGroups []string
	fullCodeGroups   []string
	targetGroups     []string
}

func executeScan(p scanCmdParams) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	cfg := config.DefaultConfig()
	if _, err := os.Stat(p.configPath); err == nil {
		loaded, loadErr := config.Load(p.configPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", loadErr)
			os.Exit(1)
		}
		cfg = *loaded
	}

	var buf bytes.Buffer
	var writer io.Writer = &buf

	var fileOut *os.File
	if !p.stdout && !p.clip {
		outFile := cfg.OutputFile
		if outFile == "" {
			outFile = "codebase.txt"
		}
		f, createErr := os.Create(outFile)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file %q: %v\n", outFile, createErr)
			os.Exit(1)
		}
		defer f.Close()
		fileOut = f
		writer = io.MultiWriter(&buf, fileOut)
	}

	scanRes, err := scanner.Scan(cwd, &cfg, writer, scanner.ScanOptions{
		SignaturesOnly:   p.signaturesOnly,
		DiffOnly:         p.diffOnly,
		ForceFull:        p.forceFull,
		Format:           p.format,
		RedactSecrets:    p.redact,
		Tree:             p.tree,
		Stdout:           p.stdout || p.clip,
		SignaturesGroups: p.signaturesGroups,
		FullCodeGroups:   p.fullCodeGroups,
		TargetGroups:     p.targetGroups,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan error: %v\n", err)
		os.Exit(1)
	}

	output := buf.String()

	if p.stdout {
		fmt.Print(output)
	}

	if p.clip {
		if clipErr := fileutil.CopyToClipboard(output); clipErr == nil {
			fmt.Fprintf(os.Stderr, "✔ Copied codebase context (%d files, ~%d tokens) to clipboard!\n", scanRes.FilesCount, scanRes.ApproxTokens)
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
			if !p.stdout {
				fmt.Print(output)
			}
		}
	} else if !p.stdout {
		fmt.Printf("\n✔ Bundled %d files (%s, ~%d tokens) into %s\n",
			scanRes.FilesCount,
			formatBytes(scanRes.TotalBytes),
			scanRes.ApproxTokens,
			cfg.OutputFile,
		)
		if scanRes.RedactedSecrets > 0 {
			fmt.Printf("🛡️ Redacted %d sensitive secrets/tokens.\n", scanRes.RedactedSecrets)
		}
	}
}

func handleTextOutput(output, label string, stdout, clip bool) {
	if stdout {
		fmt.Print(output)
	}
	if clip {
		if clipErr := fileutil.CopyToClipboard(output); clipErr == nil {
			fmt.Fprintf(os.Stderr, "✔ %s copied to clipboard!\n", label)
		} else {
			fmt.Fprintf(os.Stderr, "Notice: Could not copy to clipboard (%v)\n", clipErr)
			if !stdout {
				fmt.Print(output)
			}
		}
	} else if !stdout {
		fmt.Print(output)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
}

func splitCommaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var res []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
