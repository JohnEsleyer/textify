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
	"github.com/JohnEsleyer/textify/internal/parser"
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
	Context int    // Context lines around query matches (default 10)
	Glob    string // Glob pattern to match multiple files
	From    string // Start pattern for block extraction
	To      string // End pattern for block extraction
	Anchor  string // Anchor pattern
	Def     string // Exact symbol definition requested via AST
	Trace   bool   // Trace execution flow funcA() -> funcB() -> funcC()
}

var (
	// Matches XML-style file/slice tags
	xmlTagRegex = regexp.MustCompile(`(?i)<(?:file|slice|read|get)\s+([^>]+?)(?:/>|>.*?</(?:file|slice|read|get)>|>)`)

	// Matches XML-style project-wide grep/search tags: <grep pattern="foo" context="10" />
	xmlGrepRegex = regexp.MustCompile(`(?i)<(?:grep|search|find)\s+([^>]+?)(?:/>|>.*?</(?:grep|search|find)>|>)`)

	// Matches XML-style <def name="Foo" trace="true" /> tags
	xmlDefRegex  = regexp.MustCompile(`(?i)<(?:def|definition)\s+([^>]+?)(?:/>|>.*?</(?:def|definition)>|>)`)
	xmlAttrTrace = regexp.MustCompile(`(?i)\btrace=["']?(true|false|1|0)["']?`)
	xmlAttrName  = regexp.MustCompile(`(?i)\b(?:name|symbol|struct|func|id)=["']([^"']+)["']`)
	xmlAttrDef   = regexp.MustCompile(`(?i)\b(?:def|definition)=["']([^"']+)["']`)

	xmlAttrPath    = regexp.MustCompile(`(?i)\b(?:path|file|src)=["']([^"']+)["']`)
	xmlAttrLines   = regexp.MustCompile(`(?i)\blines=["']([^"']+)["']`)
	xmlAttrSymbols = regexp.MustCompile(`(?i)\bsymbols?=["']([^"']+)["']`)
	xmlAttrSearch  = regexp.MustCompile(`(?i)\b(?:search|grep|query|pattern|find|around)=["']([^"']+)["']`)
	xmlAttrFrom    = regexp.MustCompile(`(?i)\bfrom=["']([^"']+)["']`)
	xmlAttrTo      = regexp.MustCompile(`(?i)\bto=["']([^"']+)["']`)
	xmlAttrAnchor  = regexp.MustCompile(`(?i)\banchor=["']([^"']+)["']`)
	xmlAttrContext = regexp.MustCompile(`(?i)\bcontext=["'](\d+)["']`)
	xmlAttrGlob    = regexp.MustCompile(`(?i)\bglob=["']([^"']+)["']`)

	// Markdown bullet patterns like: - path/to/file.go:50-120 or - grep: "keyword"
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

		if strings.HasSuffix(part, ":") || strings.HasSuffix(part, "+") || strings.HasSuffix(part, "..") {
			trimmed := strings.TrimRight(part, ":+.")
			val, err := strconv.Atoi(trimmed)
			if err == nil {
				ranges = append(ranges, LineRange{Start: val, End: 0})
			}
			continue
		}

		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "..") {
			trimmed := strings.TrimLeft(part, ":.")
			val, err := strconv.Atoi(trimmed)
			if err == nil {
				ranges = append(ranges, LineRange{Start: 1, End: val})
			}
			continue
		}

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

		val, err := strconv.Atoi(part)
		if err == nil {
			ranges = append(ranges, LineRange{Start: val, End: val})
		}
	}

	return ranges
}

// ParseRequest parses an LLM response containing file or grep requests.
func ParseRequest(input string) []FileRequest {
	var requests []FileRequest
	seen := make(map[string]bool)

	// 0. <def name="..." file="..." trace="true" /> tags
	defMatches := xmlDefRegex.FindAllStringSubmatch(input, -1)
	for _, m := range defMatches {
		attrs := m[1]
		nameMatch := xmlAttrName.FindStringSubmatch(attrs)
		if len(nameMatch) < 2 {
			continue
		}
		pathMatch := xmlAttrPath.FindStringSubmatch(attrs)
		pathVal := ""
		if len(pathMatch) >= 2 {
			pathVal = filepath.ToSlash(strings.TrimSpace(pathMatch[1]))
		}

		traceVal := false
		if traceMatch := xmlAttrTrace.FindStringSubmatch(attrs); len(traceMatch) >= 2 {
			traceVal = traceMatch[1] == "true" || traceMatch[1] == "1"
		}

		req := FileRequest{
			Path:  pathVal,
			Def:   strings.TrimSpace(nameMatch[1]),
			Trace: traceVal,
		}
		key := fmt.Sprintf("DEF|%s|%s|%t", req.Def, req.Path, req.Trace)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	// 1. Project-wide <grep ... /> or <search ... /> tags
	grepMatches := xmlGrepRegex.FindAllStringSubmatch(input, -1)
	for _, m := range grepMatches {
		attrs := m[1]
		searchMatch := xmlAttrSearch.FindStringSubmatch(attrs)
		if len(searchMatch) < 2 {
			continue
		}

		ctx := 10
		if ctxMatch := xmlAttrContext.FindStringSubmatch(attrs); len(ctxMatch) >= 2 {
			ctx, _ = strconv.Atoi(ctxMatch[1])
		}

		globVal := ""
		if globMatch := xmlAttrGlob.FindStringSubmatch(attrs); len(globMatch) >= 2 {
			globVal = filepath.ToSlash(strings.TrimSpace(globMatch[1]))
		}

		req := FileRequest{
			Query:   strings.TrimSpace(searchMatch[1]),
			Context: ctx,
			Glob:    globVal,
		}

		key := fmt.Sprintf("GREP|%s|%d|%s", req.Query, req.Context, req.Glob)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	// 2. File-specific <file ... /> or <slice ... /> tags
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

		if fromMatch := xmlAttrFrom.FindStringSubmatch(attrs); len(fromMatch) >= 2 {
			req.From = strings.TrimSpace(fromMatch[1])
		}

		if toMatch := xmlAttrTo.FindStringSubmatch(attrs); len(toMatch) >= 2 {
			req.To = strings.TrimSpace(toMatch[1])
		}

		if anchorMatch := xmlAttrAnchor.FindStringSubmatch(attrs); len(anchorMatch) >= 2 {
			req.Anchor = strings.TrimSpace(anchorMatch[1])
		}

		if defMatch := xmlAttrDef.FindStringSubmatch(attrs); len(defMatch) >= 2 {
			req.Def = strings.TrimSpace(defMatch[1])
		}
		if traceMatch := xmlAttrTrace.FindStringSubmatch(attrs); len(traceMatch) >= 2 {
			req.Trace = traceMatch[1] == "true" || traceMatch[1] == "1"
		}
		// Allow <file path="..." symbol="Foo" trace="true" /> as an AST request.
		if req.Def == "" && req.Trace && len(req.Symbols) == 1 {
			req.Def = req.Symbols[0]
		}

		ctx := 10
		if ctxMatch := xmlAttrContext.FindStringSubmatch(attrs); len(ctxMatch) >= 2 {
			ctx, _ = strconv.Atoi(ctxMatch[1])
		}
		req.Context = ctx

		key := fmt.Sprintf("%s|%v|%s|%s|%s|%s|%s|%s|%t", req.Path, req.Ranges, strings.Join(req.Symbols, ","), req.Query, req.From, req.To, req.Anchor, req.Def, req.Trace)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	if len(requests) > 0 {
		return requests
	}

	// 3. Fallback to Markdown bullet lists
	mdMatches := mdLinePattern.FindAllStringSubmatch(input, -1)
	for _, m := range mdMatches {
		p := filepath.ToSlash(strings.Trim(strings.TrimSpace(m[1]), "`'\""))
		if strings.HasPrefix(p, "http") || strings.HasPrefix(p, "#") || p == "" {
			continue
		}

		// Check for markdown bullet grep: - grep: "keyword"
		if p == "grep" || p == "search" || p == "find" {
			query := ""
			if len(m) >= 3 && m[2] != "" {
				query = strings.Trim(strings.TrimSpace(m[2]), `"' `)
			}
			if query != "" {
				req := FileRequest{Query: query, Context: 10}
				key := fmt.Sprintf("GREP|%s", query)
				if !seen[key] {
					seen[key] = true
					requests = append(requests, req)
				}
				continue
			}
		}

		// Check for markdown bullet definitions/traces: - def: "Foo" / - trace: Foo
		if p == "def" || p == "definition" || p == "trace" || p == "flow" {
			name := ""
			if len(m) >= 3 && m[2] != "" {
				name = strings.Trim(strings.TrimSpace(m[2]), `"' `)
			}
			if name != "" {
				req := FileRequest{
					Def:   name,
					Trace: p == "trace" || p == "flow",
				}
				key := fmt.Sprintf("DEF|%s|%t", req.Def, req.Trace)
				if !seen[key] {
					seen[key] = true
					requests = append(requests, req)
				}
				continue
			}
		}

		req := FileRequest{Path: p, Context: 10}

		if len(m) >= 3 && m[2] != "" {
			detail := strings.TrimSpace(m[2])

			switch {
			case strings.HasPrefix(detail, "search:") || strings.HasPrefix(detail, "grep:") || strings.HasPrefix(detail, "find:"):
				val := strings.TrimSpace(detail[strings.Index(detail, ":")+1:])
				req.Query = strings.Trim(val, `"' `)
			case strings.HasPrefix(detail, "def:") || strings.HasPrefix(detail, "definition:"):
				val := strings.TrimSpace(detail[strings.Index(detail, ":")+1:])
				req.Def = strings.Trim(val, `"' `)
			case strings.HasPrefix(detail, "trace:") || strings.HasPrefix(detail, "flow:"):
				val := strings.TrimSpace(detail[strings.Index(detail, ":")+1:])
				req.Def = strings.Trim(val, `"' `)
				req.Trace = true
			case strings.HasPrefix(detail, "symbols:") || strings.HasPrefix(detail, "symbol:"):
				val := strings.TrimSpace(detail[strings.Index(detail, ":")+1:])
				for _, sym := range strings.Split(val, ",") {
					req.Symbols = append(req.Symbols, strings.TrimSpace(sym))
				}
			case strings.HasPrefix(detail, "from:"):
				// format: (from: "A", to: "B")
				parts := strings.Split(detail, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "from:") {
						req.From = strings.Trim(strings.TrimSpace(part[5:]), `"' `)
					} else if strings.HasPrefix(part, "to:") {
						req.To = strings.Trim(strings.TrimSpace(part[3:]), `"' `)
					}
				}
			default:
				detail = strings.TrimPrefix(detail, "lines")
				req.Ranges = ParseLineRanges(detail)
				if len(req.Ranges) > 0 {
					req.Start = req.Ranges[0].Start
					req.End = req.Ranges[0].End
				}
			}
		}

		key := fmt.Sprintf("%s|%v|%s|%s|%s|%s|%s|%s|%t", req.Path, req.Ranges, strings.Join(req.Symbols, ","), req.Query, req.From, req.To, req.Anchor, req.Def, req.Trace)
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

	// Normalize trace requests that used symbol="..." without def="...".
	for i := range requests {
		if requests[i].Def == "" && requests[i].Trace && len(requests[i].Symbols) == 1 {
			requests[i].Def = requests[i].Symbols[0]
		}
	}

	// Build the AST index once if any request needs exact definitions or traces.
	var astIndex *parser.CodebaseIndex
	for _, req := range requests {
		if req.Def != "" || req.Trace {
			astIndex, _ = parser.BuildIndex(rootPath, parser.ParseLanguages("all"))
			break
		}
	}

	// Expand requests (globs or project-wide greps)
	var expandedRequests []FileRequest
	for _, req := range requests {
		// AST definition/trace requests are resolved directly against the index.
		if req.Def != "" || req.Trace {
			expandedRequests = append(expandedRequests, req)
			continue
		}

		// Project-wide grep (no specific file path)
		if req.Path == "" && req.Query != "" {
			pattern := req.Glob
			if pattern == "" {
				pattern = "*"
			}
			matchedFiles := findMatchingFiles(rootPath, pattern)
			for _, mf := range matchedFiles {
				absPath := filepath.Join(rootPath, filepath.FromSlash(mf))
				if isBin, err := fileutil.IsBinary(absPath); err != nil || isBin {
					continue
				}
				data, err := os.ReadFile(absPath)
				if err != nil {
					continue
				}
				// Only include file if it actually contains the query
				if fileMatchesQuery(string(data), req.Query) {
					clone := req
					clone.Path = mf
					expandedRequests = append(expandedRequests, clone)
				}
			}
			continue
		}

		// Explicit glob path
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
		// AST-backed exact definition and optional execution flow trace.
		if req.Def != "" {
			if astIndex == nil {
				var idxErr error
				astIndex, idxErr = parser.BuildIndex(rootPath, parser.ParseLanguages("all"))
				if idxErr != nil || astIndex == nil {
					if !opts.Stdout {
						fmt.Printf("⚠️ Warning: Could not build AST index: %v\n", idxErr)
					}
					continue
				}
			}

			rendered := buildASTContent(astIndex, req, opts.RedactSecrets)
			formatted := renderFormattedEntry(rendered.label, rendered.content, format, rendered.startLine, rendered.endLine)

			n, writeErr := bufWriter.WriteString(formatted)
			if writeErr != nil {
				return nil, writeErr
			}

			result.FilesCount++
			result.TotalBytes += int64(n)
			result.RedactedSecrets += rendered.redacted

			if !opts.Stdout {
				desc := fmt.Sprintf("definition: %s", req.Def)
				if req.Trace {
					desc = fmt.Sprintf("definition + trace: %s", req.Def)
				}
				fmt.Printf("Added: %s (%s)\n", rendered.label, desc)
			}
			continue
		}

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
			if req.From != "" {
				desc = fmt.Sprintf("from: %q to: %q", req.From, req.To)
			} else if req.Anchor != "" {
				desc = fmt.Sprintf("anchor: %q", req.Anchor)
			} else if req.Query != "" {
				desc = fmt.Sprintf("find: %q (±%d lines)", req.Query, req.Context)
			} else if len(req.Symbols) > 0 {
				desc = fmt.Sprintf("symbols: %s", strings.Join(req.Symbols, ", "))
			} else if len(req.Ranges) > 0 {
				var rStrs []string
				for _, r := range req.Ranges {
					if r.End > 0 {
						rStrs = append(rStrs, fmt.Sprintf("%d-%d", r.Start, r.End))
					} else {
						rStrs = append(rStrs, fmt.Sprintf("%d-EOF", r.Start))
					}
				}
				desc = fmt.Sprintf("lines %s", strings.Join(rStrs, ", "))
			}
			fmt.Printf("Added: %s (%s)\n", req.Path, desc)
		}
	}

	result.ApproxTokens = int(result.TotalBytes / 4)
	return result, nil
}

type astRenderResult struct {
	content   string
	label     string
	startLine int
	endLine   int
	redacted  int
}

// buildASTContent resolves a symbol against the AST index and renders its
// definition with exact source line numbers. When req.Trace is set, the
// execution flow or type dependency hierarchy is prepended.
func buildASTContent(index *parser.CodebaseIndex, req FileRequest, redact bool) astRenderResult {
	var sb strings.Builder

	if req.Trace {
		sb.WriteString(index.TraceFlow(req.Def, 5))
		sb.WriteString("\n")
	}

	syms := index.FindDefinition(req.Def, req.Path)
	firstFile := ""
	startLine := 0
	endLine := 0

	if len(syms) == 0 {
		sb.WriteString(fmt.Sprintf("/* Symbol %q not found in AST index */\n", req.Def))
	} else {
		firstFile = syms[0].File
		for _, sym := range syms {
			sb.WriteString(fmt.Sprintf("/* Definition: %s %s [%s:%d-%d] */\n", sym.Kind, sym.FullName(), sym.File, sym.StartLine, sym.EndLine))
			for offset, line := range strings.Split(sym.Body, "\n") {
				sb.WriteString(fmt.Sprintf("%5d | %s\n", sym.StartLine+offset, line))
			}
			sb.WriteString("\n")

			if startLine == 0 || sym.StartLine < startLine {
				startLine = sym.StartLine
			}
			if sym.EndLine > endLine {
				endLine = sym.EndLine
			}
		}
	}

	label := req.Path
	if label == "" {
		label = firstFile
	}
	if label == "" {
		label = req.Def
	}

	content := sb.String()
	redacted := 0
	if redact {
		content, redacted = fileutil.RedactSecrets(content)
	}

	if startLine == 0 {
		startLine = 1
	}
	if endLine == 0 {
		endLine = 1
	}

	return astRenderResult{
		content:   content,
		label:     label,
		startLine: startLine,
		endLine:   endLine,
		redacted:  redacted,
	}
}

func fileMatchesQuery(content, query string) bool {
	if query == "" {
		return true
	}
	if re, err := regexp.Compile("(?i)" + query); err == nil {
		return re.MatchString(content)
	}
	return strings.Contains(strings.ToLower(content), strings.ToLower(query))
}

func extractSections(content string, req FileRequest) (string, int, int) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	if totalLines == 0 {
		return "", 0, 0
	}

	var intervals []LineRange

	// 1. Pattern-bounded range: from="..." to="..."
	if req.From != "" {
		fromRe, fromErr := regexp.Compile("(?i)" + req.From)
		toRe, toErr := regexp.Compile("(?i)" + req.To)
		fromLower := strings.ToLower(req.From)
		toLower := strings.ToLower(req.To)

		inBlock := false
		startLine := 1

		for idx, line := range lines {
			lineNum := idx + 1
			lineLower := strings.ToLower(line)

			if !inBlock {
				matched := false
				if strings.Contains(lineLower, fromLower) {
					matched = true
				} else if fromErr == nil {
					matched = fromRe.MatchString(line)
				}
				if matched {
					inBlock = true
					startLine = lineNum
				}
			} else {
				matched := false
				if req.To != "" {
					if strings.Contains(lineLower, toLower) {
						matched = true
					} else if toErr == nil {
						matched = toRe.MatchString(line)
					}
				}
				if matched {
					intervals = append(intervals, LineRange{Start: startLine, End: lineNum})
					inBlock = false
				}
			}
		}
		if inBlock {
			intervals = append(intervals, LineRange{Start: startLine, End: totalLines})
		}
	}

	// 2. Anchor with forward window
	if req.Anchor != "" {
		ctx := req.Context
		if ctx <= 0 {
			ctx = 15
		}
		aLower := strings.ToLower(req.Anchor)
		for idx, line := range lines {
			if strings.Contains(strings.ToLower(line), aLower) {
				lineNum := idx + 1
				st := lineNum - 2
				if st < 1 {
					st = 1
				}
				en := lineNum + ctx
				if en > totalLines {
					en = totalLines
				}
				intervals = append(intervals, LineRange{Start: st, End: en})
				break
			}
		}
	}

	// 3. Keyword / Query search with context
	if req.Query != "" {
		ctx := req.Context
		if ctx <= 0 {
			ctx = 10
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

	// 4. Symbol Extraction
	if len(req.Symbols) > 0 {
		symIntervals := findSymbolIntervals(lines, req.Symbols)
		intervals = append(intervals, symIntervals...)
	}

	// 5. Explicit Line Ranges
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

	// If no filters matched or specified, return full file
	if len(intervals) == 0 {
		if req.Query != "" || req.From != "" || req.Anchor != "" || len(req.Symbols) > 0 {
			return fmt.Sprintf("/* No matches found in %s */\n", req.Path), 1, totalLines
		}
		var sb strings.Builder
		for i, line := range lines {
			sb.WriteString(fmt.Sprintf("%5d | %s\n", i+1, line))
		}
		return sb.String(), 1, totalLines
	}

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
