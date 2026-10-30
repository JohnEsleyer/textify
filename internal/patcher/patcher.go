package patcher

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JohnEsleyer/textify/internal/parser"
	"github.com/JohnEsleyer/textify/internal/scanner"
)

// ActionType identifies the kind of patch operation.
type ActionType string

const (
	ActionFile    ActionType = "file"
	ActionReplace ActionType = "replace"
	ActionDelete  ActionType = "delete"
)

// Directive defines a single parsed change command.
type Directive struct {
	Type    ActionType
	Path    string
	Content string // For ActionFile

	// Target attributes for ActionReplace
	Symbol string
	From   string
	To     string
	After  string
	Before string
	Lines  string
	Search string
	With   string
}

// PatchOptions configures patch application behavior.
type PatchOptions struct {
	DryRun bool
}

// FileChangeType describes the category of modification applied to a file.
type FileChangeType string

const (
	ChangeCreate FileChangeType = "CREATE"
	ChangeUpdate FileChangeType = "UPDATE"
	ChangeDelete FileChangeType = "DELETE"
)

// FileChange records the outcome of a single directive.
type FileChange struct {
	Path        string
	Type        FileChangeType
	Description string
	Lines       int
}

// PatchResult summarizes all applied or validated operations.
type PatchResult struct {
	Changes []FileChange
}

var (
	xmlFileTagRegex    = regexp.MustCompile(`(?is)<file\s+([^>]*?)>(.*?)</file>`)
	xmlReplaceTagRegex = regexp.MustCompile(`(?is)<replace\s+([^>]*?)>(.*?)</replace>`)
	xmlDeleteTagRegex  = regexp.MustCompile(`(?i)<delete\s+([^>]*?)(?:/>|>.*?</delete>)`)

	xmlSearchBlockRegex = regexp.MustCompile(`(?is)<search>(.*?)</search>`)
	xmlWithBlockRegex   = regexp.MustCompile(`(?is)<with>(.*?)</with>`)

	xmlAttrPath   = regexp.MustCompile(`(?i)\b(?:path|file)=["']([^"']+)["']`)
	xmlAttrSymbol = regexp.MustCompile(`(?i)\b(?:symbol|func|struct|name)=["']([^"']+)["']`)
	xmlAttrFrom   = regexp.MustCompile(`(?i)\bfrom=["']([^"']+)["']`)
	xmlAttrTo     = regexp.MustCompile(`(?i)\bto=["']([^"']+)["']`)
	xmlAttrAfter  = regexp.MustCompile(`(?i)\bafter=["']([^"']+)["']`)
	xmlAttrBefore = regexp.MustCompile(`(?i)\bbefore=["']([^"']+)["']`)
	xmlAttrLines  = regexp.MustCompile(`(?i)\blines=["']([^"']+)["']`)

	lineNumberRegex = regexp.MustCompile(`(?m)^\s*\d+\s*\|\s?`)
	codeFenceRegex  = regexp.MustCompile(`(?s)^[ \t]*` + "```" + `[a-zA-Z0-9_-]*\r?\n(.*?)\r?\n[ \t]*` + "```" + `[ \t]*$`)
	tagLeakageRegex = regexp.MustCompile(`(?m)^\s*</?(?:file|replace|search|with|delete)(?:\s+[^>]*)?>\s*$`)
)

// ParseDirectives extracts and sanitizes all patch directives from input text.
func ParseDirectives(input string) ([]Directive, error) {
	type rawItem struct {
		pos int
		dir Directive
	}

	var items []rawItem

	// 1. Parse <file path="...">...</file>
	for _, m := range xmlFileTagRegex.FindAllStringSubmatchIndex(input, -1) {
		attrs := input[m[2]:m[3]]
		body := input[m[4]:m[5]]

		pathMatch := xmlAttrPath.FindStringSubmatch(attrs)
		if len(pathMatch) < 2 {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(pathMatch[1]))
		cleanBody := sanitizeContent(body, true)

		items = append(items, rawItem{
			pos: m[0],
			dir: Directive{
				Type:    ActionFile,
				Path:    path,
				Content: cleanBody,
			},
		})
	}

	// 2. Parse <replace path="...">...</replace>
	for _, m := range xmlReplaceTagRegex.FindAllStringSubmatchIndex(input, -1) {
		attrs := input[m[2]:m[3]]
		body := input[m[4]:m[5]]

		pathMatch := xmlAttrPath.FindStringSubmatch(attrs)
		if len(pathMatch) < 2 {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(pathMatch[1]))

		dir := Directive{
			Type: ActionReplace,
			Path: path,
		}

		if match := xmlAttrSymbol.FindStringSubmatch(attrs); len(match) >= 2 {
			dir.Symbol = strings.TrimSpace(match[1])
		}
		if match := xmlAttrFrom.FindStringSubmatch(attrs); len(match) >= 2 {
			dir.From = strings.TrimSpace(match[1])
		}
		if match := xmlAttrTo.FindStringSubmatch(attrs); len(match) >= 2 {
			dir.To = strings.TrimSpace(match[1])
		}
		if match := xmlAttrAfter.FindStringSubmatch(attrs); len(match) >= 2 {
			dir.After = strings.TrimSpace(match[1])
		}
		if match := xmlAttrBefore.FindStringSubmatch(attrs); len(match) >= 2 {
			dir.Before = strings.TrimSpace(match[1])
		}
		if match := xmlAttrLines.FindStringSubmatch(attrs); len(match) >= 2 {
			dir.Lines = strings.TrimSpace(match[1])
		}

		if sMatch := xmlSearchBlockRegex.FindStringSubmatch(body); len(sMatch) >= 2 {
			dir.Search = sanitizeContent(sMatch[1], false)
		}
		if wMatch := xmlWithBlockRegex.FindStringSubmatch(body); len(wMatch) >= 2 {
			dir.With = sanitizeContent(wMatch[1], true)
		} else if dir.Search == "" {
			// If neither <search> nor <with> is present, the inner block is the replacement
			dir.With = sanitizeContent(body, true)
		}

		// Validation: every <replace> must have a target
		hasTarget := dir.Symbol != "" || (dir.From != "" && dir.To != "") ||
			dir.After != "" || dir.Before != "" || dir.Lines != "" || dir.Search != ""
		if !hasTarget {
			return nil, fmt.Errorf("replace directive for %q missing target (must specify symbol, search block, from/to, after, before, or lines)", path)
		}

		items = append(items, rawItem{
			pos: m[0],
			dir: dir,
		})
	}

	// 3. Parse <delete path="..." />
	for _, m := range xmlDeleteTagRegex.FindAllStringSubmatchIndex(input, -1) {
		attrs := input[m[2]:m[3]]
		pathMatch := xmlAttrPath.FindStringSubmatch(attrs)
		if len(pathMatch) < 2 {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(pathMatch[1]))

		items = append(items, rawItem{
			pos: m[0],
			dir: Directive{
				Type: ActionDelete,
				Path: path,
			},
		})
	}

	// Maintain strict document appearance order
	sort.Slice(items, func(i, j int) bool {
		return items[i].pos < items[j].pos
	})

	directives := make([]Directive, len(items))
	for i, it := range items {
		directives[i] = it.dir
	}

	return directives, nil
}

// sanitizeContent strips line numbers, markdown fences, and accidental tag leaks.
func sanitizeContent(content string, stripLeaks bool) string {
	res := lineNumberRegex.ReplaceAllString(content, "")

	trimmed := strings.TrimSpace(res)
	if match := codeFenceRegex.FindStringSubmatch(trimmed); len(match) >= 2 {
		res = match[1]
	}

	if stripLeaks {
		res = tagLeakageRegex.ReplaceAllString(res, "")
	}

	return strings.Trim(res, "\r\n")
}

// VFS represents an in-memory virtual filesystem for atomic two-pass execution.
type VFS struct {
	rootDir string
	files   map[string][]byte
	deleted map[string]bool
	created map[string]bool
	updated map[string]bool
}

// NewVFS initializes a virtual filesystem overlay on rootDir.
func NewVFS(rootDir string) *VFS {
	return &VFS{
		rootDir: rootDir,
		files:   make(map[string][]byte),
		deleted: make(map[string]bool),
		created: make(map[string]bool),
		updated: make(map[string]bool),
	}
}

// ReadFile retrieves a file from memory or loads it from disk overlay.
func (v *VFS) ReadFile(relPath string) ([]byte, error) {
	norm := filepath.ToSlash(relPath)
	if v.deleted[norm] {
		return nil, os.ErrNotExist
	}
	if data, ok := v.files[norm]; ok {
		return data, nil
	}

	diskPath := filepath.Join(v.rootDir, filepath.FromSlash(norm))
	data, err := os.ReadFile(diskPath)
	if err != nil {
		return nil, err
	}
	v.files[norm] = data
	return data, nil
}

// WriteFile stages new file content into memory.
func (v *VFS) WriteFile(relPath string, data []byte) {
	norm := filepath.ToSlash(relPath)
	delete(v.deleted, norm)
	v.files[norm] = data
}

// DeleteFile stages a file deletion in memory.
func (v *VFS) DeleteFile(relPath string) {
	norm := filepath.ToSlash(relPath)
	v.deleted[norm] = true
	delete(v.files, norm)
}

// Apply parses directives, validates them atomically in VFS, and commits to disk.
func Apply(rootDir, input string, opts PatchOptions) (*PatchResult, error) {
	directives, err := ParseDirectives(input)
	if err != nil {
		return nil, err
	}
	if len(directives) == 0 {
		return &PatchResult{}, nil
	}

	vfs := NewVFS(rootDir)
	var changes []FileChange

	// PASS 1: Sequential In-Memory Simulation & Atomic Validation
	for idx, dir := range directives {
		switch dir.Type {
		case ActionFile:
			norm := filepath.ToSlash(dir.Path)
			diskPath := filepath.Join(rootDir, filepath.FromSlash(norm))
			_, statErr := os.Stat(diskPath)
			isNew := os.IsNotExist(statErr) && !vfs.updated[norm]

			vfs.WriteFile(norm, []byte(dir.Content))
			if isNew {
				vfs.created[norm] = true
				changes = append(changes, FileChange{
					Path:        norm,
					Type:        ChangeCreate,
					Description: "create file",
					Lines:       len(strings.Split(dir.Content, "\n")),
				})
			} else {
				vfs.updated[norm] = true
				changes = append(changes, FileChange{
					Path:        norm,
					Type:        ChangeUpdate,
					Description: "overwrite file",
					Lines:       len(strings.Split(dir.Content, "\n")),
				})
			}

		case ActionDelete:
			norm := filepath.ToSlash(dir.Path)
			vfs.DeleteFile(norm)
			changes = append(changes, FileChange{
				Path:        norm,
				Type:        ChangeDelete,
				Description: "delete file",
			})

		case ActionReplace:
			norm := filepath.ToSlash(dir.Path)
			contentBytes, readErr := vfs.ReadFile(norm)
			if readErr != nil {
				return nil, fmt.Errorf("directive #%d: target file %q does not exist", idx+1, norm)
			}

			newContent, desc, repErr := executeReplacement(norm, string(contentBytes), dir)
			if repErr != nil {
				return nil, fmt.Errorf("directive #%d on %q failed: %w", idx+1, norm, repErr)
			}

			vfs.WriteFile(norm, []byte(newContent))
			vfs.updated[norm] = true
			changes = append(changes, FileChange{
				Path:        norm,
				Type:        ChangeUpdate,
				Description: desc,
				Lines:       len(strings.Split(newContent, "\n")),
			})
		}
	}

	// PASS 2: Commit staged changes to disk (unless dry run)
	if !opts.DryRun {
		for delPath := range vfs.deleted {
			target := filepath.Join(rootDir, filepath.FromSlash(delPath))
			_ = os.Remove(target)
		}

		for normPath, content := range vfs.files {
			if !vfs.created[normPath] && !vfs.updated[normPath] {
				continue
			}
			target := filepath.Join(rootDir, filepath.FromSlash(normPath))
			if mkErr := os.MkdirAll(filepath.Dir(target), 0755); mkErr != nil {
				return nil, fmt.Errorf("failed creating parent directory for %q: %w", normPath, mkErr)
			}
			if writeErr := os.WriteFile(target, content, 0644); writeErr != nil {
				return nil, fmt.Errorf("failed writing file %q: %w", normPath, writeErr)
			}
		}
	}

	return &PatchResult{Changes: changes}, nil
}

func executeReplacement(filePath, content string, dir Directive) (string, string, error) {
	// 1. Symbol-based AST replacement
	if dir.Symbol != "" {
		res, err := replaceSymbol(filePath, content, dir.Symbol, dir.With)
		if err != nil {
			return "", "", err
		}
		return res, fmt.Sprintf("replace symbol %q", dir.Symbol), nil
	}

	// 2. Search block replacement (5-tier fuzzy match + ellipsis)
	if dir.Search != "" {
		res, err := replaceSearch(content, dir.Search, dir.With)
		if err != nil {
			return "", "", err
		}
		return res, "search & replace block", nil
	}

	// 3. Pattern bounds (from="..." to="...")
	if dir.From != "" && dir.To != "" {
		res, err := replaceFromTo(content, dir.From, dir.To, dir.With)
		if err != nil {
			return "", "", err
		}
		return res, fmt.Sprintf("replace from %q to %q", dir.From, dir.To), nil
	}

	// 4. Anchor insertion (after="...")
	if dir.After != "" {
		res, err := insertAnchor(content, dir.After, dir.With, true)
		if err != nil {
			return "", "", err
		}
		return res, fmt.Sprintf("insert after %q", dir.After), nil
	}

	// 5. Anchor insertion (before="...")
	if dir.Before != "" {
		res, err := insertAnchor(content, dir.Before, dir.With, false)
		if err != nil {
			return "", "", err
		}
		return res, fmt.Sprintf("insert before %q", dir.Before), nil
	}

	// 6. Explicit line ranges (lines="45-60")
	if dir.Lines != "" {
		res, err := replaceLines(content, dir.Lines, dir.With)
		if err != nil {
			return "", "", err
		}
		return res, fmt.Sprintf("replace lines %s", dir.Lines), nil
	}

	return "", "", fmt.Errorf("no valid replacement target specified")
}

func replaceSymbol(filePath, content, symbolName, with string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	var symbols []parser.Symbol
	var err error

	if ext == ".go" {
		symbols, err = parser.ParseGoFile(filePath, []byte(content))
	} else if ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" {
		symbols, err = parser.ParseTypeScriptFile(filePath, []byte(content))
	}

	var match *parser.Symbol
	if err == nil && len(symbols) > 0 {
		target := strings.TrimSpace(symbolName)
		for _, s := range symbols {
			if s.Name == target || s.FullName() == target {
				match = &s
				break
			}
			if s.Receiver != "" && (s.Receiver+"."+s.Name == target || "("+s.Receiver+")."+s.Name == target) {
				match = &s
				break
			}
		}
	}

	lines := strings.Split(content, "\n")

	// AST exact range found
	if match != nil {
		start := match.StartLine - 1
		end := match.EndLine
		if start < 0 {
			start = 0
		}
		if end > len(lines) {
			end = len(lines)
		}
		return spliceLines(lines, start, end, with), nil
	}

	// Generic regex symbol fallback for other languages
	declPattern := regexp.MustCompile(`(?m)^[ \t]*(?:func|function|def|class|type|struct|interface|pub\s+fn|fn)\s+` + regexp.QuoteMeta(symbolName) + `\b`)
	loc := declPattern.FindStringIndex(content)
	if loc == nil {
		return "", fmt.Errorf("symbol %q not found in %s", symbolName, filePath)
	}

	startLine := strings.Count(content[:loc[0]], "\n")
	endLine := findBlockEnd(lines, startLine)

	return spliceLines(lines, startLine, endLine, with), nil
}

func findBlockEnd(lines []string, startIdx int) int {
	braceCount := 0
	foundBrace := false

	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		for _, ch := range line {
			if ch == '{' {
				braceCount++
				foundBrace = true
			} else if ch == '}' {
				braceCount--
			}
		}
		if foundBrace && braceCount <= 0 {
			return i + 1
		}
		if !foundBrace && strings.HasSuffix(strings.TrimSpace(line), ";") {
			return i + 1
		}
	}
	return len(lines)
}

func replaceSearch(content, search, with string) (string, error) {
	// Check for Ellipsis / Wildcard matching
	if containsEllipsisLine(search) {
		return replaceSearchEllipsis(content, search, with)
	}

	// Tier 1: Exact string match
	if idx := strings.Index(content, search); idx != -1 {
		return content[:idx] + with + content[idx+len(search):], nil
	}

	// Tier 2: Line ending normalization (\r\n -> \n)
	normContent := strings.ReplaceAll(content, "\r\n", "\n")
	normSearch := strings.ReplaceAll(search, "\r\n", "\n")
	if idx := strings.Index(normContent, normSearch); idx != -1 {
		return normContent[:idx] + with + normContent[idx+len(normSearch):], nil
	}

	contentLines := strings.Split(normContent, "\n")
	searchLines := strings.Split(normSearch, "\n")

	// Tier 3: Trailing whitespace tolerance
	if res, ok := matchLinesTrailing(contentLines, searchLines, with); ok {
		return res, nil
	}

	// Tier 4: Indentation shift tolerance
	if res, ok := matchLinesIndentation(contentLines, searchLines, with); ok {
		return res, nil
	}

	return "", fmt.Errorf("search block not matched in file")
}

func containsEllipsisLine(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(l)
		if trimmed == "..." || trimmed == "…" {
			return true
		}
	}
	return false
}

func replaceSearchEllipsis(content, search, with string) (string, error) {
	normContent := strings.ReplaceAll(content, "\r\n", "\n")
	contentLines := strings.Split(normContent, "\n")
	rawSearchLines := strings.Split(strings.ReplaceAll(search, "\r\n", "\n"), "\n")

	var segments [][]string
	var current []string

	for _, l := range rawSearchLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "..." || trimmed == "…" {
			if len(current) > 0 {
				segments = append(segments, current)
				current = nil
			}
			continue
		}
		current = append(current, l)
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}

	if len(segments) == 0 {
		return "", fmt.Errorf("search block contains only wildcards")
	}

	type matchSpan struct {
		start int
		end   int
	}

	spans := make([]matchSpan, len(segments))
	currLine := 0

	for sIdx, seg := range segments {
		found := false
		for i := currLine; i <= len(contentLines)-len(seg); i++ {
			if checkSegmentMatch(contentLines[i:i+len(seg)], seg) {
				spans[sIdx] = matchSpan{start: i, end: i + len(seg)}
				currLine = i + len(seg)
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("segment %d in wildcard search block not matched", sIdx+1)
		}
	}

	// Capture wildcards between matched segments
	var wildcards []string
	for i := 0; i < len(spans)-1; i++ {
		wStart := spans[i].end
		wEnd := spans[i+1].start
		if wStart <= wEnd && wEnd <= len(contentLines) {
			wildcards = append(wildcards, strings.Join(contentLines[wStart:wEnd], "\n"))
		} else {
			wildcards = append(wildcards, "")
		}
	}

	finalWith := with
	if containsEllipsisLine(with) {
		withLines := strings.Split(strings.ReplaceAll(with, "\r\n", "\n"), "\n")
		var interpolated []string
		wIdx := 0

		for _, l := range withLines {
			trimmed := strings.TrimSpace(l)
			if trimmed == "..." || trimmed == "…" {
				if wIdx < len(wildcards) {
					if wildcards[wIdx] != "" {
						interpolated = append(interpolated, wildcards[wIdx])
					}
					wIdx++
				}
				continue
			}
			interpolated = append(interpolated, l)
		}
		finalWith = strings.Join(interpolated, "\n")
	}

	overallStart := spans[0].start
	overallEnd := spans[len(spans)-1].end
	return spliceLines(contentLines, overallStart, overallEnd, finalWith), nil
}

func checkSegmentMatch(candidate, pattern []string) bool {
	if len(candidate) != len(pattern) {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		pTrim := strings.TrimRight(pattern[i], " \t\r")
		cTrim := strings.TrimRight(candidate[i], " \t\r")
		if pTrim != cTrim && strings.TrimSpace(pTrim) != strings.TrimSpace(cTrim) {
			return false
		}
	}
	return true
}

func matchLinesTrailing(contentLines, searchLines []string, with string) (string, bool) {
	for i := 0; i <= len(contentLines)-len(searchLines); i++ {
		matched := true
		for j := 0; j < len(searchLines); j++ {
			if strings.TrimRight(contentLines[i+j], " \t\r") != strings.TrimRight(searchLines[j], " \t\r") {
				matched = false
				break
			}
		}
		if matched {
			return spliceLines(contentLines, i, i+len(searchLines), with), true
		}
	}
	return "", false
}

func matchLinesIndentation(contentLines, searchLines []string, with string) (string, bool) {
	if len(searchLines) == 0 {
		return "", false
	}

	for i := 0; i <= len(contentLines)-len(searchLines); i++ {
		sFirstTrim := strings.TrimSpace(searchLines[0])
		cFirstTrim := strings.TrimSpace(contentLines[i])

		if sFirstTrim == "" || sFirstTrim != cFirstTrim {
			continue
		}

		matched := true
		for j := 1; j < len(searchLines); j++ {
			if strings.TrimSpace(contentLines[i+j]) != strings.TrimSpace(searchLines[j]) {
				matched = false
				break
			}
		}

		if matched {
			return spliceLines(contentLines, i, i+len(searchLines), with), true
		}
	}
	return "", false
}

func replaceFromTo(content, from, to, with string) (string, error) {
	lines := strings.Split(content, "\n")
	fromIdx := -1
	toIdx := -1

	fromLower := strings.ToLower(from)
	toLower := strings.ToLower(to)

	for i, l := range lines {
		lLower := strings.ToLower(l)
		if fromIdx == -1 {
			if strings.Contains(lLower, fromLower) {
				fromIdx = i
			}
		} else {
			if strings.Contains(lLower, toLower) {
				toIdx = i + 1
				break
			}
		}
	}

	if fromIdx == -1 {
		return "", fmt.Errorf("starting pattern %q not found", from)
	}
	if toIdx == -1 {
		return "", fmt.Errorf("ending pattern %q not found after %q", to, from)
	}

	return spliceLines(lines, fromIdx, toIdx, with), nil
}

func insertAnchor(content, anchor, with string, after bool) (string, error) {
	lines := strings.Split(content, "\n")
	targetIdx := -1
	aLower := strings.ToLower(anchor)

	for i, l := range lines {
		if strings.Contains(strings.ToLower(l), aLower) {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		return "", fmt.Errorf("anchor pattern %q not found", anchor)
	}

	splicePoint := targetIdx
	if after {
		splicePoint = targetIdx + 1
	}

	return spliceLines(lines, splicePoint, splicePoint, with), nil
}

func replaceLines(content, rangeStr, with string) (string, error) {
	ranges := scanner.ParseLineRanges(rangeStr)
	if len(ranges) == 0 {
		return "", fmt.Errorf("invalid line range %q", rangeStr)
	}

	lines := strings.Split(content, "\n")
	r := ranges[0]

	start := r.Start - 1
	end := r.End
	if start < 0 {
		start = 0
	}
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return "", fmt.Errorf("start line %d greater than end line %d", r.Start, r.End)
	}

	return spliceLines(lines, start, end, with), nil
}

func spliceLines(lines []string, start, end int, replacement string) string {
	var out []string
	out = append(out, lines[:start]...)
	if replacement != "" {
		out = append(out, replacement)
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}
