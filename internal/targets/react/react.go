package react

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/JohnEsleyer/textify/internal/targets"
)

func init() {
	targets.RegisterTarget(&ReactTarget{})
}

// ReactTarget implements specialized processing for React, TSX, TypeScript,
// JavaScript, JSX, and module variants.
type ReactTarget struct{}

func (r *ReactTarget) Name() string {
	return "react"
}

func (r *ReactTarget) Match(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".tsx", ".jsx", ".ts", ".js", ".mjs", ".cjs":
		return true
	}
	return false
}

func (r *ReactTarget) ExtractSignatures(filePath string, content string) string {
	return ExtractSignatures(filePath, content)
}

func (r *ReactTarget) Parse(filePath string, src []byte) ([]targets.Symbol, error) {
	return ParseTypeScriptFile(filePath, src)
}

var (
	// Function declarations, including async functions, generators and generics.
	tsFuncRegex = regexp.MustCompile(`^(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s*(\*?\s*[A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*(\([^{;]*\))`)

	// Arrow-function constants, including React.FC annotations, memo, and forwardRef.
	tsArrowRegex = regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*([^=]+))?\s*=\s*(?:async\s*)?(?:<[^>]*>\s*)?(?:(?:React\.)?(?:memo|forwardRef)\s*(?:<[^>]*>\s*)?\(\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=>]+)?\s*=>\s*\{?`)

	// Class, interface, and type-alias declarations.
	tsClassRegex     = regexp.MustCompile(`^(?:export\s+(?:default\s+)?)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s*<[^>]*>)?(?:\s+extends\s+[^{]+)?(?:\s+implements\s+[^{]+)?`)
	tsInterfaceRegex = regexp.MustCompile(`^(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s*<[^>]*>)?(?:\s+extends\s+[^{]+)?`)
	tsTypeRegex      = regexp.MustCompile(`^(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s*<[^>]*>)?\s*=`)

	// Methods and arrow-function class properties.
	tsMethodRegex     = regexp.MustCompile(`^(?:(?:public|private|protected|static|readonly|abstract|override|declare|async|get|set)\s+)*(?:\*\s*)?([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*\([^)]*\)\s*(?::[^;]*)?(?:\{|;|$)`)
	tsMethodPropRegex = regexp.MustCompile(`^(?:(?:public|private|protected|static|readonly|abstract|override|declare|async)\s+)*([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:<[^>]*>\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`)

	tsCallRegex = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*(?:\.[A-Za-z_$][A-Za-z0-9_$]*)?)\s*\(`)

	tsExtendsRegex    = regexp.MustCompile(`\bextends\s+([^{]+)`)
	tsImplementsRegex = regexp.MustCompile(`\bimplements\s+([^{]+)`)

	tsReactClassRegex = regexp.MustCompile(`\bextends\s+(?:React\.)?(?:Pure)?Component\b`)
)

var tsIgnoredCallees = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"function": true, "return": true, "import": true, "require": true,
	"with": true, "typeof": true, "delete": true, "void": true, "await": true,
	"yield": true, "super": true, "throw": true, "new": true,
}

// ExtractSignatures renders a compact React/TypeScript signature view. Imports,
// interfaces, type aliases, enums, context declarations, and props are kept
// intact, while component, hook, function, and class method bodies are
// collapsed. Original 1-based line numbers are preserved.
func ExtractSignatures(filePath, content string) string {
	lines := strings.Split(content, "\n")
	var out strings.Builder

	inImport := false

	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]
		lineNum := idx + 1
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// 1. Preserve imports (single-line and multi-line).
		if inImport {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			if strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "'") || strings.HasSuffix(trimmed, "\"") {
				inImport = false
			}
			continue
		}
		if isImportStart(trimmed) {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			if !strings.HasSuffix(trimmed, ";") && strings.HasPrefix(trimmed, "import ") {
				if !(strings.HasSuffix(trimmed, "'") || strings.HasSuffix(trimmed, "\"")) {
					inImport = true
				}
			}
			continue
		}

		// 2. Preserve full interfaces, type aliases, and enums (critical for props).
		if isTypeDeclarationStart(trimmed) {
			endLine := typeDeclarationEnd(lines, idx)
			for j := idx; j < endLine && j < len(lines); j++ {
				out.WriteString(fmt.Sprintf("%5d | %s\n", j+1, lines[j]))
			}
			idx = endLine - 1
			continue
		}

		// 3. Preserve React context creation.
		if strings.Contains(trimmed, "createContext") {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			continue
		}

		// 4. Classes / React class components.
		if match := tsClassRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			endLine := findMatchingBrace(lines, idx)
			header := collectTSHeader(lines, idx, false)
			if header == "" {
				header = trimmed
			}
			comment := ""
			if isReactClassSignature(header) {
				comment = " /* component class */"
			}
			out.WriteString(fmt.Sprintf("%5d | %s {%s\n", lineNum, header, comment))
			emitClassMembers(lines, idx, endLine, &out)
			if endLine > idx {
				out.WriteString(fmt.Sprintf("%5d | }\n", endLine))
			}
			idx = endLine - 1
			continue
		}

		// 5. Function declarations / functional components / hooks.
		if match := tsFuncRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := strings.TrimPrefix(strings.TrimSpace(match[1]), "*")
			header := collectTSHeader(lines, idx, false)
			if header == "" {
				header = trimmed
			}
			out.WriteString(fmt.Sprintf("%5d | %s { %s }\n", lineNum, header, kindComment(name)))
			idx = findMatchingBrace(lines, idx) - 1
			continue
		}

		// 6. Arrow-function components, hooks, and helpers.
		if match := tsArrowRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := match[1]
			header := collectTSHeader(lines, idx, true)
			header = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(header), "=>"))
			if header == "" {
				header = trimmed
			}
			out.WriteString(fmt.Sprintf("%5d | %s => { %s }\n", lineNum, header, kindComment(name)))
			idx = findMatchingBrace(lines, idx) - 1
			continue
		}

		// 7. Preserve top-level closing braces for structural context.
		if trimmed == "}" || trimmed == "};" {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
		}
	}

	return out.String()
}

// ParseTypeScriptFile parses TypeScript or JavaScript source into symbols with
// React-specific classification for components and hooks.
func ParseTypeScriptFile(filePath string, src []byte) ([]targets.Symbol, error) {
	lines := strings.Split(string(src), "\n")
	var symbols []targets.Symbol

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineNum := idx + 1

		// 1. Regular functions & functional components.
		if match := tsFuncRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := strings.TrimPrefix(strings.TrimSpace(match[1]), "*")
			endLine := findMatchingBrace(lines, idx)
			body := targets.SliceLines(lines, lineNum, endLine)
			header := collectTSHeader(lines, idx, false)
			if header == "" {
				header = collectTSSignature(lines, idx, 6)
			}

			symbols = append(symbols, targets.Symbol{
				Name:      name,
				Kind:      classifyTSName(name),
				File:      filePath,
				StartLine: lineNum,
				EndLine:   endLine,
				Signature: header,
				Body:      body,
				Calls:     extractTSCalls(body, name),
			})
			continue
		}

		// 2. Arrow-function components, hooks, and helpers.
		if match := tsArrowRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := match[1]
			endLine := findMatchingBrace(lines, idx)
			body := targets.SliceLines(lines, lineNum, endLine)
			header := collectTSHeader(lines, idx, true)
			if header == "" {
				header = collectTSSignature(lines, idx, 6)
			}

			symbols = append(symbols, targets.Symbol{
				Name:      name,
				Kind:      classifyTSName(name),
				File:      filePath,
				StartLine: lineNum,
				EndLine:   endLine,
				Signature: header,
				Body:      body,
				Calls:     extractTSCalls(body, name),
			})
			continue
		}

		// 3. Classes (including React class components).
		if match := tsClassRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := match[1]
			endLine := findMatchingBrace(lines, idx)
			body := targets.SliceLines(lines, lineNum, endLine)
			sig := collectTSHeader(lines, idx, false)
			if sig == "" {
				sig = collectTSSignature(lines, idx, 8)
			}
			kind := targets.KindClass
			if isReactClassSignature(sig) {
				kind = targets.KindComponent
			}

			symbols = append(symbols, targets.Symbol{
				Name:       name,
				Kind:       kind,
				File:       filePath,
				StartLine:  lineNum,
				EndLine:    endLine,
				Signature:  sig,
				Body:       body,
				References: extractTSClassRefs(sig),
			})
			symbols = append(symbols, parseTSClassMethods(lines, idx, endLine, name, filePath)...)
			continue
		}

		// 4. Interfaces.
		if match := tsInterfaceRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := match[1]
			endLine := findMatchingBrace(lines, idx)
			body := targets.SliceLines(lines, lineNum, endLine)
			sig := collectTSSignature(lines, idx, 8)

			symbols = append(symbols, targets.Symbol{
				Name:       name,
				Kind:       targets.KindInterface,
				File:       filePath,
				StartLine:  lineNum,
				EndLine:    endLine,
				Signature:  sig,
				Body:       body,
				References: extractTSClassRefs(sig),
			})
			continue
		}

		// 5. Type aliases.
		if match := tsTypeRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := match[1]
			endLine := findTypeEndLine(lines, idx)
			body := targets.SliceLines(lines, lineNum, endLine)

			symbols = append(symbols, targets.Symbol{
				Name:      name,
				Kind:      targets.KindType,
				File:      filePath,
				StartLine: lineNum,
				EndLine:   endLine,
				Signature: collectTSSignature(lines, idx, 8),
				Body:      body,
			})
			continue
		}
	}

	return symbols, nil
}

func classifyTSName(name string) targets.SymbolKind {
	if isHookName(name) {
		return targets.KindHook
	}
	if isPascalCase(name) {
		return targets.KindComponent
	}
	return targets.KindFunction
}

func kindComment(name string) string {
	switch classifyTSName(name) {
	case targets.KindHook:
		return "/* hook */"
	case targets.KindComponent:
		return "/* component */"
	default:
		return "/* ... */"
	}
}

func isPascalCase(s string) bool {
	if len(s) == 0 {
		return false
	}
	return unicode.IsUpper(rune(s[0]))
}

func isHookName(s string) bool {
	return strings.HasPrefix(s, "use") && len(s) > 3 && unicode.IsUpper(rune(s[3]))
}

func isReactClassSignature(sig string) bool {
	return tsReactClassRegex.MatchString(sig)
}

func stripTSModifiers(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"export default ", "export ", "declare ", "abstract "} {
		line = strings.TrimPrefix(line, prefix)
	}
	return strings.TrimSpace(line)
}

func isImportStart(line string) bool {
	if strings.HasPrefix(line, "import ") {
		return true
	}
	if strings.HasPrefix(line, "export * from ") {
		return true
	}
	return strings.HasPrefix(line, "export {") && strings.Contains(line, " from ")
}

func isTypeDeclarationStart(line string) bool {
	base := stripTSModifiers(line)
	return strings.HasPrefix(base, "interface ") ||
		strings.HasPrefix(base, "type ") ||
		strings.HasPrefix(base, "enum ")
}

func typeDeclarationEnd(lines []string, startIdx int) int {
	base := stripTSModifiers(strings.TrimSpace(lines[startIdx]))
	if strings.HasPrefix(base, "interface ") || strings.HasPrefix(base, "enum ") {
		return findMatchingBrace(lines, startIdx)
	}
	return findTypeEndLine(lines, startIdx)
}

// collectTSHeader gathers a declaration header while respecting nested
// parentheses/brackets and ignoring braces that belong to destructuring or
// object types. For arrow functions the header ends at the top-level =>.
func collectTSHeader(lines []string, startIdx int, arrow bool) string {
	var b strings.Builder
	parenDepth, bracketDepth, braceDepth := 0, 0, 0
	limit := startIdx + 20
	if limit > len(lines) {
		limit = len(lines)
	}

	for i := startIdx; i < limit; i++ {
		line := lines[i]
		var quote byte
		lineDone := false

		for j := 0; j < len(line); j++ {
			ch := line[j]

			if quote != 0 {
				if ch == '\\' && quote != '`' {
					j++
					continue
				}
				if ch == quote {
					quote = 0
				}
				continue
			}

			if ch == '/' && j+1 < len(line) {
				if line[j+1] == '/' {
					break
				}
				if line[j+1] == '*' {
					end := strings.Index(line[j+2:], "*/")
					if end == -1 {
						break
					}
					j += 2 + end + 1
					continue
				}
			}

			switch ch {
			case '\'', '"', '`':
				quote = ch
			case '(':
				parenDepth++
			case ')':
				parenDepth--
			case '[':
				bracketDepth++
			case ']':
				bracketDepth--
			case '=':
				// Allow one wrapper paren so memo(...) and forwardRef(...)
				// components still terminate at their arrow.
				if arrow && j+1 < len(line) && line[j+1] == '>' && parenDepth <= 1 && bracketDepth == 0 {
					b.WriteString(line[:j+2])
					return collapseTSWhitespace(b.String())
				}
			case '{':
				if !arrow && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
					b.WriteString(line[:j])
					lineDone = true
					break
				}
				braceDepth++
			case '}':
				braceDepth--
			}
			if lineDone {
				break
			}
		}

		if lineDone {
			break
		}

		b.WriteString(line)
		b.WriteString("\n")

		if !arrow && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 &&
			strings.HasSuffix(strings.TrimSpace(line), ";") {
			break
		}
	}

	return collapseTSWhitespace(b.String())
}

func collapseTSWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// emitClassMembers prints class method signatures (collapsing their bodies)
// with their original line numbers.
func emitClassMembers(lines []string, classIdx, classEndLine int, out *strings.Builder) {
	if classEndLine <= classIdx+1 {
		return
	}

	depth := 1
	for i := classIdx + 1; i < classEndLine && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") ||
			strings.HasPrefix(trimmed, "*") {
			continue
		}

		if depth == 1 && trimmed != "}" {
			if strings.HasPrefix(trimmed, "@") {
				continue
			}
			sig := collectTSSignature(lines, i, 10)
			name := ""
			if m := tsMethodPropRegex.FindStringSubmatch(sig); len(m) > 1 {
				name = m[1]
			} else if m := tsMethodRegex.FindStringSubmatch(sig); len(m) > 1 {
				name = m[1]
			}

			if name != "" && !tsIgnoredCallees[name] {
				header := collectTSHeader(lines, i, false)
				if header == "" {
					header = strings.TrimSuffix(sig, "{")
				}
				out.WriteString(fmt.Sprintf("%5d |   %s { /* ... */ }\n", i+1, header))
				endLine := findMatchingBrace(lines, i)
				i = endLine - 1
				depth = 1
				continue
			}
		}

		delta, _ := braceStats(lines[i])
		depth += delta
		if depth <= 0 {
			break
		}
	}
}

// collectTSSignature joins a declaration across a few lines so multi-line
// parameter lists still produce a readable signature.
func collectTSSignature(lines []string, startIdx, maxLookahead int) string {
	var parts []string
	limit := startIdx + maxLookahead
	if limit > len(lines) {
		limit = len(lines)
	}

	for i := startIdx; i < limit; i++ {
		part := strings.TrimSpace(lines[i])
		parts = append(parts, part)
		joined := strings.Join(parts, " ")
		if strings.Contains(joined, "{") || strings.HasSuffix(joined, ";") {
			break
		}
		if i > startIdx && part == "" {
			break
		}
	}

	return collapseTSWhitespace(strings.Join(parts, " "))
}

// parseTSClassMethods finds methods declared directly in a class body (brace
// depth 1) and records their exact line ranges.
func parseTSClassMethods(lines []string, classIdx, classEndLine int, className, filePath string) []targets.Symbol {
	if classEndLine <= classIdx+1 {
		return nil
	}

	var methods []targets.Symbol
	depth := 0

	for i := classIdx; i < classEndLine && i < len(lines); i++ {
		if i == classIdx {
			delta, _ := braceStats(lines[i])
			depth += delta
			continue
		}

		if depth == 1 {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed != "" &&
				!strings.HasPrefix(trimmed, "//") &&
				!strings.HasPrefix(trimmed, "/*") &&
				!strings.HasPrefix(trimmed, "*") &&
				!strings.HasPrefix(trimmed, "@") {

				sig := collectTSSignature(lines, i, 10)
				name := ""
				if m := tsMethodPropRegex.FindStringSubmatch(sig); len(m) > 1 {
					name = m[1]
				} else if m := tsMethodRegex.FindStringSubmatch(sig); len(m) > 1 {
					name = m[1]
				}

				if name != "" && !tsIgnoredCallees[name] {
					endLine := findMatchingBrace(lines, i)
					body := targets.SliceLines(lines, i+1, endLine)
					methods = append(methods, targets.Symbol{
						Name:      name,
						Receiver:  className,
						Kind:      targets.KindMethod,
						File:      filePath,
						StartLine: i + 1,
						EndLine:   endLine,
						Signature: sig,
						Body:      body,
						Calls:     extractTSCalls(body, name),
					})
					i = endLine - 1 // Continue after the method body.
					depth = 1
					continue
				}
			}
		}

		delta, _ := braceStats(lines[i])
		depth += delta
		if depth <= 0 {
			break
		}
	}

	return methods
}

// findMatchingBrace returns the 1-based line number where the block starting
// on startIdx (0-based) closes. It understands strings, line comments, and
// block comments.
func findMatchingBrace(lines []string, startIdx int) int {
	braceCount := 0
	foundFirst := false

	for i := startIdx; i < len(lines); i++ {
		delta, sawOpen := braceStats(lines[i])
		if sawOpen {
			foundFirst = true
		}
		braceCount += delta

		if foundFirst && braceCount <= 0 {
			return i + 1
		}
		if !foundFirst && strings.HasSuffix(strings.TrimSpace(lines[i]), ";") {
			return i + 1
		}
	}
	return len(lines)
}

// findTypeEndLine finds the end of a type alias without consuming the next
// declaration.
func findTypeEndLine(lines []string, startIdx int) int {
	depth := 0

	for i := startIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if i > startIdx && depth == 0 {
			if trimmed == "" || isTSDeclarationStart(trimmed) {
				return i
			}
		}

		depth += balanceDelta(lines[i])
		if depth < 0 {
			depth = 0
		}

		if depth == 0 && i > startIdx {
			if strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "}") || strings.HasSuffix(trimmed, "]") {
				return i + 1
			}
		}
		if depth == 0 && i == startIdx && strings.HasSuffix(trimmed, ";") {
			return i + 1
		}
	}
	return len(lines)
}

func isTSDeclarationStart(line string) bool {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"export default ", "export ", "declare ", "abstract "} {
		line = strings.TrimPrefix(line, prefix)
	}
	for _, prefix := range []string{"function ", "class ", "interface ", "type ", "const ", "let ", "var ", "import "} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// isTSCallSkippableLine ignores declaration headers when collecting call
// expressions. Unlike isTSDeclarationStart it does not skip const/let/var
// lines, because those commonly contain constructor calls or callbacks.
func isTSCallSkippableLine(line string) bool {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"export default ", "export ", "declare ", "abstract "} {
		line = strings.TrimPrefix(line, prefix)
	}
	for _, prefix := range []string{"function ", "class ", "interface ", "type ", "import "} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// braceStats counts brace depth changes on a line while ignoring braces inside
// strings and comments. sawOpen reports whether an opening brace was seen.
func braceStats(line string) (int, bool) {
	delta := 0
	sawOpen := false
	var quote byte

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if quote != 0 {
			if ch == '\\' && quote != '`' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		if ch == '/' && i+1 < len(line) {
			if line[i+1] == '/' {
				break
			}
			if line[i+1] == '*' {
				end := strings.Index(line[i+2:], "*/")
				if end == -1 {
					break
				}
				i += 2 + end + 1
				continue
			}
		}

		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '{':
			delta++
			sawOpen = true
		case '}':
			delta--
		}
	}
	return delta, sawOpen
}

// balanceDelta counts braces, brackets and parentheses for multi-line type
// aliases.
func balanceDelta(line string) int {
	delta := 0
	var quote byte

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if quote != 0 {
			if ch == '\\' && quote != '`' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		if ch == '/' && i+1 < len(line) {
			if line[i+1] == '/' {
				break
			}
			if line[i+1] == '*' {
				end := strings.Index(line[i+2:], "*/")
				if end == -1 {
					break
				}
				i += 2 + end + 1
				continue
			}
		}

		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '{', '[', '(':
			delta++
		case '}', ']', ')':
			delta--
		}
	}
	return delta
}

func extractTSCalls(body, self string) []string {
	var calls []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if isTSCallSkippableLine(trimmed) {
			continue
		}

		for _, m := range tsCallRegex.FindAllStringSubmatch(line, -1) {
			if len(m) < 2 {
				continue
			}
			callee := m[1]
			if callee == self || tsIgnoredCallees[callee] || seen[callee] {
				continue
			}
			seen[callee] = true
			calls = append(calls, callee)
		}
	}
	return calls
}

func extractTSClassRefs(sig string) []string {
	var refs []string
	seen := make(map[string]bool)

	add := func(value string) {
		for _, part := range strings.Split(value, ",") {
			name := cleanTypeName(part)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			refs = append(refs, name)
		}
	}

	if m := tsExtendsRegex.FindStringSubmatch(sig); len(m) > 1 {
		value := m[1]
		if idx := strings.Index(strings.ToLower(value), " implements "); idx >= 0 {
			value = value[:idx]
		}
		add(value)
	}
	if m := tsImplementsRegex.FindStringSubmatch(sig); len(m) > 1 {
		add(m[1])
	}
	return refs
}

func cleanTypeName(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexAny(value, "<{("); idx >= 0 {
		value = value[:idx]
	}
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]{}(),;:'\"")
	value = strings.TrimPrefix(value, "typeof ")
	return strings.TrimSpace(value)
}
