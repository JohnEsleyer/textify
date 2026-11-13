package rust

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

func init() {
	targets.RegisterTarget(&RustTarget{})
}

// RustTarget implements processing for Rust source files (.rs).
type RustTarget struct{}

func (r *RustTarget) Name() string {
	return "rust"
}

func (r *RustTarget) Match(filePath string) bool {
	return strings.ToLower(filepath.Ext(filePath)) == ".rs"
}

func (r *RustTarget) ExtractSignatures(filePath string, content string) string {
	return ExtractSignatures(content)
}

func (r *RustTarget) Parse(filePath string, src []byte) ([]targets.Symbol, error) {
	return ParseRustFile(filePath, src)
}

type signatureChunk struct {
	startLine int
	text      string
}

// ExtractSignatures produces clean Rust signatures with original 1-based line numbers.
// Preserves structs, enums, traits, type aliases, attributes, and modules; collapses
// function and impl method bodies into `{ /* ... */ }`.
func ExtractSignatures(content string) string {
	lines := strings.Split(content, "\n")
	var chunks []signatureChunk

	inBlockComment := false
	inImpl := false
	implDepth := 0
	inItem := false
	itemStartLine := 0
	itemDepth := 0
	var itemHeader strings.Builder
	pendingAttrs := make([]string, 0)
	pendingAttrStart := 0

	for i := 0; i < len(lines); i++ {
		lineNum := i + 1
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Handle block comments across lines
		if inBlockComment {
			if idx := strings.Index(trimmed, "*/"); idx != -1 {
				inBlockComment = false
				trimmed = strings.TrimSpace(trimmed[idx+2:])
			} else {
				continue
			}
		} else if strings.HasPrefix(trimmed, "/*") && !strings.Contains(trimmed, "*/") {
			inBlockComment = true
			continue
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Buffer outer attributes (#[...]) so they stay attached to the item
		if strings.HasPrefix(trimmed, "#[") || strings.HasPrefix(trimmed, "#![") {
			if len(pendingAttrs) == 0 {
				pendingAttrStart = lineNum
			}
			pendingAttrs = append(pendingAttrs, line)
			continue
		}

		// Top-level imports, modules, macros, and constants
		if !inItem && !inImpl {
			if strings.HasPrefix(trimmed, "use ") || strings.HasPrefix(trimmed, "pub use ") ||
				strings.HasPrefix(trimmed, "mod ") || strings.HasPrefix(trimmed, "pub mod ") ||
				strings.HasPrefix(trimmed, "extern crate ") ||
				strings.HasPrefix(trimmed, "macro_rules!") {
				text := line
				if len(pendingAttrs) > 0 {
					text = strings.Join(pendingAttrs, "\n") + "\n" + line
					lineNum = pendingAttrStart
					pendingAttrs = pendingAttrs[:0]
				}
				chunks = append(chunks, signatureChunk{startLine: lineNum, text: text})
				continue
			}
		}

		// If currently skipping a function body, consume lines until the function's closing brace
		if inItem && itemDepth > 0 && !isRustTypeDef(itemHeader.String()) {
			itemDepth += countBraceDelta(line)
			if itemDepth <= 0 {
				inItem = false
				itemDepth = 0
			}
			continue
		}

		// Detect impl block start
		if !inItem && (strings.HasPrefix(trimmed, "impl ") || strings.HasPrefix(trimmed, "impl<") ||
			strings.HasPrefix(trimmed, "pub impl ") || strings.HasPrefix(trimmed, "unsafe impl ")) {
			inImpl = true
			implDepth = 0
			text := line
			if len(pendingAttrs) > 0 {
				text = strings.Join(pendingAttrs, "\n") + "\n" + line
				lineNum = pendingAttrStart
				pendingAttrs = pendingAttrs[:0]
			}
			chunks = append(chunks, signatureChunk{startLine: lineNum, text: text})
			implDepth += countBraceDelta(line)
			if implDepth <= 0 && strings.Contains(line, "{") {
				inImpl = false
			}
			continue
		}

		// Track impl block closing
		if inImpl && !inItem {
			openDelta := countBraceDelta(line)
			implDepth += openDelta
			if implDepth <= 0 && (strings.Contains(line, "}") || openDelta < 0) {
				inImpl = false
				chunks = append(chunks, signatureChunk{startLine: lineNum, text: "}"})
				continue
			}
		}

		// Detect item start (functions, structs, enums, traits, type aliases)
		isItemStart := isRustDeclStart(trimmed)
		if isItemStart && !inItem {
			inItem = true
			itemStartLine = lineNum
			itemDepth = 0
			itemHeader.Reset()

			if len(pendingAttrs) > 0 {
				itemHeader.WriteString(strings.Join(pendingAttrs, "\n"))
				itemHeader.WriteString("\n")
				itemStartLine = pendingAttrStart
				pendingAttrs = pendingAttrs[:0]
			}
		}

		if inItem {
			openIdx := findBraceIndexOutsideStrings(line, '{')
			semiIdx := strings.Index(line, ";")

			// Check if this item is a struct, enum, or trait (keep internal fields/variants)
			isTypeDefinition := isRustTypeDef(itemHeader.String() + "\n" + line)

			if isTypeDefinition {
				itemHeader.WriteString(line)
				itemHeader.WriteString("\n")
				itemDepth += countBraceDelta(line)
				if (itemDepth <= 0 && strings.Contains(line, "}")) || (openIdx == -1 && semiIdx != -1 && itemDepth == 0) {
					chunks = append(chunks, signatureChunk{
						startLine: itemStartLine,
						text:      strings.TrimRight(itemHeader.String(), "\n"),
					})
					inItem = false
					itemDepth = 0
				}
				continue
			}

			// For functions and methods: collapse body into `{ /* ... */ }`
			if openIdx != -1 {
				headerPart := line[:openIdx]
				itemHeader.WriteString(headerPart)
				sigText := strings.TrimRight(itemHeader.String(), " \t") + " { /* ... */ }"
				chunks = append(chunks, signatureChunk{
					startLine: itemStartLine,
					text:      sigText,
				})
				// Track remaining braces from the opening brace onwards
				remainingLine := line[openIdx:]
				itemDepth = countBraceDelta(remainingLine)
				if itemDepth <= 0 {
					inItem = false
					itemDepth = 0
				}
			} else if semiIdx != -1 && itemDepth == 0 {
				// Trait method signature without body (e.g. `fn poll(&mut self) -> Poll<()>;`)
				itemHeader.WriteString(line)
				chunks = append(chunks, signatureChunk{
					startLine: itemStartLine,
					text:      strings.TrimRight(itemHeader.String(), "\n"),
				})
				inItem = false
				itemDepth = 0
			} else {
				itemHeader.WriteString(line)
				itemHeader.WriteString("\n")
			}
			continue
		}

		pendingAttrs = pendingAttrs[:0]
	}

	sort.SliceStable(chunks, func(i, j int) bool {
		return chunks[i].startLine < chunks[j].startLine
	})

	var out strings.Builder
	for _, chunk := range chunks {
		chunkLines := strings.Split(chunk.text, "\n")
		for idx, cl := range chunkLines {
			out.WriteString(fmt.Sprintf("%5d | %s\n", chunk.startLine+idx, cl))
		}
	}
	return out.String()
}

func findBraceIndexOutsideStrings(line string, target rune) int {
	inStr := false
	var strChar rune
	chars := []rune(line)
	for i := 0; i < len(chars); i++ {
		ch := chars[i]
		if inStr {
			if ch == '\\' && i+1 < len(chars) {
				i++
				continue
			}
			if ch == strChar {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			strChar = ch
			continue
		}
		if ch == '/' && i+1 < len(chars) && chars[i+1] == '/' {
			break
		}
		if ch == target {
			return i
		}
	}
	return -1
}

func isRustDeclStart(trimmed string) bool {
	prefixes := []string{
		"pub fn ", "fn ", "pub async fn ", "async fn ",
		"pub const fn ", "const fn ", "pub unsafe fn ", "unsafe fn ",
		"pub extern ", "extern ",
		"pub struct ", "struct ",
		"pub enum ", "enum ",
		"pub union ", "union ",
		"pub trait ", "trait ", "pub unsafe trait ", "unsafe trait ",
		"pub type ", "type ",
		"pub const ", "const ",
		"pub static ", "static ",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(trimmed, p) {
			return true
		}
	}
	if strings.HasPrefix(trimmed, "pub(") {
		idx := strings.Index(trimmed, ") ")
		if idx != -1 {
			rest := strings.TrimSpace(trimmed[idx+2:])
			return isRustDeclStart(rest)
		}
	}
	return false
}

func isRustTypeDef(text string) bool {
	return strings.Contains(text, "struct ") ||
		strings.Contains(text, "enum ") ||
		strings.Contains(text, "trait ") ||
		strings.Contains(text, "union ") ||
		strings.Contains(text, "type ")
}

func countBraceDelta(line string) int {
	delta := 0
	inStr := false
	var strChar rune
	chars := []rune(line)
	for i := 0; i < len(chars); i++ {
		ch := chars[i]
		if inStr {
			if ch == '\\' && i+1 < len(chars) {
				i++
				continue
			}
			if ch == strChar {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			strChar = ch
			continue
		}
		if ch == '/' && i+1 < len(chars) && chars[i+1] == '/' {
			break
		}
		if ch == '{' {
			delta++
		} else if ch == '}' {
			delta--
		}
	}
	return delta
}

var (
	fnRegex     = regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^\)]+\))?\s+)?(?:async\s+)?(?:const\s+)?(?:unsafe\s+)?(?:extern(?:\s+"[^"]+")?\s+)?fn\s+([a-zA-Z0-9_]+)`)
	structRegex = regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^\)]+\))?\s+)?struct\s+([a-zA-Z0-9_]+)`)
	enumRegex   = regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^\)]+\))?\s+)?enum\s+([a-zA-Z0-9_]+)`)
	traitRegex  = regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^\)]+\))?\s+)?(?:unsafe\s+)?trait\s+([a-zA-Z0-9_]+)`)
	typeRegex   = regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^\)]+\))?\s+)?type\s+([a-zA-Z0-9_]+)`)
	implRegex   = regexp.MustCompile(`(?m)^\s*(?:unsafe\s+)?impl(?:<[^>]+>)?\s+(?:([a-zA-Z0-9_:]+)\s+for\s+)?([a-zA-Z0-9_:]+)`)
	callRegex   = regexp.MustCompile(`\b([a-zA-Z0-9_]+)(?:::([a-zA-Z0-9_]+))?\s*\(`)
)

// ParseRustFile parses a Rust source file into high-level AST symbols.
func ParseRustFile(filePath string, src []byte) ([]targets.Symbol, error) {
	lines := strings.Split(string(src), "\n")
	var symbols []targets.Symbol

	type implContext struct {
		targetType string
		traitName  string
		startLine  int
		endLine    int
	}
	var impls []implContext

	// Step 1: Scan for impl blocks to map methods to receivers
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "impl") || strings.HasPrefix(trimmed, "unsafe impl") {
			if m := implRegex.FindStringSubmatch(trimmed); len(m) >= 3 {
				traitName := cleanRustName(m[1])
				targetType := cleanRustName(m[2])
				startLine := i + 1
				endLine := findBlockEnd(lines, i)
				impls = append(impls, implContext{
					targetType: targetType,
					traitName:  traitName,
					startLine:  startLine,
					endLine:    endLine,
				})
			}
		}
	}

	// Step 2: Scan for item declarations
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		lineNum := i + 1

		// Struct
		if structRegex.MatchString(trimmed) {
			m := structRegex.FindStringSubmatch(trimmed)
			name := m[1]
			end := findItemEnd(lines, i)
			body := targets.SliceLines(lines, lineNum, end)
			refs := extractRustRefs(body, name)
			symbols = append(symbols, targets.Symbol{
				Name:       name,
				Kind:       targets.KindStruct,
				File:       filePath,
				StartLine:  lineNum,
				EndLine:    end,
				Signature:  extractFirstLineSig(body),
				Body:       body,
				References: refs,
			})
			continue
		}

		// Enum
		if enumRegex.MatchString(trimmed) {
			m := enumRegex.FindStringSubmatch(trimmed)
			name := m[1]
			end := findBlockEnd(lines, i)
			body := targets.SliceLines(lines, lineNum, end)
			refs := extractRustRefs(body, name)
			symbols = append(symbols, targets.Symbol{
				Name:       name,
				Kind:       targets.KindType,
				File:       filePath,
				StartLine:  lineNum,
				EndLine:    end,
				Signature:  extractFirstLineSig(body),
				Body:       body,
				References: refs,
			})
			continue
		}

		// Trait
		if traitRegex.MatchString(trimmed) {
			m := traitRegex.FindStringSubmatch(trimmed)
			name := m[1]
			end := findBlockEnd(lines, i)
			body := targets.SliceLines(lines, lineNum, end)
			symbols = append(symbols, targets.Symbol{
				Name:      name,
				Kind:      targets.KindInterface,
				File:      filePath,
				StartLine: lineNum,
				EndLine:   end,
				Signature: extractFirstLineSig(body),
				Body:      body,
			})
			continue
		}

		// Type alias
		if typeRegex.MatchString(trimmed) && strings.Contains(trimmed, "=") {
			m := typeRegex.FindStringSubmatch(trimmed)
			name := m[1]
			end := lineNum
			for j := i; j < len(lines); j++ {
				if strings.Contains(lines[j], ";") {
					end = j + 1
					break
				}
			}
			body := targets.SliceLines(lines, lineNum, end)
			refs := extractRustRefs(body, name)
			symbols = append(symbols, targets.Symbol{
				Name:       name,
				Kind:       targets.KindType,
				File:       filePath,
				StartLine:  lineNum,
				EndLine:    end,
				Signature:  strings.TrimSpace(body),
				Body:       body,
				References: refs,
			})
			continue
		}

		// Function or Method
		if fnRegex.MatchString(trimmed) {
			m := fnRegex.FindStringSubmatch(trimmed)
			name := m[1]
			end := findBlockEnd(lines, i)
			body := targets.SliceLines(lines, lineNum, end)

			receiver := ""
			kind := targets.KindFunction
			for _, imp := range impls {
				if lineNum >= imp.startLine && lineNum <= imp.endLine {
					receiver = imp.targetType
					kind = targets.KindMethod
					break
				}
			}

			sig := extractFnSignature(body)
			calls := extractRustCalls(body, name)
			refs := extractRustRefs(sig, name)

			symbols = append(symbols, targets.Symbol{
				Name:       name,
				Receiver:   receiver,
				Kind:       kind,
				File:       filePath,
				StartLine:  lineNum,
				EndLine:    end,
				Signature:  sig,
				Body:       body,
				Calls:      calls,
				References: refs,
			})
		}
	}

	return symbols, nil
}

func findBlockEnd(lines []string, startIdx int) int {
	braceCount := 0
	foundBrace := false
	for j := startIdx; j < len(lines); j++ {
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
			return j + 1
		}
		if !foundBrace && strings.Contains(l, ";") {
			return j + 1
		}
	}
	return len(lines)
}

func findItemEnd(lines []string, startIdx int) int {
	// For structs, could be `struct Foo { ... }` or tuple struct `struct Foo(...);` or unit struct `struct Foo;`
	for j := startIdx; j < len(lines); j++ {
		l := lines[j]
		if strings.Contains(l, "{") {
			return findBlockEnd(lines, startIdx)
		}
		if strings.Contains(l, ";") {
			return j + 1
		}
	}
	return startIdx + 1
}

func extractFirstLineSig(body string) string {
	idx := strings.Index(body, "{")
	if idx != -1 {
		return strings.TrimSpace(body[:idx])
	}
	semi := strings.Index(body, ";")
	if semi != -1 {
		return strings.TrimSpace(body[:semi+1])
	}
	if lineEnd := strings.Index(body, "\n"); lineEnd != -1 {
		return strings.TrimSpace(body[:lineEnd])
	}
	return strings.TrimSpace(body)
}

func extractFnSignature(body string) string {
	idx := strings.Index(body, "{")
	if idx != -1 {
		raw := strings.TrimSpace(body[:idx])
		return strings.Join(strings.Fields(raw), " ")
	}
	return extractFirstLineSig(body)
}

func extractRustCalls(body, fnName string) []string {
	var calls []string
	seen := make(map[string]bool)
	seen[fnName] = true
	rustKeywords := map[string]bool{
		"if": true, "while": true, "for": true, "match": true, "return": true,
		"let": true, "mut": true, "pub": true, "fn": true, "impl": true,
		"Some": true, "None": true, "Ok": true, "Err": true,
	}

	matches := callRegex.FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		target := m[1]
		if m[2] != "" {
			target = m[1] + "::" + m[2]
		}
		if rustKeywords[target] || seen[target] {
			continue
		}
		seen[target] = true
		calls = append(calls, target)
	}
	return calls
}

func extractRustRefs(text, selfName string) []string {
	var refs []string
	seen := make(map[string]bool)
	seen[selfName] = true
	rustBuiltins := map[string]bool{
		"bool": true, "char": true, "str": true, "u8": true, "u16": true, "u32": true, "u64": true, "u128": true, "usize": true,
		"i8": true, "i16": true, "i32": true, "i64": true, "i128": true, "isize": true, "f32": true, "f64": true,
		"String": true, "Vec": true, "Option": true, "Result": true, "Box": true, "Rc": true, "Arc": true,
		"self": true, "Self": true, "true": true, "false": true,
	}

	identRe := regexp.MustCompile(`\b([A-Z][a-zA-Z0-9_]*)\b`)
	for _, m := range identRe.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if seen[name] || rustBuiltins[name] {
			continue
		}
		seen[name] = true
		refs = append(refs, name)
	}
	return refs
}

func cleanRustName(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.LastIndex(s, "::"); idx != -1 {
		s = s[idx+2:]
	}
	return strings.TrimSpace(s)
}

// CleanRustCaseTarget strips boilerplate from switch-case / match-arm queries.
func CleanRustCaseTarget(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "case ")
	s = strings.TrimSuffix(s, "=>")
	s = strings.TrimSuffix(s, ":")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`")
	return strings.TrimSpace(s)
}

// ExtractRustCase extracts the body, start line, and end line of a match arm
// in a Rust source file.
func ExtractRustCase(filePath, content, fnName, caseTarget string) (string, int, int, error) {
	cleanTarget := CleanRustCaseTarget(caseTarget)
	lines := strings.Split(content, "\n")

	startSearch := 0
	endSearch := len(lines)

	if fnName != "" {
		syms, err := ParseRustFile(filePath, []byte(content))
		if err == nil {
			for _, s := range syms {
				if s.Name == fnName || s.FullName() == fnName {
					startSearch = s.StartLine - 1
					endSearch = s.EndLine
					break
				}
			}
		}
	}

	matchArmRegex := regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9_:]+(?:\s*\([^\)]*\))?|_)\s*=>`)

	for i := startSearch; i < endSearch && i < len(lines); i++ {
		line := lines[i]
		if !strings.Contains(line, "=>") {
			continue
		}

		m := matchArmRegex.FindStringSubmatch(line)
		pattern := ""
		if len(m) >= 2 {
			pattern = CleanRustCaseTarget(m[1])
		}

		matched := false
		if pattern == cleanTarget || strings.HasSuffix(pattern, "::"+cleanTarget) || strings.EqualFold(pattern, cleanTarget) {
			matched = true
		} else if strings.Contains(strings.TrimSpace(line), cleanTarget) && strings.Contains(line, "=>") {
			matched = true
		}

		if matched {
			armStart := i + 1
			armEnd := armStart

			// Determine if arm has a block { ... } or is a single expression ending with comma
			if strings.Contains(line, "{") {
				braceCount := 0
				for j := i; j < endSearch && j < len(lines); j++ {
					for _, ch := range lines[j] {
						if ch == '{' {
							braceCount++
						} else if ch == '}' {
							braceCount--
						}
					}
					if braceCount <= 0 {
						armEnd = j + 1
						break
					}
				}
			} else {
				// Expression ending with comma or closing brace
				for j := i; j < endSearch && j < len(lines); j++ {
					trimmed := strings.TrimSpace(lines[j])
					if strings.HasSuffix(trimmed, ",") || strings.HasSuffix(trimmed, "}") {
						armEnd = j + 1
						break
					}
				}
			}

			body := targets.SliceLines(lines, armStart, armEnd)
			return body, armStart, armEnd, nil
		}
	}

	return "", 0, 0, targets.ErrCaseNotFound(caseTarget)
}
