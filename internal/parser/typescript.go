package parser

import (
	"regexp"
	"strings"
)

var (
	// Matches function signatures (including async, generators and generics).
	tsFuncRegex = regexp.MustCompile(`^(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s*(\*?\s*[A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*(\([^{;]*\))`)

	// Matches arrow-function constants, including optional type annotations.
	tsArrowRegex = regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:<[^>]*>\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>\s*\{?`)

	// Matches class, interface and type-alias declarations.
	tsClassRegex     = regexp.MustCompile(`^(?:export\s+(?:default\s+)?)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s*<[^>]*>)?(?:\s+extends\s+[^{]+)?(?:\s+implements\s+[^{]+)?`)
	tsInterfaceRegex = regexp.MustCompile(`^(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s*<[^>]*>)?(?:\s+extends\s+[^{]+)?`)
	tsTypeRegex      = regexp.MustCompile(`^(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s*<[^>]*>)?\s*=`)

	// Matches methods and arrow-function class properties.
	tsMethodRegex     = regexp.MustCompile(`^(?:(?:public|private|protected|static|readonly|abstract|override|declare|async|get|set)\s+)*(?:\*\s*)?([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*\([^)]*\)\s*(?::[^;]*)?(?:\{|;|$)`)
	tsMethodPropRegex = regexp.MustCompile(`^(?:(?:public|private|protected|static|readonly|abstract|override|declare|async)\s+)*([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:<[^>]*>\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`)

	tsCallRegex = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*(?:\.[A-Za-z_$][A-Za-z0-9_$]*)?)\s*\(`)

	tsExtendsRegex    = regexp.MustCompile(`\bextends\s+([^{]+)`)
	tsImplementsRegex = regexp.MustCompile(`\bimplements\s+([^{]+)`)
)

var tsIgnoredCallees = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"function": true, "return": true, "import": true, "require": true,
	"with": true, "typeof": true, "delete": true, "void": true, "await": true,
	"yield": true, "super": true, "throw": true, "new": true,
}

// ParseTypeScriptFile parses TypeScript or JavaScript source into symbols.
// It is intentionally dependency-free: a lightweight, brace-aware scanner
// provides exact 1-based declaration ranges without requiring node/tsc.
func ParseTypeScriptFile(filePath string, src []byte) ([]Symbol, error) {
	lines := strings.Split(string(src), "\n")
	var symbols []Symbol

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineNum := idx + 1

		// 1. Regular functions.
		if match := tsFuncRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := strings.TrimPrefix(strings.TrimSpace(match[1]), "*")
			endLine := findMatchingBrace(lines, idx)
			body := sliceLines(lines, lineNum, endLine)

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      KindFunction,
				File:      filePath,
				StartLine: lineNum,
				EndLine:   endLine,
				Signature: collectTSSignature(lines, idx, 6),
				Body:      body,
				Calls:     extractTSCalls(body, name),
			})
			continue
		}

		// 2. Arrow-function constants.
		if match := tsArrowRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := match[1]
			endLine := findMatchingBrace(lines, idx)
			body := sliceLines(lines, lineNum, endLine)

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      KindFunction,
				File:      filePath,
				StartLine: lineNum,
				EndLine:   endLine,
				Signature: collectTSSignature(lines, idx, 6),
				Body:      body,
				Calls:     extractTSCalls(body, name),
			})
			continue
		}

		// 3. Classes.
		if match := tsClassRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			name := match[1]
			endLine := findMatchingBrace(lines, idx)
			body := sliceLines(lines, lineNum, endLine)
			sig := collectTSSignature(lines, idx, 8)

			symbols = append(symbols, Symbol{
				Name:       name,
				Kind:       KindClass,
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
			body := sliceLines(lines, lineNum, endLine)
			sig := collectTSSignature(lines, idx, 8)

			symbols = append(symbols, Symbol{
				Name:       name,
				Kind:       KindInterface,
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
			body := sliceLines(lines, lineNum, endLine)

			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      KindType,
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

// parseTSClassMethods finds methods declared directly in a class body (brace
// depth 1) and records their exact line ranges.
func parseTSClassMethods(lines []string, classIdx, classEndLine int, className, filePath string) []Symbol {
	if classEndLine <= classIdx+1 {
		return nil
	}

	var methods []Symbol
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
					body := sliceLines(lines, i+1, endLine)
					methods = append(methods, Symbol{
						Name:      name,
						Receiver:  className,
						Kind:      KindMethod,
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

	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// findMatchingBrace returns the 1-based line number where the block starting
// on startIdx (0-based) closes.
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
