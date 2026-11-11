package jvm

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

var (
	javaTypeDeclRegex = regexp.MustCompile(`\b(?:public\s+|protected\s+|private\s+|static\s+|final\s+|abstract\s+|sealed\s+|non-sealed\s+)*(class|interface|enum|record|@interface)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	javaMethodRegex   = regexp.MustCompile(`\b(?:public\s+|protected\s+|private\s+|static\s+|final\s+|abstract\s+|synchronized\s+|default\s+|native\s+)*(?:<[^>]+>\s*)?([A-Za-z0-9_$<>\[\], ?]+)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	javaCtorRegex     = regexp.MustCompile(`\b(?:public\s+|protected\s+|private\s+)*([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
)

// ExtractJavaSignatures generates a compact, line-numbered signature representation
// for Java source files, preserving package, imports, annotations, interfaces, records,
// enum constants, and field definitions while collapsing method bodies.
func ExtractJavaSignatures(content string) string {
	lines := strings.Split(content, "\n")
	var out strings.Builder
	var state codeState

	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]
		lineNum := idx + 1
		clean, delta, _ := state.processLine(line)
		trimmed := strings.TrimSpace(clean)
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
		if strings.HasPrefix(origTrimmed, "@") && !strings.Contains(clean, ";") {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			continue
		}

		// 3. Type declarations (class, interface, record, enum)
		if m := javaTypeDeclRegex.FindStringSubmatch(clean); len(m) > 2 {
			kind := m[1]
			endBrace := findMatchingBrace(lines, idx)

			header := collectJavaHeader(lines, idx)
			out.WriteString(fmt.Sprintf("%5d | %s {\n", lineNum, header))

			emitJavaClassMembers(lines, idx, endBrace, kind, &out)

			if endBrace > idx && endBrace <= len(lines) {
				out.WriteString(fmt.Sprintf("%5d | }\n", endBrace))
			}
			idx = endBrace - 1
			continue
		}

		// 4. Closing braces
		if origTrimmed == "}" || origTrimmed == "};" {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
		}
		_ = delta
		_ = trimmed
	}

	return out.String()
}

func collectJavaHeader(lines []string, startIdx int) string {
	var parts []string
	for i := startIdx; i < len(lines) && i < startIdx+8; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if braceIdx := strings.Index(trimmed, "{"); braceIdx >= 0 {
			parts = append(parts, strings.TrimSpace(trimmed[:braceIdx]))
			break
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func emitJavaClassMembers(lines []string, classStartIdx, classEndIdx int, classKind string, out *strings.Builder) {
	depth := 1
	var state codeState

	for i := classStartIdx + 1; i < classEndIdx && i < len(lines); i++ {
		line := lines[i]
		lineNum := i + 1
		clean, delta, _ := state.processLine(line)
		trimmedClean := strings.TrimSpace(clean)
		origTrimmed := strings.TrimSpace(line)

		if origTrimmed == "" {
			continue
		}

		if depth == 1 {
			// Enum constants (before first semicolon)
			if classKind == "enum" && !strings.Contains(trimmedClean, "(") && !strings.HasPrefix(trimmedClean, "public") && !strings.HasPrefix(trimmedClean, "private") {
				out.WriteString(fmt.Sprintf("%5d |   %s\n", lineNum, origTrimmed))
				depth += delta
				continue
			}

			// Annotations on members
			if strings.HasPrefix(origTrimmed, "@") {
				out.WriteString(fmt.Sprintf("%5d |   %s\n", lineNum, origTrimmed))
				depth += delta
				continue
			}

			// Method / Constructor
			if strings.Contains(trimmedClean, "(") {
				header := collectJavaHeader(lines, i)
				if strings.Contains(clean, "{") || (i+1 < len(lines) && strings.Contains(lines[i+1], "{")) {
					out.WriteString(fmt.Sprintf("%5d |   %s { /* ... */ }\n", lineNum, header))
					endMethod := findMatchingBrace(lines, i)
					i = endMethod - 1
					depth = 1
					continue
				} else if strings.HasSuffix(origTrimmed, ";") {
					out.WriteString(fmt.Sprintf("%5d |   %s\n", lineNum, origTrimmed))
					depth += delta
					continue
				}
			}

			// Fields (preserve annotated, public, protected, static final fields)
			if strings.HasSuffix(origTrimmed, ";") {
				out.WriteString(fmt.Sprintf("%5d |   %s\n", lineNum, origTrimmed))
				depth += delta
				continue
			}
		}

		depth += delta
		if depth <= 0 {
			break
		}
	}
}

// ParseJavaFile extracts classes, records, interfaces, enums, constructors, and methods
// into structured targets.Symbol objects.
func ParseJavaFile(filePath string, src []byte) ([]targets.Symbol, error) {
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

		// Type declaration
		if m := javaTypeDeclRegex.FindStringSubmatch(clean); len(m) > 2 {
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

			endLine := findMatchingBrace(lines, idx)
			header := collectJavaHeader(lines, idx)
			body := targets.SliceLines(lines, startLine, endLine)

			kind := targets.KindClass
			if typeKindStr == "interface" || typeKindStr == "@interface" {
				kind = targets.KindInterface
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

			// Extract members of this type
			memberSymbols := parseJavaMembers(lines, idx, endLine, typeName, filePath)
			symbols = append(symbols, memberSymbols...)

			idx = endLine - 1
			continue
		}
	}

	return symbols, nil
}

func parseJavaMembers(lines []string, classStartIdx, classEndIdx int, className, filePath string) []targets.Symbol {
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
			trimmedClean := strings.TrimSpace(clean)
			if strings.Contains(trimmedClean, "(") && !jvmIgnoredCallees[trimmedClean] {
				name := ""
				if m := javaMethodRegex.FindStringSubmatch(clean); len(m) > 2 {
					name = m[2]
				} else if m := javaCtorRegex.FindStringSubmatch(clean); len(m) > 1 {
					name = m[1]
				}

				if name != "" && !jvmIgnoredCallees[name] {
					mStartLine := i + 1
					for k := i - 1; k >= classStartIdx; k-- {
						prevTrimmed := strings.TrimSpace(lines[k])
						if strings.HasPrefix(prevTrimmed, "@") {
							mStartLine = k + 1
						} else if prevTrimmed != "" {
							break
						}
					}

					header := collectJavaHeader(lines, i)
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
		}

		depth += delta
		if depth <= 0 {
			break
		}
	}

	return methods
}
