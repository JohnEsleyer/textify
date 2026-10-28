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

	// Initial rule (Root ".")
	rootRule, ok := cfg.Dirs["."]
	if !ok {
		rootRule = config.DirRule{Enabled: true, Extensions: []string{}}
	}

	// Resolve diff filtering if requested
	var changedFiles map[string]bool
	if opts.DiffOnly {
		var err error
		changedFiles, err = GetGitChangedFiles(rootPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get git diff status: %w", err)
		}
	}

	// Collect matching files
	var matchedFiles []string
	if err := collectFiles(rootPath, rootPath, cfg, rootRule, chain, changedFiles, &matchedFiles); err != nil {
		return nil, err
	}

	// Prepend Tree if enabled
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

	format := "tags"
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

	// Render file contents
	for _, relPath := range matchedFiles {
		absPath := filepath.Join(rootPath, filepath.FromSlash(relPath))

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
			if IsDocFile(relPath) {
				modeSuffix = " (full doc)"
			} else {
				modeSuffix = " (signatures)"
			}
		} else if rule.Compact {
			modeSuffix = " (compacted)"
		}
		if !opts.Stdout {
			fmt.Printf("Added: %s%s\n", relPath, modeSuffix)
		}
	}

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

		if shouldAlwaysExclude(entry.Name(), cfg.OutputFile) {
			continue
		}

		if checkPatternMatch(entry.Name(), relEntryPath, currentRule.Exclude) {
			continue
		}

		isForced := checkPatternMatch(entry.Name(), relEntryPath, currentRule.Include)

		if entry.IsDir() {
			isIgnored := ignoreChain.Match(entryPath, true)
			explicitRule, hasExplicitRule := cfg.Dirs[relEntryPath]

			if hasExplicitRule && !explicitRule.Enabled {
				continue
			}

			isExplicitlyEnabled := hasExplicitRule && explicitRule.Enabled

			if isIgnored && !isForced && !isExplicitlyEnabled {
				continue
			}

			var subChain config.GitIgnoreChain
			if isIgnored && isExplicitlyEnabled {
				subChain = config.NewGitIgnoreChain(entryPath)
			} else {
				subChain = ignoreChain.AddIfExists(entryPath)
			}

			if err := collectFiles(entryPath, rootPath, cfg, currentRule, subChain, changedFiles, matchedFiles); err != nil {
				return err
			}
			continue
		}

		if !isForced && ignoreChain.Match(entryPath, false) {
			continue
		}

		if changedFiles != nil && !isForced {
			if !changedFiles[relEntryPath] {
				continue
			}
		}

		if !isForced && len(currentRule.ExcludeExtensions) > 0 {
			if contains(currentRule.ExcludeExtensions, ext) {
				continue
			}
		}

		if !isForced && len(currentRule.Extensions) > 0 {
			if !contains(currentRule.Extensions, ext) {
				continue
			}
		}

		*matchedFiles = append(*matchedFiles, relEntryPath)
	}

	return nil
}

func shouldAlwaysExclude(name, outputFile string) bool {
	outName := filepath.Base(outputFile)
	return name == ".git" || name == "textify.yaml" || name == outName
}

func checkPatternMatch(name, relPath string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		if matched, _ := filepath.Match(p, relPath); matched {
			return true
		}
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

func appendFileContent(
	absPath, relPath string,
	rule config.DirRule,
	writer *bufio.Writer,
	signaturesOnly bool,
	format string,
	maxSize int64,
	redactSecrets bool,
) (int64, int, error) {
	isBin, err := fileutil.IsBinary(absPath)
	if err != nil || isBin {
		return 0, 0, nil
	}

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

	if isMinified(rawBytes) {
		return 0, 0, nil
	}

	content := string(rawBytes)

	useSignatures := signaturesOnly && !IsDocFile(relPath)

	if useSignatures {
		content = ExtractSignatures(relPath, content)
	} else if rule.Compact {
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

	origLines := strings.Split(string(rawBytes), "\n")
	totalLines := len(origLines)

	lines := strings.Split(content, "\n")
	if !useSignatures {
		var numbered strings.Builder
		for idx, line := range lines {
			numbered.WriteString(fmt.Sprintf("%5d | %s\n", idx+1, line))
		}
		content = numbered.String()
	}

	formatted := renderFormattedEntry(relPath, content, format, 1, totalLines)

	n, writeErr := writer.WriteString(formatted)
	return int64(n), redactedCount, writeErr
}

func renderFormattedEntry(relPath, content, format string, startLine, endLine int) string {
	lineAttr := ""
	if startLine > 0 && endLine > 0 {
		lineAttr = fmt.Sprintf(" lines=\"%d-%d\"", startLine, endLine)
	}

	switch strings.ToLower(format) {
	case "markdown", "md":
		ext := strings.TrimPrefix(filepath.Ext(relPath), ".")
		lineRangeDesc := ""
		if startLine > 0 && endLine > 0 {
			lineRangeDesc = fmt.Sprintf(" (lines %d-%d)", startLine, endLine)
		}
		return fmt.Sprintf("### File: `%s`%s\n\n```%s\n%s\n```\n\n", relPath, lineRangeDesc, ext, strings.TrimRight(content, "\n"))
	case "raw":
		separator := strings.Repeat("-", 50)
		lineRangeDesc := ""
		if startLine > 0 && endLine > 0 {
			lineRangeDesc = fmt.Sprintf(" (lines %d-%d)", startLine, endLine)
		}
		return fmt.Sprintf("%s\nFILE: %s%s\n%s\n\n%s\n\n", separator, relPath, lineRangeDesc, separator, strings.TrimRight(content, "\n"))
	default: // "tags", "xml", or default
		return fmt.Sprintf("\x3cfile path=\"%s\"%s\x3e\n%s\n\x3c/file\x3e\n\n", relPath, lineAttr, strings.TrimRight(content, "\n"))
	}
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
