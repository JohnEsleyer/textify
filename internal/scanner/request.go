package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
)

// LineRange represents a 1-based start and end line range.
type LineRange struct {
	Start int
	End   int // 0 means through end of file
}

// FileRequest describes a target file and specific sub-slices or queries requested.
type FileRequest struct {
	Path    string
	Ranges  []LineRange
	Start   int // Legacy compatibility (first range start)
	End     int // Legacy compatibility (first range end)
	Symbols []string
	Query   string // Text or regex pattern to search for
	Context int    // Context lines around query matches (default 5)
	Glob    string // Glob pattern to match multiple files
}

var (
	// Matches XML-style tags: <file ... />, <slice ... />, <read ... />, <get ... />
	xmlTagRegex    = regexp.MustCompile(`(?i)<(?:file|slice|read|get)\s+([^>]+?)(?:/>|>.*?</(?:file|slice|read|get)>|>)`)
	xmlAttrPath    = regexp.MustCompile(`(?i)\b(?:path|file|src)=["']([^"']+)["']`)
	xmlAttrLines   = regexp.MustCompile(`(?i)\blines=["']([^"']+)["']`)
	xmlAttrSymbols = regexp.MustCompile(`(?i)\bsymbols?=["']([^"']+)["']`)
	xmlAttrSearch  = regexp.MustCompile(`(?i)\b(?:search|grep|query|pattern)=["']([^"']+)["']`)
	xmlAttrContext = regexp.MustCompile(`(?i)\bcontext=["'](\d+)["']`)
	xmlAttrGlob    = regexp.MustCompile(`(?i)\bglob=["']([^"']+)["']`)

	// Markdown bullet patterns like: - path/to/file.go:50-120 or - `path/to/file.go`:10-20, 50-80
	mdLinePattern = regexp.MustCompile(`(?m)^\s*(?:[-*+]|\d+\.)(?:\s+\[[ xX]\])?\s+` + "`?" + `([^\s:` + "`" + `]+)` + "`?" + `(?:\s*[:(]\s*([^\r\n)]+)\)?)?`)
)

// ParseLineRanges parses strings like "50-120", "10-20, 50-80", "100:", ":50", or "42".
func ParseLineRanges(s string) []LineRange {
	parts := strings.Split(s, ",")
	var ranges []LineRange

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Open-ended range like "100:" or "100+" or "100.."
		if strings.HasSuffix(part, ":") || strings.HasSuffix(part, "+") || strings.HasSuffix(part, "..") {
			trimmed := strings.TrimRight(part, ":+.")
			val, err := strconv.Atoi(trimmed)
			if err == nil {
				ranges = append(ranges, LineRange{Start: val, End: 0})
			}
			continue
		}

		// Leading open range like ":50" or "..50"
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "..") {
			trimmed := strings.TrimLeft(part, ":.")
			val, err := strconv.Atoi(trimmed)
			if err == nil {
				ranges = append(ranges, LineRange{Start: 1, End: val})
			}
			continue
		}

		// Standard range like "50-120"
		if idx := strings.IndexAny(part, "-:"); idx != -1 {
			startStr := strings.TrimSpace(part[:idx])
			endStr := strings.TrimSpace(part[idx+1:])
			startVal, err1 := strconv.Atoi(startStr)
			endVal, err2 := strconv.Atoi(endStr)
			if err1 == nil && err2 == nil {
				ranges = append(ranges, LineRange{Start: startVal, End: endVal})
			} else if err1 == nil {
				ranges = append(ranges, LineRange{Start: startVal, End: 0})
			}
			continue
		}

		// Single line like "42"
		val, err := strconv.Atoi(part)
		if err == nil {
			ranges = append(ranges, LineRange{Start: val, End: val})
		}
	}

	return ranges
}

// ParseRequest parses an LLM response containing file requests.
// Supports rich XML tags and various Markdown list formats.
func ParseRequest(input string) []FileRequest {
	var requests []FileRequest
	seen := make(map[string]bool)

	// 1. Try XML Tag Matching first
	matches := xmlTagRegex.FindAllStringSubmatch(input, -1)
	for _, m := range matches {
		attrs := m[1]
		pathMatch := xmlAttrPath.FindStringSubmatch(attrs)
		globMatch := xmlAttrGlob.FindStringSubmatch(attrs)

		targetPath := ""
		if len(pathMatch) >= 2 {
			targetPath = filepath.ToSlash(strings.TrimSpace(pathMatch[1]))
		}

		targetGlob := ""
		if len(globMatch) >= 2 {
			targetGlob = filepath.ToSlash(strings.TrimSpace(globMatch[1]))
		}

		if targetPath == "" && targetGlob == "" {
			continue
		}
		if targetPath == "" {
			targetPath = targetGlob
		}

		req := FileRequest{
			Path: targetPath,
			Glob: targetGlob,
		}

		if linesMatch := xmlAttrLines.FindStringSubmatch(attrs); len(linesMatch) >= 2 {
			req.Ranges = ParseLineRanges(linesMatch[1])
			if len(req.Ranges) > 0 {
				req.Start = req.Ranges[0].Start
				req.End = req.Ranges[0].End
			}
		}

		if symMatch := xmlAttrSymbols.FindStringSubmatch(attrs); len(symMatch) >= 2 {
			for _, s := range strings.Split(symMatch[1], ",") {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					req.Symbols = append(req.Symbols, trimmed)
				}
			}
		}

		if searchMatch := xmlAttrSearch.FindStringSubmatch(attrs); len(searchMatch) >= 2 {
			req.Query = strings.TrimSpace(searchMatch[1])
		}

		if ctxMatch := xmlAttrContext.FindStringSubmatch(attrs); len(ctxMatch) >= 2 {
			ctx, _ := strconv.Atoi(ctxMatch[1])
			req.Context = ctx
		}

		key := fmt.Sprintf("%s|%v|%s|%s", req.Path, req.Ranges, strings.Join(req.Symbols, ","), req.Query)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	if len(requests) > 0 {
		return requests
	}

	// 2. Fallback to Markdown bullet lists
	mdMatches := mdLinePattern.FindAllStringSubmatch(input, -1)
	for _, m := range mdMatches {
		p := filepath.ToSlash(strings.Trim(strings.TrimSpace(m[1]), "`'\""))
		if strings.HasPrefix(p, "http") || strings.HasPrefix(p, "#") || p == "" {
			continue
		}

		req := FileRequest{Path: p}

		if len(m) >= 3 && m[2] != "" {
			detail := strings.TrimSpace(m[2])

			// Check for search / grep pattern in bullet: (grep: "foo")
			if strings.HasPrefix(detail, "search:") || strings.HasPrefix(detail, "grep:") {
				val := strings.TrimSpace(detail[strings.Index(detail, ":")+1:])
				req.Query = strings.Trim(val, `"' `)
			} else if strings.HasPrefix(detail, "symbols:") || strings.HasPrefix(detail, "symbol:") {
				val := strings.TrimSpace(detail[strings.Index(detail, ":")+1:])
				for _, sym := range strings.Split(val, ",") {
					req.Symbols = append(req.Symbols, strings.TrimSpace(sym))
				}
			} else {
				detail = strings.TrimPrefix(detail, "lines")
				req.Ranges = ParseLineRanges(detail)
				if len(req.Ranges) > 0 {
					req.Start = req.Ranges[0].Start
					req.End = req.Ranges[0].End
				}
			}
		}

		key := fmt.Sprintf("%s|%v|%s|%s", req.Path, req.Ranges, strings.Join(req.Symbols, ","), req.Query)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	return requests
}

// LoadRequests reads the requested files, extracts line slices, searches, or symbols,
// applies formatting and secret redaction, and writes the output.
func LoadRequests(rootPath string, cfg *config.Config, requests []FileRequest, writer io.Writer, opts ScanOptions) (*ScanResult, error) {
	bufWriter := bufio.NewWriter(writer)
	defer bufWriter.Flush()

	format := "raw"
	if cfg.Format != "" {
		format = cfg.Format
	}
	if opts.Format != "" {
		format = opts.Format
	}

	result := &ScanResult{}

	// Expand globs if any request targets multiple files
	var expandedRequests []FileRequest
	for _, req := range requests {
		if req.Glob != "" || strings.ContainsAny(req.Path, "*?[") {
			pattern := req.Glob
			if pattern == "" {
				pattern = req.Path
			}
			matchedFiles := findMatchingFiles(rootPath, pattern)
			for _, mf := range matchedFiles {
				clone := req
				clone.Path = mf
				clone.Glob = ""
				expandedRequests = append(expandedRequests, clone)
			}
		} else {
			expandedRequests = append(expandedRequests, req)
		}
	}

	for _, req := range expandedRequests {
		absPath := filepath.Join(rootPath, filepath.FromSlash(req.Path))

		isBin, err := fileutil.IsBinary(absPath)
		if err != nil || isBin {
			continue
		}

		rawBytes, err := os.ReadFile(absPath)
		if err != nil {
			if !opts.Stdout {
				fmt.Printf("⚠️ Warning: Could not read requested file '%s': %v\n", req.Path, err)
			}
			continue
		}

		content, startLine, endLine := extractSections(string(rawBytes), req)

		redactedCount := 0
		if opts.RedactSecrets {
			content, redactedCount = fileutil.RedactSecrets(content)
		}

		formatted := renderFormattedEntry(req.Path, content, format, startLine, endLine)

		n, writeErr := bufWriter.WriteString(formatted)
		if writeErr != nil {
			return nil, writeErr
		}

		result.FilesCount++
		result.TotalBytes += int64(n)
		result.RedactedSecrets += redactedCount

		if !opts.Stdout {
			desc := "full"
			if len(req.Ranges) > 0 {
				var rStrs []string
				for _, r := range req.Ranges {
					if r.End > 0 {
						rStrs = append(rStrs, fmt.Sprintf("%d-%d", r.Start, r.End))
					} else {
						rStrs = append(rStrs, fmt.Sprintf("%d-EOF", r.Start))
					}
				}
				desc = fmt.Sprintf("lines %s", strings.Join(rStrs, ", "))
			} else if req.Query != "" {
				desc = fmt.Sprintf("search: %q", req.Query)
			} else if len(req.Symbols) > 0 {
				desc = fmt.Sprintf("symbols: %s", strings.Join(req.Symbols, ", "))
			}
			fmt.Printf("Added: %s (%s)\n", req.Path, desc)
		}
	}

	result.ApproxTokens = int(result.TotalBytes / 4)
	return result, nil
}

func extractSections(content string, req FileRequest) (string, int, int) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	if totalLines == 0 {
		return "", 0, 0
	}

	var intervals []LineRange

	// 1. Text Search / Grep with context
	if req.Query != "" {
		ctx := req.Context
		if ctx <= 0 {
			ctx = 5
		}
		qLower := strings.ToLower(req.Query)
		re, reErr := regexp.Compile("(?i)" + req.Query)

		for idx, line := range lines {
			matched := false
			if reErr == nil {
				matched = re.MatchString(line)
			} else {
				matched = strings.Contains(strings.ToLower(line), qLower)
			}
			if matched {
				lineNum := idx + 1
				st := lineNum - ctx
				if st < 1 {
					st = 1
				}
				en := lineNum + ctx
				if en > totalLines {
					en = totalLines
				}
				intervals = append(intervals, LineRange{Start: st, End: en})
			}
		}
	}

	// 2. Symbol Extraction
	if len(req.Symbols) > 0 {
		symIntervals := findSymbolIntervals(lines, req.Symbols)
		intervals = append(intervals, symIntervals...)
	}

	// 3. Explicit Line Ranges
	if len(req.Ranges) > 0 {
		for _, r := range req.Ranges {
			st := r.Start
			en := r.End
			if st < 1 {
				st = 1
			}
			if en <= 0 || en > totalLines {
				en = totalLines
			}
			if en < st {
				en = st
			}
			intervals = append(intervals, LineRange{Start: st, End: en})
		}
	}

	// If no specific intervals were requested or matched, return the full file
	if len(intervals) == 0 {
		if req.Query != "" || len(req.Symbols) > 0 {
			return fmt.Sprintf("/* No matches found in %s */\n", req.Path), 1, totalLines
		}
		var sb strings.Builder
		for i, line := range lines {
			sb.WriteString(fmt.Sprintf("%5d | %s\n", i+1, line))
		}
		return sb.String(), 1, totalLines
	}

	// Merge overlapping and contiguous intervals
	merged := mergeRanges(intervals)

	var sb strings.Builder
	minLine := merged[0].Start
	maxLine := merged[len(merged)-1].End

	for idx, r := range merged {
		if idx > 0 {
			prevEnd := merged[idx-1].End
			if r.Start > prevEnd+1 {
				sb.WriteString(fmt.Sprintf("      ... [skipped lines %d-%d] ...\n", prevEnd+1, r.Start-1))
			}
		}
		for i := r.Start - 1; i < r.End && i < totalLines; i++ {
			sb.WriteString(fmt.Sprintf("%5d | %s\n", i+1, lines[i]))
		}
	}

	return sb.String(), minLine, maxLine
}

func mergeRanges(ranges []LineRange) []LineRange {
	if len(ranges) == 0 {
		return nil
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].Start < ranges[j].Start
	})

	var merged []LineRange
	curr := ranges[0]

	for i := 1; i < len(ranges); i++ {
		next := ranges[i]
		if next.Start <= curr.End+1 {
			if next.End > curr.End {
				curr.End = next.End
			}
		} else {
			merged = append(merged, curr)
			curr = next
		}
	}
	merged = append(merged, curr)
	return merged
}

func findSymbolIntervals(lines []string, symbols []string) []LineRange {
	var intervals []LineRange
	prefixes := []string{
		"func ", "fn ", "pub fn ", "type ", "struct ", "interface ",
		"class ", "def ", "trait ", "enum ", "export function ", "export class ",
	}

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		isDecl := false
		for _, pfx := range prefixes {
			if strings.HasPrefix(trimmed, pfx) {
				isDecl = true
				break
			}
		}
		if !isDecl {
			continue
		}

		for _, sym := range symbols {
			if strings.Contains(trimmed, sym) {
				startLine := idx + 1
				endLine := startLine

				// Walk forward to find declaration end
				braceCount := 0
				foundBrace := false
				for j := idx; j < len(lines); j++ {
					l := lines[j]
					for _, ch := range l {
						if ch == '{' {
							braceCount++
							foundBrace = true
						} else if ch == '}' {
							braceCount--
						}
					}
					if foundBrace && braceCount <= 0 {
						endLine = j + 1
						break
					}
					if !foundBrace && strings.HasSuffix(strings.TrimSpace(l), ";") {
						endLine = j + 1
						break
					}
				}
				intervals = append(intervals, LineRange{Start: startLine, End: endLine})
				break
			}
		}
	}
	return intervals
}

func findMatchingFiles(rootPath, pattern string) []string {
	var matches []string
	_ = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		matched, _ := filepath.Match(pattern, rel)
		if !matched {
			matched, _ = filepath.Match(pattern, filepath.Base(rel))
		}
		if matched {
			matches = append(matches, rel)
		}
		return nil
	})
	return matches
}