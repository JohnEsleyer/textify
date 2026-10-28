package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
)

// ScanOptions controls advanced scanning, formatting, and filtering behaviors.
type ScanOptions struct {
	SignaturesOnly bool
	DiffOnly       bool
	Format         string
	MaxFileSize    int64
	RedactSecrets  bool
	Tree           bool
	Stdout         bool
}

// ScanResult contains metadata and statistics about the completed scan.
type ScanResult struct {
	FilesCount      int
	TotalBytes      int64
	ApproxTokens    int
	RedactedSecrets int
}

// Scan initiates the directory walk based on the provided configuration and options.
func Scan(rootPath string, cfg *config.Config, writer io.Writer, opts ScanOptions) (*ScanResult, error) {
	chain := config.NewGitIgnoreChain(rootPath)
	bufWriter := bufio.NewWriter(writer)
	defer bufWriter.Flush()

	// 1. Initial rule (Root ".")
	rootRule, ok := cfg.Dirs["."]
	if !ok {
		rootRule = config.DirRule{Enabled: true, Extensions: []string{}}
	}

	// 2. Resolve diff filtering if requested
	var changedFiles map[string]bool
	if opts.DiffOnly {
		var err error
		changedFiles, err = GetGitChangedFiles(rootPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get git diff status: %w", err)
		}
	}

	// 3. Collect matching files first
	var matchedFiles []string
	var walkErr error

	walkErr = collectFiles(rootPath, rootPath, cfg, rootRule, chain, changedFiles, &matchedFiles)
	if walkErr != nil {
		return nil, walkErr
	}

	// 4. Prepend Tree if enabled
	showTree := true
	if cfg.Tree != nil {
		showTree = *cfg.Tree
	}
	if opts.Tree {
		showTree = true
	}

	if showTree && len(matchedFiles) > 0 {
		treeStr := GenerateTree(matchedFiles)
		fmt.Fprint(bufWriter, treeStr)
		fmt.Fprintln(bufWriter, strings.Repeat("=", 60)+"\n")
	}

	// Format selection: opts override config, defaults to raw
	format := "raw"
	if cfg.Format != "" {
		format = cfg.Format
	}
	if opts.Format != "" {
		format = opts.Format
	}

	maxSize := cfg.MaxFileSize
	if maxSize <= 0 {
		maxSize = 500 * 1024
	}
	if opts.MaxFileSize > 0 {
		maxSize = opts.MaxFileSize
	}

	result := &ScanResult{}

	// 5. Render file contents
	for _, relPath := range matchedFiles {
		absPath := filepath.Join(rootPath, filepath.FromSlash(relPath))

		// Check for specific directory rule to observe 'compact'
		dirPath := filepath.Dir(relPath)
		rule := rootRule
		if r, exists := cfg.Dirs[dirPath]; exists {
			rule = r
		}

		addedBytes, redacted, err := appendFileContent(absPath, relPath, rule, bufWriter, opts.SignaturesOnly, format, maxSize, opts.RedactSecrets)
		if err != nil {
			continue
		}

		result.FilesCount++
		result.TotalBytes += addedBytes
		result.RedactedSecrets += redacted

		modeSuffix := ""
		if opts.SignaturesOnly {
			modeSuffix = " (signatures)"
		} else if rule.Compact {
			modeSuffix = " (compacted)"
		}
		if !opts.Stdout {
			fmt.Printf("Added: %s%s\n", relPath, modeSuffix)
		}
	}

	// Token estimation: ~4 chars per token
	result.ApproxTokens = int(result.TotalBytes / 4)

	return result, nil
}

func collectFiles(
	fullPath string,
	rootPath string,
	cfg *config.Config,
	currentRule config.DirRule,
	ignoreChain config.GitIgnoreChain,
	changedFiles map[string]bool,
	matchedFiles *[]string,
) error {

	relDir, _ := filepath.Rel(rootPath, fullPath)
	if relDir == "." {
		relDir = "."
	} else {
		relDir = filepath.ToSlash(relDir)
	}

	if specificRule, exists := cfg.Dirs[relDir]; exists {
		currentRule = specificRule
	}

	if !currentRule.Enabled {
		return nil
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())
		relEntryPath, _ := filepath.Rel(rootPath, entryPath)
		relEntryPath = filepath.ToSlash(relEntryPath)
		ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")

		// System exclusions (including dynamic OutputFile name)
		if shouldAlwaysExclude(entry.Name(), cfg.OutputFile) {
			continue
		}

		// Exclude pattern rule
		if checkPatternMatch(entry.Name(), relEntryPath, currentRule.Exclude) {
			continue
		}

		isForced := checkPatternMatch(entry.Name(), relEntryPath, currentRule.Include)

		if entry.IsDir() {
			isIgnored := ignoreChain.Match(entryPath, true)
			explicitRule, hasExplicitRule := cfg.Dirs[relEntryPath]

			// If explicitly disabled in config, always skip
			if hasExplicitRule && !explicitRule.Enabled {
				continue
			}

			// If explicitly enabled in config, it takes precedence over .gitignore
			isExplicitlyEnabled := hasExplicitRule && explicitRule.Enabled

			// If gitignored and not forced and not explicitly enabled, skip
			if isIgnored && !isForced && !isExplicitlyEnabled {
				continue
			}

			var subChain config.GitIgnoreChain
			if isIgnored && isExplicitlyEnabled {
				// Parent gitignore ignored this directory, but user explicitly enabled it in config.
				// Reset ignore chain to this directory so the parent directory's ignore rule
				// does not suppress files inside this folder.
				subChain = config.NewGitIgnoreChain(entryPath)
			} else {
				subChain = ignoreChain.AddIfExists(entryPath)
			}

			if err := collectFiles(entryPath, rootPath, cfg, currentRule, subChain, changedFiles, matchedFiles); err != nil {
				return err
			}
			continue
		}

		// Gitignore check
		if !isForced && ignoreChain.Match(entryPath, false) {
			continue
		}

		// Git diff filter check
		if changedFiles != nil && !isForced {
			if !changedFiles[relEntryPath] {
				continue
			}
		}

		// Extension excludes
		if !isForced && len(currentRule.ExcludeExtensions) > 0 {
			if contains(currentRule.ExcludeExtensions, ext) {
				continue
			}
		}

		// Extension includes
		if !isForced && len(currentRule.Extensions) > 0 {
			if !contains(currentRule.Extensions, ext) {
				continue
			}
		}

		*matchedFiles = append(*matchedFiles, relEntryPath)
	}

	return nil
}

// shouldAlwaysExclude handles hardcoded exclusions and dynamic output file.
func shouldAlwaysExclude(name, outputFile string) bool {
	outName := filepath.Base(outputFile)
	return name == ".git" || name == "textify.yaml" || name == outName
}

// checkPatternMatch checks if the file matches any of the glob patterns.
func checkPatternMatch(name, relPath string, patterns []string) bool {
	for _, p := range patterns {
		// Match against filename
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		// Match against relative path
		if matched, _ := filepath.Match(p, relPath); matched {
			return true
		}
		// Direct folder/file path match
		if p == relPath {
			return true
		}
	}
	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// appendFileContent writes the formatted file entry to the buffer.
func appendFileContent(
	absPath, relPath string,
	rule config.DirRule,
	writer *bufio.Writer,
	signaturesOnly bool,
	format string,
	maxSize int64,
	redactSecrets bool,
) (int64, int, error) {
	// Skip binary files
	isBin, err := fileutil.IsBinary(absPath)
	if err != nil || isBin {
		return 0, 0, nil
	}

	// Enforce file size limits
	info, err := os.Stat(absPath)
	if err != nil {
		return 0, 0, err
	}
	if info.Size() > maxSize {
		return 0, 0, nil
	}

	rawBytes, err := os.ReadFile(absPath)
	if err != nil {
		return 0, 0, err
	}

	// Detect and skip minified files (e.g. bundle.js, single line with > 2000 chars)
	if isMinified(rawBytes) {
		return 0, 0, nil
	}

	content := string(rawBytes)

	// Process Signatures mode
	if signaturesOnly {
		content = ExtractSignatures(relPath, content)
	} else if rule.Compact {
		// Strip blank lines and trailing spaces
		var sb strings.Builder
		sc := bufio.NewScanner(strings.NewReader(content))
		for sc.Scan() {
			t := strings.TrimSpace(sc.Text())
			if t != "" {
				sb.WriteString(t)
				sb.WriteString("\n")
			}
		}
		content = sb.String()
	}

	redactedCount := 0
	if redactSecrets {
		content, redactedCount = fileutil.RedactSecrets(content)
	}

	// Render formatted content
	var formatted strings.Builder
	switch strings.ToLower(format) {
	case "markdown", "md":
		ext := strings.TrimPrefix(filepath.Ext(relPath), ".")
		formatted.WriteString(fmt.Sprintf("### File: `%s`\n\n```%s\n%s\n```\n\n", relPath, ext, strings.TrimRight(content, "\n")))
	case "xml":
		formatted.WriteString(fmt.Sprintf("<file path=\"%s\">\n%s\n</file>\n\n", relPath, strings.TrimRight(content, "\n")))
	default: // "raw"
		separator := strings.Repeat("-", 50)
		formatted.WriteString(fmt.Sprintf("%s\nFILE: %s\n%s\n\n%s\n\n", separator, relPath, separator, strings.TrimRight(content, "\n")))
	}

	n, writeErr := writer.WriteString(formatted.String())
	return int64(n), redactedCount, writeErr
}

func isMinified(content []byte) bool {
	lineLen := 0
	for _, b := range content {
		if b == '\n' {
			if lineLen > 2000 {
				return true
			}
			lineLen = 0
		} else {
			lineLen++
		}
	}
	return lineLen > 2000
}
