package jvm

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

var (
	ktTypeDeclRegex = regexp.MustCompile(`\b(?:data\s+|sealed\s+|value\s+|enum\s+|fun\s+|inner\s+|open\s+|abstract\s+|private\s+|protected\s+|internal\s+|public\s+)*(class|interface|object|typealias)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	ktFuncRegex     = regexp.MustCompile(`\b(?:suspend\s+|inline\s+|operator\s+|infix\s+|override\s+|open\s+|abstract\s+|private\s+|protected\s+|internal\s+|public\s+|tailrec\s+|external\s+)*fun\s*(?:<[^>]+>\s*)?([A-Za-z_$][A-Za-z0-9_$]*(?:\.[A-Za-z_$][A-Za-z0-9_$]*)?)\s*\(`)
)

// ExtractKotlinSignatures produces a line-anchored signature overview for Kotlin
// files, keeping package, imports, data classes, interfaces, object headers, and
// property declarations while collapsing function bodies.
func ExtractKotlinSignatures(content string) string {
	lines := strings.Split(content, "\n")
	var out strings.Builder
	var state codeState

	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]
		lineNum := idx + 1
		clean, delta, _ := state.processLine(line)
		origTrimmed := strings.TrimSpace(line)

		if origTrimmed == "" {
			continue
		}

		// 1. Package & imports
		if strings.HasPrefix(origTrimmed, "package ") || strings.HasPrefix(origTrimmed, "import ") {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			continue
		}

		// 2. Annotations
		if strings.HasPrefix(origTrimmed, "@") {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			continue
		}

		// 3. Data class without block body
		if strings.Contains(clean, "data class ") && !strings.Contains(clean, "{") {
			endClass := findKotlinDeclEnd(lines, idx)
			for j := idx; j < endClass && j < len(lines); j++ {
				out.WriteString(fmt.Sprintf("%5d | %s\n", j+1, lines[j]))
			}
			idx = endClass - 1
			continue
		}

		// 4. Type declarations with block body
		if m := ktTypeDeclRegex.FindStringSubmatch(clean); len(m) > 2 {
			bodyOpen := findKotlinBodyOpen(lines, idx)
			var endBrace int
			if bodyOpen >= 0 {
				endBrace = findMatchingBrace(lines, idx)
			} else {
				endBrace = findKotlinDeclEnd(lines, idx)
			}
			header := collectKotlinHeader(lines, idx)
			out.WriteString(fmt.Sprintf("%5d | %s {\n", lineNum, header))

			if bodyOpen >= 0 {
				emitKotlinClassMembers(lines, bodyOpen, endBrace, &out)
			}

			if endBrace > idx && endBrace <= len(lines) {
				out.WriteString(fmt.Sprintf("%5d | }\n", endBrace))
			}
			idx = endBrace - 1
			continue
		}

		// 5. Top-level functions
		if m := ktFuncRegex.FindStringSubmatch(clean); len(m) > 1 {
			header := collectKotlinHeader(lines, idx)
			if strings.Contains(clean, "{") || (idx+1 < len(lines) && strings.Contains(lines[idx+1], "{")) {
				out.WriteString(fmt.Sprintf("%5d | %s { /* ... */ }\n", lineNum, header))
				idx = findMatchingBrace(lines, idx) - 1
			} else {
				out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, origTrimmed))
			}
			continue
		}

		// 6. Closing braces
		if origTrimmed == "}" || origTrimmed == "};" {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
		}
		_ = delta
	}

	return out.String()
}

func collectKotlinHeader(lines []string, startIdx int) string {
	var parts []string
	for i := startIdx; i < len(lines) && i < startIdx+8; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if braceIdx := strings.Index(trimmed, "{"); braceIdx >= 0 {
			parts = append(parts, strings.TrimSpace(trimmed[:braceIdx]))
			break
		}
		parts = append(parts, trimmed)
		if strings.Contains(trimmed, " = ") {
			break
		}
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func findKotlinDeclEnd(lines []string, startIdx int) int {
	parenDepth := 0
	foundParen := false

	for i := startIdx; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '(' {
				parenDepth++
				foundParen = true
			} else if ch == ')' {
				parenDepth--
			}
		}
		if foundParen && parenDepth <= 0 {
			return i + 1
		}
		if !foundParen && strings.HasSuffix(strings.TrimSpace(lines[i]), ";") {
			return i + 1
		}
	}
	return startIdx + 1
}

// findKotlinBodyOpen locates the line holding the opening brace of a Kotlin type
// declaration body. Declarations may span multiple lines when the primary
// constructor wraps; the body brace then appears on the line closing the
// constructor, or on a line starting with '{'. Returns -1 for bodyless
// declarations such as `data class User(...) : Serializable`.
func findKotlinBodyOpen(lines []string, startIdx int) int {
	var state codeState
	parenDepth := 0
	sawParen := false

	for i := startIdx; i < len(lines) && i <= startIdx+8; i++ {
		clean, _, _ := state.processLine(lines[i])
		ctorClosedHere := false

		// Position-aware scan: a body brace only counts once the primary
		// constructor parentheses are balanced.
		for j := 0; j < len(clean); j++ {
			switch clean[j] {
			case '(':
				parenDepth++
				sawParen = true
			case ')':
				if parenDepth > 0 {
					parenDepth--
					if parenDepth == 0 {
						ctorClosedHere = true
					}
				}
			case '{':
				if parenDepth == 0 {
					return i
				}
			}
		}

		trimmed := strings.TrimSpace(clean)

		if sawParen {
			if parenDepth > 0 {
				continue // still inside the primary constructor parameter list
			}
			if ctorClosedHere {
				// The brace would already have matched in the scan above when
				// it sits on this line; allow a lone `{` on the next line.
				if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "{") {
					return i + 1
				}
				return -1
			}
			// Constructor closed on an earlier line; only supertype
			// continuations may follow before the (optional) body.
			if strings.HasPrefix(trimmed, ":") || strings.HasPrefix(trimmed, "where") || strings.HasPrefix(trimmed, "{") {
				continue
			}
			return -1
		}

		// No primary constructor seen yet.
		if trimmed == "" {
			return -1
		}
		if i > startIdx && !strings.HasPrefix(trimmed, ":") && !strings.HasPrefix(trimmed, "{") {
			return -1
		}
		if i == startIdx {
			// Peek at the next line; only `{` or `:` continuations keep the
			// search alive, anything else ends the declaration.
			if i+1 >= len(lines) {
				return -1
			}
			next := strings.TrimSpace(lines[i+1])
			if !strings.HasPrefix(next, "{") && !strings.HasPrefix(next, ":") {
				return -1
			}
		}
	}

	return -1
}

func emitKotlinClassMembers(lines []string, classStartIdx, classEndIdx int, out *strings.Builder) {
	depth := 1
	var state codeState

	for i := classStartIdx + 1; i < classEndIdx && i < len(lines); i++ {
		line := lines[i]
		lineNum := i + 1
		clean, delta, _ := state.processLine(line)
		origTrimmed := strings.TrimSpace(line)

		if origTrimmed == "" {
			continue
		}

		if depth == 1 {
			// Annotations on member
			if strings.HasPrefix(origTrimmed, "@") {
				out.WriteString(fmt.Sprintf("%5d |   %s\n", lineNum, origTrimmed))
				depth += delta
				continue
			}

			// Properties (val / var)
			if strings.HasPrefix(origTrimmed, "val ") || strings.HasPrefix(origTrimmed, "var ") ||
				strings.HasPrefix(origTrimmed, "const ") || strings.Contains(origTrimmed, " val ") || strings.Contains(origTrimmed, " var ") {
				out.WriteString(fmt.Sprintf("%5d |   %s\n", lineNum, origTrimmed))
				depth += delta
				continue
			}

			// Member functions
			if m := ktFuncRegex.FindStringSubmatch(clean); len(m) > 1 {
				header := collectKotlinHeader(lines, i)
				if strings.Contains(clean, "{") || (i+1 < len(lines) && strings.Contains(lines[i+1], "{")) {
					out.WriteString(fmt.Sprintf("%5d |   %s { /* ... */ }\n", lineNum, header))
					endMethod := findMatchingBrace(lines, i)
					i = endMethod - 1
					depth = 1
					continue
				} else {
					out.WriteString(fmt.Sprintf("%5d |   %s\n", lineNum, origTrimmed))
					depth += delta
					continue
				}
			}
		}

		depth += delta
		if depth <= 0 {
			break
		}
	}
}

// ParseKotlinFile extracts Kotlin classes, interfaces, objects, data classes,
// top-level functions, and member methods into structured targets.Symbol objects.
func ParseKotlinFile(filePath string, src []byte) ([]targets.Symbol, error) {
	lines := strings.Split(string(src), "\n")
	var symbols []targets.Symbol
	var state codeState

	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]
		clean, _, _ := state.processLine(line)
		origTrimmed := strings.TrimSpace(line)

		if origTrimmed == "" || strings.HasPrefix(origTrimmed, "//") || strings.HasPrefix(origTrimmed, "/*") {
			continue
		}

		// 1. Types (class, interface, object, typealias)
		if m := ktTypeDeclRegex.FindStringSubmatch(clean); len(m) > 2 {
			typeKindStr := m[1]
			typeName := m[2]

			startLine := idx + 1
			for k := idx - 1; k >= 0; k-- {
				prevTrimmed := strings.TrimSpace(lines[k])
				if strings.HasPrefix(prevTrimmed, "@") {
					startLine = k + 1
				} else if prevTrimmed != "" {
					break
				}
			}

			bodyOpen := findKotlinBodyOpen(lines, idx)
			var endLine int
			if bodyOpen >= 0 {
				endLine = findMatchingBrace(lines, idx)
			} else {
				endLine = findKotlinDeclEnd(lines, idx)
			}

			header := collectKotlinHeader(lines, idx)
			body := targets.SliceLines(lines, startLine, endLine)

			kind := targets.KindClass
			if typeKindStr == "interface" {
				kind = targets.KindInterface
			} else if typeKindStr == "typealias" {
				kind = targets.KindType
			}

			symbols = append(symbols, targets.Symbol{
				Name:       typeName,
				Kind:       kind,
				File:       filePath,
				StartLine:  startLine,
				EndLine:    endLine,
				Signature:  header,
				Body:       body,
				References: extractJVMTypeReferences(header),
			})

			// If type has a block body, parse its methods
			if bodyOpen >= 0 {
				memberSymbols := parseKotlinMembers(lines, bodyOpen, endLine, typeName, filePath)
				symbols = append(symbols, memberSymbols...)
			}

			idx = endLine - 1
			continue
		}

		// 2. Top-level functions (including extension functions)
		if m := ktFuncRegex.FindStringSubmatch(clean); len(m) > 1 {
			rawName := m[1]
			receiver := ""
			name := rawName

			if strings.Contains(rawName, ".") {
				parts := strings.Split(rawName, ".")
				receiver = parts[0]
				name = parts[1]
			}

			startLine := idx + 1
			for k := idx - 1; k >= 0; k-- {
				prevTrimmed := strings.TrimSpace(lines[k])
				if strings.HasPrefix(prevTrimmed, "@") {
					startLine = k + 1
				} else if prevTrimmed != "" {
					break
				}
			}

			header := collectKotlinHeader(lines, idx)

			var endLine int
			var body string
			var calls []string

			if strings.Contains(clean, "{") || (idx+1 < len(lines) && strings.Contains(lines[idx+1], "{")) {
				endLine = findMatchingBrace(lines, idx)
				body = targets.SliceLines(lines, startLine, endLine)
				calls = extractJVMCalls(body, name)
				idx = endLine - 1
			} else {
				endLine = idx + 1
				body = targets.SliceLines(lines, startLine, endLine)
			}

			symbols = append(symbols, targets.Symbol{
				Name:       name,
				Receiver:   receiver,
				Kind:       targets.KindFunction,
				File:       filePath,
				StartLine:  startLine,
				EndLine:    endLine,
				Signature:  header,
				Body:       body,
				Calls:      calls,
				References: extractJVMTypeReferences(header),
			})
			continue
		}
	}

	return symbols, nil
}

func parseKotlinMembers(lines []string, classStartIdx, classEndIdx int, className, filePath string) []targets.Symbol {
	var methods []targets.Symbol
	depth := 0
	var state codeState

	for i := classStartIdx; i < classEndIdx && i < len(lines); i++ {
		clean, delta, _ := state.processLine(lines[i])

		if i == classStartIdx {
			depth += delta
			continue
		}

		if depth == 1 {
			if m := ktFuncRegex.FindStringSubmatch(clean); len(m) > 1 {
				name := m[1]
				if strings.Contains(name, ".") {
					parts := strings.Split(name, ".")
					name = parts[len(parts)-1]
				}

				mStartLine := i + 1
				for k := i - 1; k >= classStartIdx; k-- {
					prevTrimmed := strings.TrimSpace(lines[k])
					if strings.HasPrefix(prevTrimmed, "@") {
						mStartLine = k + 1
					} else if prevTrimmed != "" {
						break
					}
				}

				header := collectKotlinHeader(lines, i)
				var mEndLine int
				var mBody string
				var calls []string

				if strings.Contains(clean, "{") || (i+1 < len(lines) && strings.Contains(lines[i+1], "{")) {
					mEndLine = findMatchingBrace(lines, i)
					mBody = targets.SliceLines(lines, mStartLine, mEndLine)
					calls = extractJVMCalls(mBody, name)
					i = mEndLine - 1
					depth = 1
				} else {
					mEndLine = i + 1
					mBody = targets.SliceLines(lines, mStartLine, mEndLine)
					depth += delta
				}

				methods = append(methods, targets.Symbol{
					Name:       name,
					Receiver:   className,
					Kind:       targets.KindMethod,
					File:       filePath,
					StartLine:  mStartLine,
					EndLine:    mEndLine,
					Signature:  header,
					Body:       mBody,
					Calls:      calls,
					References: extractJVMTypeReferences(header),
				})
				continue
			}
		}

		depth += delta
		if depth <= 0 {
			break
		}
	}

	return methods
}
