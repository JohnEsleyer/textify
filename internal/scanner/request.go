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

// EntrypointReq describes an entrypoint symbol inside a feature-context query.
type EntrypointReq struct {
	File   string
	Symbol string
	Depth  int
}

// FeatureContextReq represents a 360-degree composite query bundle.
type FeatureContextReq struct {
	Lang        string
	TargetTypes []string
	Entrypoints []EntrypointReq
	Touchpoints []string
	ForPatch    bool
}

// FileRequest describes a target file and specific sub-slices or queries requested.
type FileRequest struct {
	Path       string
	Ranges     []LineRange
	Start      int // Legacy compatibility (first range start)
	End        int // Legacy compatibility (first range end)
	Symbols    []string
	Query      string // Text or regex pattern to search for
	Context    int    // Context lines around query matches (default 10)
	Glob       string // Glob pattern to match multiple files
	From       string // Start pattern for block extraction
	To         string // End pattern for block extraction
	Anchor     string // Anchor pattern
	Def        string // Exact symbol definition requested via AST
	Trace      bool   // Trace execution flow
	Depth      int    // Trace depth
	Case       string // AST switch-case branch selector
	Fn         string // Function containing the switch-case
	Struct     string // Struct name for method selection
	Methods    []string
	XRef       string // Cross-reference symbol
	XRefAction string // "write", "read", "call", "all"
	ForPatch   bool   // Output ready-to-use block without line numbers
	FeatureCtx *FeatureContextReq
}

var (
	xmlTagRegex            = regexp.MustCompile(`(?i)<(?:file|slice|read|get)\s+([^>]+?)(?:/>|>.*?</(?:file|slice|read|get)>|>)`)
	xmlGrepRegex           = regexp.MustCompile(`(?i)<(?:grep|search|find)\s+([^>]+?)(?:/>|>.*?</(?:grep|search|find)>|>)`)
	xmlDefRegex            = regexp.MustCompile(`(?i)<(?:def|definition)\s+([^>]+?)(?:/>|>.*?</(?:def|definition)>|>)`)
	xmlTraceRegex          = regexp.MustCompile(`(?i)<(?:trace|flow)\s+([^>]+?)(?:/>|>.*?</(?:trace|flow)>|>)`)
	xmlSelectRegex         = regexp.MustCompile(`(?i)<(?:select|case)\s+([^>]+?)(?:/>|>.*?</(?:select|case)>|>)`)
	xmlXrefRegex           = regexp.MustCompile(`(?i)<(?:xref|deps-of|references)\s+([^>]+?)(?:/>|>.*?</(?:xref|deps-of|references)>|>)`)
	xmlFeatureContextRegex = regexp.MustCompile(`(?is)<feature-context(?:\s+([^>]*))?>(.*?)</feature-context>`)

	xmlAttrTrace    = regexp.MustCompile(`(?i)\btrace=["']?(true|false|1|0)["']?`)
	xmlAttrName     = regexp.MustCompile(`(?i)\b(?:name|symbol|struct|func|id|pattern)=["']([^"']+)["']`)
	xmlAttrDef      = regexp.MustCompile(`(?i)\b(?:def|definition)=["']([^"']+)["']`)
	xmlAttrPath     = regexp.MustCompile(`(?i)\b(?:path|file|src)=["']([^"']+)["']`)
	xmlAttrLines    = regexp.MustCompile(`(?i)\blines=["']([^"']+)["']`)
	xmlAttrSymbols  = regexp.MustCompile(`(?i)\bsymbols?=["']([^"']+)["']`)
	xmlAttrSearch   = regexp.MustCompile(`(?i)\b(?:search|grep|query|pattern|find|around)=["']([^"']+)["']`)
	xmlAttrFrom     = regexp.MustCompile(`(?i)\bfrom=["']([^"']+)["']`)
	xmlAttrTo       = regexp.MustCompile(`(?i)\bto=["']([^"']+)["']`)
	xmlAttrAnchor   = regexp.MustCompile(`(?i)\banchor=["']([^"']+)["']`)
	xmlAttrContext  = regexp.MustCompile(`(?i)\bcontext=["'](\d+)["']`)
	xmlAttrGlob     = regexp.MustCompile(`(?i)\bglob=["']([^"']+)["']`)
	xmlAttrForPatch = regexp.MustCompile(`(?i)\b(?:for-patch|anchors|patch-ready)=["']?(true|false|1|0)["']?`)
	xmlAttrCase     = regexp.MustCompile(`(?i)\bcase=["']([^"']+)["']`)
	xmlAttrFn       = regexp.MustCompile(`(?i)\b(?:fn|func|function|method)=["']([^"']+)["']`)
	xmlAttrStruct   = regexp.MustCompile(`(?i)\b(?:struct|class)=["']([^"']+)["']`)
	xmlAttrMethods  = regexp.MustCompile(`(?i)\bmethods=["']([^"']+)["']`)
	xmlAttrDepth    = regexp.MustCompile(`(?i)\bdepth=["'](\d+)["']`)
	xmlAttrLang     = regexp.MustCompile(`(?i)\blang(?:uage)?=["']([^"']+)["']`)
	xmlAttrAction   = regexp.MustCompile(`(?i)\baction=["']([^"']+)["']`)

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

// ParseRequest parses an LLM response containing targeted requests.
func ParseRequest(input string) []FileRequest {
	var requests []FileRequest
	seen := make(map[string]bool)

	// 1. <feature-context> tags
	for _, m := range xmlFeatureContextRegex.FindAllStringSubmatch(input, -1) {
		attrs := m[1]
		body := m[2]
		lang := "all"
		if lMatch := xmlAttrLang.FindStringSubmatch(attrs); len(lMatch) >= 2 {
			lang = lMatch[1]
		}
		forPatch := false
		if fpMatch := xmlAttrForPatch.FindStringSubmatch(attrs); len(fpMatch) >= 2 {
			forPatch = fpMatch[1] == "true" || fpMatch[1] == "1"
		}
		fc := &FeatureContextReq{
			Lang:     lang,
			ForPatch: forPatch,
		}

		targetTypeRe := regexp.MustCompile(`(?i)<(?:target-type|target)\s+([^>]+?)(?:/>|>)`)
		for _, tm := range targetTypeRe.FindAllStringSubmatch(body, -1) {
			if nm := xmlAttrName.FindStringSubmatch(tm[1]); len(nm) >= 2 {
				fc.TargetTypes = append(fc.TargetTypes, strings.TrimSpace(nm[1]))
			}
		}

		epRe := regexp.MustCompile(`(?i)<entrypoint\s+([^>]+?)(?:/>|>)`)
		for _, em := range epRe.FindAllStringSubmatch(body, -1) {
			epAttrs := em[1]
			epFile := ""
			if pm := xmlAttrPath.FindStringSubmatch(epAttrs); len(pm) >= 2 {
				epFile = filepath.ToSlash(strings.TrimSpace(pm[1]))
			}
			epSym := ""
			if sm := xmlAttrSymbols.FindStringSubmatch(epAttrs); len(sm) >= 2 {
				epSym = strings.TrimSpace(sm[1])
			} else if nm := xmlAttrName.FindStringSubmatch(epAttrs); len(nm) >= 2 {
				epSym = strings.TrimSpace(nm[1])
			}
			epDepth := 2
			if dm := xmlAttrDepth.FindStringSubmatch(epAttrs); len(dm) >= 2 {
				epDepth, _ = strconv.Atoi(dm[1])
			}
			if epSym != "" {
				fc.Entrypoints = append(fc.Entrypoints, EntrypointReq{
					File:   epFile,
					Symbol: epSym,
					Depth:  epDepth,
				})
			}
		}

		tpRe := regexp.MustCompile(`(?i)<(?:touchpoints|touchpoint)\s+([^>]+?)(?:/>|>)`)
		for _, tm := range tpRe.FindAllStringSubmatch(body, -1) {
			if sm := xmlAttrSearch.FindStringSubmatch(tm[1]); len(sm) >= 2 {
				fc.Touchpoints = append(fc.Touchpoints, strings.TrimSpace(sm[1]))
			} else if nm := xmlAttrName.FindStringSubmatch(tm[1]); len(nm) >= 2 {
				fc.Touchpoints = append(fc.Touchpoints, strings.TrimSpace(nm[1]))
			}
		}

		requests = append(requests, FileRequest{FeatureCtx: fc})
	}

	// 2. <trace ... /> tags
	for _, m := range xmlTraceRegex.FindAllStringSubmatch(input, -1) {
		attrs := m[1]
		sym := ""
		if sm := xmlAttrSymbols.FindStringSubmatch(attrs); len(sm) >= 2 {
			sym = strings.TrimSpace(sm[1])
		} else if nm := xmlAttrName.FindStringSubmatch(attrs); len(nm) >= 2 {
			sym = strings.TrimSpace(nm[1])
		}
		if sym == "" {
			continue
		}

		depth := 2
		if dm := xmlAttrDepth.FindStringSubmatch(attrs); len(dm) >= 2 {
			depth, _ = strconv.Atoi(dm[1])
		}

		forPatch := false
		if fpMatch := xmlAttrForPatch.FindStringSubmatch(attrs); len(fpMatch) >= 2 {
			forPatch = fpMatch[1] == "true" || fpMatch[1] == "1"
		}

		pathVal := ""
		if pm := xmlAttrPath.FindStringSubmatch(attrs); len(pm) >= 2 {
			pathVal = filepath.ToSlash(strings.TrimSpace(pm[1]))
		}

		req := FileRequest{
			Path:     pathVal,
			Def:      sym,
			Trace:    true,
			Depth:    depth,
			ForPatch: forPatch,
		}
		key := fmt.Sprintf("TRACE|%s|%d|%t", sym, depth, forPatch)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	// 3. <select ... /> or <case ... /> tags
	for _, m := range xmlSelectRegex.FindAllStringSubmatch(input, -1) {
		attrs := m[1]
		pathVal := ""
		if pm := xmlAttrPath.FindStringSubmatch(attrs); len(pm) >= 2 {
			pathVal = filepath.ToSlash(strings.TrimSpace(pm[1]))
		}
		fnVal := ""
		if fm := xmlAttrFn.FindStringSubmatch(attrs); len(fm) >= 2 {
			fnVal = strings.TrimSpace(fm[1])
		}
		caseVal := ""
		if cm := xmlAttrCase.FindStringSubmatch(attrs); len(cm) >= 2 {
			caseVal = strings.TrimSpace(cm[1])
		}
		structVal := ""
		if sm := xmlAttrStruct.FindStringSubmatch(attrs); len(sm) >= 2 {
			structVal = strings.TrimSpace(sm[1])
		}
		var methods []string
		if mm := xmlAttrMethods.FindStringSubmatch(attrs); len(mm) >= 2 {
			for _, part := range strings.Split(mm[1], ",") {
				if p := strings.TrimSpace(part); p != "" {
					methods = append(methods, p)
				}
			}
		}
		forPatch := false
		if fpMatch := xmlAttrForPatch.FindStringSubmatch(attrs); len(fpMatch) >= 2 {
			forPatch = fpMatch[1] == "true" || fpMatch[1] == "1"
		}

		req := FileRequest{
			Path:     pathVal,
			Fn:       fnVal,
			Case:     caseVal,
			Struct:   structVal,
			Methods:  methods,
			ForPatch: forPatch,
		}
		key := fmt.Sprintf("SELECT|%s|%s|%s|%s|%v|%t", pathVal, fnVal, caseVal, structVal, methods, forPatch)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	// 4. <xref ... /> or <deps-of ... /> tags
	for _, m := range xmlXrefRegex.FindAllStringSubmatch(input, -1) {
		attrs := m[1]
		sym := ""
		if sm := xmlAttrSymbols.FindStringSubmatch(attrs); len(sm) >= 2 {
			sym = strings.TrimSpace(sm[1])
		} else if nm := xmlAttrName.FindStringSubmatch(attrs); len(nm) >= 2 {
			sym = strings.TrimSpace(nm[1])
		}
		if sym == "" {
			continue
		}
		action := "all"
		if am := xmlAttrAction.FindStringSubmatch(attrs); len(am) >= 2 {
			action = strings.ToLower(strings.TrimSpace(am[1]))
		}
		ctx := 2
		if cm := xmlAttrContext.FindStringSubmatch(attrs); len(cm) >= 2 {
			ctx, _ = strconv.Atoi(cm[1])
		}
		req := FileRequest{
			XRef:       sym,
			XRefAction: action,
			Context:    ctx,
		}
		key := fmt.Sprintf("XREF|%s|%s|%d", sym, action, ctx)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	// 5. <def name="..." trace="true" /> tags
	for _, m := range xmlDefRegex.FindAllStringSubmatch(input, -1) {
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
		depth := 2
		if dm := xmlAttrDepth.FindStringSubmatch(attrs); len(dm) >= 2 {
			depth, _ = strconv.Atoi(dm[1])
		}

		forPatch := false
		if fpMatch := xmlAttrForPatch.FindStringSubmatch(attrs); len(fpMatch) >= 2 {
			forPatch = fpMatch[1] == "true" || fpMatch[1] == "1"
		}

		req := FileRequest{
			Path:     pathVal,
			Def:      strings.TrimSpace(nameMatch[1]),
			Trace:    traceVal,
			Depth:    depth,
			ForPatch: forPatch,
		}
		key := fmt.Sprintf("DEF|%s|%s|%t|%t", req.Def, req.Path, req.Trace, req.ForPatch)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	// 6. Project-wide <grep ... /> or <search ... /> tags
	for _, m := range xmlGrepRegex.FindAllStringSubmatch(input, -1) {
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

		forPatch := false
		if fpMatch := xmlAttrForPatch.FindStringSubmatch(attrs); len(fpMatch) >= 2 {
			forPatch = fpMatch[1] == "true" || fpMatch[1] == "1"
		}

		req := FileRequest{
			Query:    strings.TrimSpace(searchMatch[1]),
			Context:  ctx,
			Glob:     globVal,
			ForPatch: forPatch,
		}

		key := fmt.Sprintf("GREP|%s|%d|%s|%t", req.Query, req.Context, req.Glob, req.ForPatch)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	// 7. File-specific <file ... /> or <slice ... /> tags
	for _, m := range xmlTagRegex.FindAllStringSubmatch(input, -1) {
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
		if dm := xmlAttrDepth.FindStringSubmatch(attrs); len(dm) >= 2 {
			req.Depth, _ = strconv.Atoi(dm[1])
		}
		if cm := xmlAttrCase.FindStringSubmatch(attrs); len(cm) >= 2 {
			req.Case = strings.TrimSpace(cm[1])
		}
		if fm := xmlAttrFn.FindStringSubmatch(attrs); len(fm) >= 2 {
			req.Fn = strings.TrimSpace(fm[1])
		}
		if fpMatch := xmlAttrForPatch.FindStringSubmatch(attrs); len(fpMatch) >= 2 {
			req.ForPatch = fpMatch[1] == "true" || fpMatch[1] == "1"
		}

		if req.Def == "" && req.Trace && len(req.Symbols) == 1 {
			req.Def = req.Symbols[0]
		}

		ctx := 10
		if ctxMatch := xmlAttrContext.FindStringSubmatch(attrs); len(ctxMatch) >= 2 {
			ctx, _ = strconv.Atoi(ctxMatch[1])
		}
		req.Context = ctx

		key := fmt.Sprintf("%s|%v|%s|%s|%s|%s|%s|%s|%t|%s|%s|%t", req.Path, req.Ranges, strings.Join(req.Symbols, ","), req.Query, req.From, req.To, req.Anchor, req.Def, req.Trace, req.Case, req.Fn, req.ForPatch)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	if len(requests) > 0 {
		return requests
	}

	// 8. Fallback to Markdown bullet lists
	for _, m := range mdLinePattern.FindAllStringSubmatch(input, -1) {
		p := filepath.ToSlash(strings.Trim(strings.TrimSpace(m[1]), "`'\""))
		if strings.HasPrefix(p, "http") || strings.HasPrefix(p, "#") || p == "" {
			continue
		}

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

		if p == "def" || p == "definition" || p == "trace" || p == "flow" {
			name := ""
			if len(m) >= 3 && m[2] != "" {
				name = strings.Trim(strings.TrimSpace(m[2]), `"' `)
			}
			if name != "" {
				req := FileRequest{
					Def:   name,
					Trace: p == "trace" || p == "flow",
					Depth: 2,
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
				req.Depth = 2
			case strings.HasPrefix(detail, "case:"):
				req.Case = strings.Trim(strings.TrimSpace(detail[5:]), `"' `)
			default:
				detail = strings.TrimPrefix(detail, "lines")
				req.Ranges = ParseLineRanges(detail)
				if len(req.Ranges) > 0 {
					req.Start = req.Ranges[0].Start
					req.End = req.Ranges[0].End
				}
			}
		}

		key := fmt.Sprintf("%s|%v|%s|%s|%s|%s|%s|%s|%t|%s|%t", req.Path, req.Ranges, strings.Join(req.Symbols, ","), req.Query, req.From, req.To, req.Anchor, req.Def, req.Trace, req.Case, req.ForPatch)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	return requests
}

// LoadRequests reads requested files, extracts line slices/searches/symbols,
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

	var astIndex *parser.CodebaseIndex
	for _, req := range requests {
		if req.Def != "" || req.Trace || req.Case != "" || req.Struct != "" || req.XRef != "" || req.FeatureCtx != nil {
			astIndex, _ = parser.BuildIndex(rootPath, parser.ParseLanguages("all"))
			break
		}
	}

	var expandedRequests []FileRequest
	for _, req := range requests {
		if req.FeatureCtx != nil || req.XRef != "" || req.Def != "" || req.Trace || req.Case != "" || req.Struct != "" {
			expandedRequests = append(expandedRequests, req)
			continue
		}

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
				if fileMatchesQuery(string(data), req.Query) {
					clone := req
					clone.Path = mf
					expandedRequests = append(expandedRequests, clone)
				}
			}
			continue
		}

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
		// 1. Composite Feature Context Query
		if req.FeatureCtx != nil {
			if astIndex == nil {
				astIndex, _ = parser.BuildIndex(rootPath, parser.ParseLanguages(req.FeatureCtx.Lang))
			}
			content := renderFeatureContext(astIndex, rootPath, req.FeatureCtx, opts.RedactSecrets)
			n, writeErr := bufWriter.WriteString(content)
			if writeErr != nil {
				return nil, writeErr
			}
			result.FilesCount++
			result.TotalBytes += int64(n)
			if !opts.Stdout {
				fmt.Printf("Added: [composite feature-context bundle]\n")
			}
			continue
		}

		// 2. Cross-Reference (XRef) Query
		if req.XRef != "" {
			if astIndex == nil {
				astIndex, _ = parser.BuildIndex(rootPath, parser.ParseLanguages("all"))
			}
			content := renderXRefs(astIndex, req.XRef, req.XRefAction, rootPath, opts.RedactSecrets)
			n, writeErr := bufWriter.WriteString(content)
			if writeErr != nil {
				return nil, writeErr
			}
			result.FilesCount++
			result.TotalBytes += int64(n)
			if !opts.Stdout {
				fmt.Printf("Added: xrefs for %q (action: %s)\n", req.XRef, req.XRefAction)
			}
			continue
		}

		// 3. AST Sub-Branch Case Selector
		if req.Case != "" {
			content, sLine, eLine, err := handleCaseRequest(rootPath, req)
			if err != nil {
				if !opts.Stdout {
					fmt.Printf("⚠️ Warning: Case extraction failed: %v\n", err)
				}
				continue
			}

			if opts.RedactSecrets {
				var count int
				content, count = fileutil.RedactSecrets(content)
				result.RedactedSecrets += count
			}

			formatted := ""
			if req.ForPatch {
				formatted = fmt.Sprintf("<!-- textify: AST case replacement block (ready for <replace>) -->\n<replace path=\"%s\" fn=\"%s\" case=\"%s\">\n<with>\n%s\n</with>\n</replace>\n\n", req.Path, req.Fn, req.Case, content)
			} else {
				formatted = renderFormattedEntry(req.Path, content, format, sLine, eLine)
			}

			n, writeErr := bufWriter.WriteString(formatted)
			if writeErr != nil {
				return nil, writeErr
			}
			result.FilesCount++
			result.TotalBytes += int64(n)
			if !opts.Stdout {
				fmt.Printf("Added: %s (case: %s)\n", req.Path, req.Case)
			}
			continue
		}

		// 4. Struct Methods Selection
		if req.Struct != "" && len(req.Methods) > 0 {
			if astIndex == nil {
				astIndex, _ = parser.BuildIndex(rootPath, parser.ParseLanguages("all"))
			}
			content := renderStructMethods(astIndex, req, opts.RedactSecrets)
			n, writeErr := bufWriter.WriteString(content)
			if writeErr != nil {
				return nil, writeErr
			}
			result.FilesCount++
			result.TotalBytes += int64(n)
			if !opts.Stdout {
				fmt.Printf("Added: %s (methods: %s)\n", req.Struct, strings.Join(req.Methods, ", "))
			}
			continue
		}

		// 5. Deep Contextual Trace or Definition
		if req.Def != "" {
			if astIndex == nil {
				astIndex, _ = parser.BuildIndex(rootPath, parser.ParseLanguages("all"))
			}
			if astIndex == nil {
				continue
			}

			rendered := buildASTContent(astIndex, req, opts.RedactSecrets)
			var formatted string
			if req.ForPatch && !req.Trace {
				formatted = fmt.Sprintf("<!-- textify: patch anchor block (ready for <replace>) -->\n<replace path=\"%s\" symbol=\"%s\">\n<with>\n%s\n</with>\n</replace>\n\n", rendered.label, req.Def, strings.TrimRight(rendered.content, "\n"))
			} else {
				formatted = renderFormattedEntry(rendered.label, rendered.content, format, rendered.startLine, rendered.endLine)
			}

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
					desc = fmt.Sprintf("deep contextual trace: %s", req.Def)
				}
				fmt.Printf("Added: %s (%s)\n", rendered.label, desc)
			}
			continue
		}

		// 6. Regular File / Slices / Searches
		absPath := filepath.Join(rootPath, filepath.FromSlash(req.Path))
		if isBin, err := fileutil.IsBinary(absPath); err != nil || isBin {
			continue
		}

		rawBytes, err := os.ReadFile(absPath)
		if err != nil {
			if !opts.Stdout {
				fmt.Printf("⚠️ Warning: Could not read requested file '%s': %v\n", req.Path, err)
			}
			continue
		}

		content, startLine, endLine := extractSectionsCustom(string(rawBytes), req, !req.ForPatch)
		redactedCount := 0
		if opts.RedactSecrets {
			content, redactedCount = fileutil.RedactSecrets(content)
		}

		var formatted string
		if req.ForPatch {
			formatted = fmt.Sprintf("<!-- textify: patch anchor block (ready to copy into <replace> or <search>) -->\n<replace path=\"%s\">\n<search>\n%s\n</search>\n<with>\n%s\n</with>\n</replace>\n\n", req.Path, strings.TrimRight(content, "\n"), strings.TrimRight(content, "\n"))
		} else {
			formatted = renderFormattedEntry(req.Path, content, format, startLine, endLine)
		}

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
			if req.ForPatch {
				desc += " [patch-ready]"
			}
			fmt.Printf("Added: %s (%s)\n", req.Path, desc)
		}
	}

	result.ApproxTokens = int(result.TotalBytes / 4)
	return result, nil
}

func handleCaseRequest(rootPath string, req FileRequest) (string, int, int, error) {
	filePath := req.Path
	if filePath == "" {
		return "", 0, 0, fmt.Errorf("case extraction requires path attribute")
	}
	absPath := filepath.Join(rootPath, filepath.FromSlash(filePath))
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", 0, 0, err
	}
	body, sLine, eLine, err := parser.ExtractCase(filePath, string(data), req.Fn, req.Case)
	if err != nil {
		return "", 0, 0, err
	}

	if req.ForPatch {
		return body, sLine, eLine, nil
	}

	lines := strings.Split(body, "\n")
	var numbered strings.Builder
	for i, l := range lines {
		numbered.WriteString(fmt.Sprintf("%5d | %s\n", sLine+i, l))
	}
	return numbered.String(), sLine, eLine, nil
}

func renderStructMethods(idx *parser.CodebaseIndex, req FileRequest, redact bool) string {
	methods := idx.FindStructMethods(req.Struct, req.Methods)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Methods for Struct/Class %q (%d matched) ===\n\n", req.Struct, len(methods)))
	for _, m := range methods {
		sb.WriteString(fmt.Sprintf("--- %s %s [%s:%d-%d] ---\n", m.Kind, m.FullName(), m.File, m.StartLine, m.EndLine))
		if req.ForPatch {
			sb.WriteString(m.Body)
			sb.WriteString("\n\n")
		} else {
			for offset, line := range strings.Split(m.Body, "\n") {
				sb.WriteString(fmt.Sprintf("%5d | %s\n", m.StartLine+offset, line))
			}
			sb.WriteString("\n\n")
		}
	}
	content := sb.String()
	if redact {
		content, _ = fileutil.RedactSecrets(content)
	}
	return content
}

func renderXRefs(idx *parser.CodebaseIndex, symbol, action, rootPath string, redact bool) string {
	xrefs := idx.FindXRefs(symbol, action, rootPath)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Cross-References for Symbol %q (action: %s, %d matches) ===\n\n", symbol, action, len(xrefs)))
	for _, x := range xrefs {
		sb.WriteString(fmt.Sprintf("File: %s:%d [%s]\n%s\n\n", x.File, x.Line, x.Action, x.Context))
	}
	content := sb.String()
	if redact {
		content, _ = fileutil.RedactSecrets(content)
	}
	return content
}

func renderFeatureContext(idx *parser.CodebaseIndex, rootPath string, fc *FeatureContextReq, redact bool) string {
	var sb strings.Builder
	sb.WriteString("================================================================================\n")
	sb.WriteString(fmt.Sprintf("FEATURE CONTEXT BUNDLE [Language: %s]\n", fc.Lang))
	sb.WriteString("================================================================================\n\n")

	if len(fc.TargetTypes) > 0 {
		sb.WriteString("### 1. Target Type Definitions & Schemas\n\n")
		for _, name := range fc.TargetTypes {
			syms := idx.FindDefinition(name, "")
			for _, s := range syms {
				sb.WriteString(fmt.Sprintf("--- %s %s [%s:%d-%d] ---\n", s.Kind, s.FullName(), s.File, s.StartLine, s.EndLine))
				sb.WriteString(s.Body)
				sb.WriteString("\n\n")
			}
		}
	}

	if len(fc.Entrypoints) > 0 {
		sb.WriteString("### 2. Entrypoint Deep Execution Traces\n\n")
		for _, ep := range fc.Entrypoints {
			deep := idx.TraceDeep(ep.Symbol, ep.Depth)
			if deep != nil {
				sb.WriteString(deep.Render(fc.ForPatch))
			}
		}
	}

	if len(fc.Touchpoints) > 0 {
		sb.WriteString("### 3. Touchpoint Cross-References\n\n")
		for _, tp := range fc.Touchpoints {
			xrefs := idx.FindXRefs(tp, "all", rootPath)
			sb.WriteString(fmt.Sprintf("=== Touchpoint %q (%d occurrences) ===\n", tp, len(xrefs)))
			for _, x := range xrefs {
				sb.WriteString(fmt.Sprintf("%s:%d | %s\n", x.File, x.Line, x.LineText))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("================================================================================\n\n")
	content := sb.String()
	if redact {
		content, _ = fileutil.RedactSecrets(content)
	}
	return content
}

type astRenderResult struct {
	content   string
	label     string
	startLine int
	endLine   int
	redacted  int
}

func buildASTContent(index *parser.CodebaseIndex, req FileRequest, redact bool) astRenderResult {
	var sb strings.Builder

	if req.Trace {
		depth := req.Depth
		if depth <= 0 {
			depth = 2
		}
		deep := index.TraceDeep(req.Def, depth)
		if deep != nil {
			sb.WriteString(deep.Render(req.ForPatch))
		} else {
			sb.WriteString(index.TraceFlow(req.Def, depth))
			sb.WriteString("\n")
		}
	}

	syms := index.FindDefinition(req.Def, req.Path)
	firstFile := ""
	startLine := 0
	endLine := 0

	if len(syms) == 0 && !req.Trace {
		sb.WriteString(fmt.Sprintf("/* Symbol %q not found in AST index */\n", req.Def))
	} else if !req.Trace {
		firstFile = syms[0].File
		for _, sym := range syms {
			sb.WriteString(fmt.Sprintf("/* Definition: %s %s [%s:%d-%d] */\n", sym.Kind, sym.FullName(), sym.File, sym.StartLine, sym.EndLine))
			if req.ForPatch {
				sb.WriteString(sym.Body)
				sb.WriteString("\n\n")
			} else {
				for offset, line := range strings.Split(sym.Body, "\n") {
					sb.WriteString(fmt.Sprintf("%5d | %s\n", sym.StartLine+offset, line))
				}
				sb.WriteString("\n")
			}

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
	return extractSectionsCustom(content, req, true)
}

func extractSectionsCustom(content string, req FileRequest, numbered bool) (string, int, int) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	if totalLines == 0 {
		return "", 0, 0
	}

	var intervals []LineRange

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

	if len(req.Symbols) > 0 {
		symIntervals := findSymbolIntervals(lines, req.Symbols)
		intervals = append(intervals, symIntervals...)
	}

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

	if len(intervals) == 0 {
		if req.Query != "" || req.From != "" || req.Anchor != "" || len(req.Symbols) > 0 {
			return fmt.Sprintf("/* No matches found in %s */\n", req.Path), 1, totalLines
		}
		var sb strings.Builder
		for i, line := range lines {
			if numbered {
				sb.WriteString(fmt.Sprintf("%5d | %s\n", i+1, line))
			} else {
				sb.WriteString(line + "\n")
			}
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
				if numbered {
					sb.WriteString(fmt.Sprintf("      ... [skipped lines %d-%d] ...\n", prevEnd+1, r.Start-1))
				} else {
					sb.WriteString("...\n")
				}
			}
		}
		for i := r.Start - 1; i < r.End && i < totalLines; i++ {
			if numbered {
				sb.WriteString(fmt.Sprintf("%5d | %s\n", i+1, lines[i]))
			} else {
				sb.WriteString(lines[i] + "\n")
			}
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
		"func ", "fn ", "pub fn ", "type ", "struct ", "interface ", "class ", "def ", "trait ", "enum ", "export function ", "export class ",
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
