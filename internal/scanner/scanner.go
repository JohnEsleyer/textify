package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
)

// ScanOptions controls advanced scanning, formatting, and filtering behaviors.
type ScanOptions struct {
	SignaturesOnly   bool
	DiffOnly         bool
	ForceFull        bool
	Format           string
	MaxFileSize      int64
	RedactSecrets    bool
	Tree             bool
	Stdout           bool
	SignaturesGroups []string
	FullCodeGroups   []string
	TargetGroups     []string
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
	if err := collectFiles(rootPath, rootPath, cfg, chain, changedFiles, opts.TargetGroups, &matchedFiles); err != nil {
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

		rule := resolveEffectiveRule(relPath, cfg, rootRule)

		useSignatures := shouldExtractSignatures(relPath, rule.Group, opts.SignaturesOnly, cfg, opts)
		addedBytes, redacted, err := appendFileContent(absPath, relPath, rule, bufWriter, useSignatures, format, maxSize, opts.RedactSecrets)
		if err != nil {
			continue
		}

		result.FilesCount++
		result.TotalBytes += addedBytes
		result.RedactedSecrets += redacted

		modeSuffix := ""
		if useSignatures {
			modeSuffix = " (signatures)"
		} else if opts.SignaturesOnly && IsDocFile(relPath) {
			modeSuffix = " (full doc)"
		} else if rule.Compact {
			modeSuffix = " (compacted)"
		}
		if rule.Group != "" {
			modeSuffix += fmt.Sprintf(" [group: %s]", rule.Group)
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
	ignoreChain config.GitIgnoreChain,
	changedFiles map[string]bool,
	targetGroups []string,
	matchedFiles *[]string,
) error {
	relDir, _ := filepath.Rel(rootPath, fullPath)
	if relDir == "." {
		relDir = "."
	} else {
		relDir = filepath.ToSlash(relDir)
	}

	rootRule, ok := cfg.Dirs["."]
	if !ok {
		rootRule = config.DirRule{Enabled: true}
	}

	currentRule := resolveEffectiveRule(relDir, cfg, rootRule)
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

		entryRule, baseDir := resolveEffectiveRuleWithBase(relEntryPath, cfg, rootRule)

		// 1. Force Exclude (highest user priority)
		if matchPattern(entry.Name(), relEntryPath, baseDir, entryRule.Exclude) {
			continue
		}

		// 2. Force Include (overrides gitignore & extension filters)
		isForced := matchPattern(entry.Name(), relEntryPath, baseDir, entryRule.Include)

		if entry.IsDir() {
			isIgnored := ignoreChain.Match(entryPath, true)
			explicitRule, hasExplicitRule := cfg.Dirs[relEntryPath]

			if hasExplicitRule && !explicitRule.Enabled {
				continue
			}

			isExplicitlyEnabled := hasExplicitRule && explicitRule.Enabled
			isDirForced := isForced || hasIncludePrefix(relEntryPath, baseDir, entryRule.Include)

			if isIgnored && !isDirForced && !isExplicitlyEnabled {
				continue
			}

			var subChain config.GitIgnoreChain
			if isIgnored && isExplicitlyEnabled {
				subChain = config.NewGitIgnoreChain(entryPath)
			} else {
				subChain = ignoreChain.AddIfExists(entryPath)
			}

			if err := collectFiles(entryPath, rootPath, cfg, subChain, changedFiles, targetGroups, matchedFiles); err != nil {
				return err
			}
			continue
		}

		// 3. Gitignore check (skipped if force-included)
		if !isForced && ignoreChain.Match(entryPath, false) {
			continue
		}

		// 4. Git diff check
		if changedFiles != nil && !isForced {
			if !changedFiles[relEntryPath] {
				continue
			}
		}

		// 5. Group filter
		if len(targetGroups) > 0 && !containsCaseInsensitive(targetGroups, entryRule.Group) {
			continue
		}

		// 6. Block-list extensions
		if !isForced && len(entryRule.ExcludeExtensions) > 0 {
			if contains(entryRule.ExcludeExtensions, ext) {
				continue
			}
		}

		// 7. Allow-list extensions
		if !isForced && len(entryRule.Extensions) > 0 {
			if !contains(entryRule.Extensions, ext) {
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

// matchPattern matches file/directory names or relative paths against rule patterns,
// supporting exact matches, subfolder prefixes, name globs, and patterns relative
// to the directory rule's base.
func matchPattern(name, relPath, baseDir string, patterns []string) bool {
	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	nameLower := strings.ToLower(name)
	relLower := strings.ToLower(cleanRel)

	for _, p := range patterns {
		pat := strings.TrimSpace(filepath.ToSlash(p))
		pat = strings.TrimPrefix(pat, "/")
		pat = strings.TrimSuffix(pat, "/")
		if pat == "" {
			continue
		}

		// Literal "." matches the directory rule itself
		if pat == "." {
			if cleanRel == baseDir || (baseDir == "." && cleanRel == ".") {
				return true
			}
			continue
		}

		patLower := strings.ToLower(pat)

		// 1. Direct match on file/directory base name (e.g. "errors.go" or "*.log")
		if nameLower == patLower {
			return true
		}
		if matched, _ := filepath.Match(patLower, nameLower); matched {
			return true
		}

		// 2. Direct match relative to root
		if relLower == patLower {
			return true
		}
		if strings.HasPrefix(relLower, patLower+"/") {
			return true
		}
		if matched, _ := filepath.Match(patLower, relLower); matched {
			return true
		}

		// 3. Match relative to baseDir (when baseDir != ".")
		if baseDir != "." && baseDir != "" {
			var combined string
			if !strings.HasPrefix(patLower, strings.ToLower(baseDir)+"/") {
				combined = strings.ToLower(baseDir) + "/" + patLower
			} else {
				combined = patLower
			}

			if relLower == combined {
				return true
			}
			if strings.HasPrefix(relLower, combined+"/") {
				return true
			}
			if matched, _ := filepath.Match(combined, relLower); matched {
				return true
			}

			// Subpath within baseDir
			if strings.HasPrefix(relLower, strings.ToLower(baseDir)+"/") {
				inner := strings.TrimPrefix(relLower, strings.ToLower(baseDir)+"/")
				if inner == patLower || strings.HasPrefix(inner, patLower+"/") {
					return true
				}
				if matched, _ := filepath.Match(patLower, inner); matched {
					return true
				}
			}
		}
	}

	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func resolveEffectiveRule(relPath string, cfg *config.Config, rootRule config.DirRule) config.DirRule {
	r, _ := resolveEffectiveRuleWithBase(relPath, cfg, rootRule)
	return r
}

func resolveEffectiveRuleWithBase(relPath string, cfg *config.Config, rootRule config.DirRule) (config.DirRule, string) {
	effective := rootRule
	effectiveBase := "."

	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	if cleanRel == "." || cleanRel == "" {
		return effective, effectiveBase
	}

	parts := strings.Split(cleanRel, "/")
	current := ""
	for i, part := range parts {
		if i == 0 {
			current = part
		} else {
			current = current + "/" + part
		}

		rule, exists := cfg.Dirs[current]
		if !exists {
			continue
		}

		effective.Enabled = rule.Enabled
		effectiveBase = current

		if rule.Compact {
			effective.Compact = true
		}

		if rule.Group != "" {
			effective.Group = rule.Group
		}

		if len(rule.Extensions) > 0 {
			effective.Extensions = rule.Extensions
		}

		for _, ext := range rule.ExcludeExtensions {
			if !contains(effective.ExcludeExtensions, ext) {
				effective.ExcludeExtensions = append(effective.ExcludeExtensions, ext)
			}
		}

		for _, inc := range rule.Include {
			if !contains(effective.Include, inc) {
				effective.Include = append(effective.Include, inc)
			}
		}

		for _, exc := range rule.Exclude {
			if !contains(effective.Exclude, exc) {
				effective.Exclude = append(effective.Exclude, exc)
			}
		}
	}

	return effective, effectiveBase
}

func hasIncludePrefix(relPath, baseDir string, includes []string) bool {
	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	prefix := cleanRel + "/"

	for _, inc := range includes {
		inc = strings.TrimSpace(filepath.ToSlash(inc))
		inc = strings.TrimPrefix(inc, "/")
		inc = strings.TrimSuffix(inc, "/")
		if inc == "" || inc == "." {
			continue
		}

		var targetPath string
		if baseDir != "." && baseDir != "" && !strings.HasPrefix(inc, baseDir+"/") {
			targetPath = baseDir + "/" + inc
		} else {
			targetPath = inc
		}

		if strings.HasPrefix(targetPath, prefix) {
			return true
		}
	}
	return false
}

func shouldExtractSignatures(relPath string, group string, globalSigsOnly bool, cfg *config.Config, opts ScanOptions) bool {
	if IsDocFile(relPath) {
		return false
	}

	if opts.ForceFull {
		return false
	}

	if globalSigsOnly {
		return true
	}

	if group != "" {
		if containsCaseInsensitive(opts.SignaturesGroups, group) {
			return true
		}
		if containsCaseInsensitive(opts.FullCodeGroups, group) {
			return false
		}
		if cfg != nil {
			if cfg.IsGroupSignaturesOnly(group) {
				return true
			}
			if cfg.IsGroupFullCode(group) {
				return false
			}
		}
	}

	return false
}

func containsCaseInsensitive(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func appendFileContent(
	absPath, relPath string,
	rule config.DirRule,
	writer *bufio.Writer,
	useSignatures bool,
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

	if useSignatures && !IsDocFile(relPath) {
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

// FolderTokenStat represents the estimated token size of an included folder.
type FolderTokenStat struct {
	Path         string
	TotalBytes   int64
	ApproxTokens int
	FilesCount   int
}

// CalculateFolderTokens scans included files and aggregates token estimates by folder from largest to smallest.
func CalculateFolderTokens(rootPath string, cfg *config.Config, opts ScanOptions) ([]FolderTokenStat, error) {
	chain := config.NewGitIgnoreChain(rootPath)

	rootRule, ok := cfg.Dirs["."]
	if !ok {
		rootRule = config.DirRule{Enabled: true, Extensions: []string{}}
	}

	var changedFiles map[string]bool
	if opts.DiffOnly {
		var err error
		changedFiles, err = GetGitChangedFiles(rootPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get git diff status: %w", err)
		}
	}

	var matchedFiles []string
	if err := collectFiles(rootPath, rootPath, cfg, chain, changedFiles, opts.TargetGroups, &matchedFiles); err != nil {
		return nil, err
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

	folderMap := make(map[string]*FolderTokenStat)

	for _, relPath := range matchedFiles {
		absPath := filepath.Join(rootPath, filepath.FromSlash(relPath))
		rule := resolveEffectiveRule(relPath, cfg, rootRule)

		useSignatures := shouldExtractSignatures(relPath, rule.Group, opts.SignaturesOnly, cfg, opts)
		addedBytes, _, err := appendFileContent(absPath, relPath, rule, bufio.NewWriter(io.Discard), useSignatures, format, maxSize, opts.RedactSecrets)
		if err != nil {
			continue
		}

		dir := filepath.ToSlash(filepath.Dir(relPath))
		stat, exists := folderMap[dir]
		if !exists {
			stat = &FolderTokenStat{Path: dir}
			folderMap[dir] = stat
		}
		stat.TotalBytes += addedBytes
		stat.FilesCount++
	}

	stats := make([]FolderTokenStat, 0, len(folderMap))
	for _, stat := range folderMap {
		stat.ApproxTokens = int(stat.TotalBytes / 4)
		stats = append(stats, *stat)
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].ApproxTokens != stats[j].ApproxTokens {
			return stats[i].ApproxTokens > stats[j].ApproxTokens
		}
		if stats[i].TotalBytes != stats[j].TotalBytes {
			return stats[i].TotalBytes > stats[j].TotalBytes
		}
		return stats[i].Path < stats[j].Path
	})

	return stats, nil
}
