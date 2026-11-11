package jvm

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

func init() {
	targets.RegisterTarget(&JavaTarget{})
	targets.RegisterTarget(&KotlinTarget{})
}

// JavaTarget implements processing for Java source files.
type JavaTarget struct{}

func (j *JavaTarget) Name() string {
	return "java"
}

func (j *JavaTarget) Match(filePath string) bool {
	return strings.ToLower(filepath.Ext(filePath)) == ".java"
}

func (j *JavaTarget) ExtractSignatures(filePath string, content string) string {
	return ExtractJavaSignatures(content)
}

func (j *JavaTarget) Parse(filePath string, src []byte) ([]targets.Symbol, error) {
	return ParseJavaFile(filePath, src)
}

// KotlinTarget implements processing for Kotlin source files and scripts.
type KotlinTarget struct{}

func (k *KotlinTarget) Name() string {
	return "kotlin"
}

func (k *KotlinTarget) Match(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return ext == ".kt" || ext == ".kts"
}

func (k *KotlinTarget) ExtractSignatures(filePath string, content string) string {
	return ExtractKotlinSignatures(content)
}

func (k *KotlinTarget) Parse(filePath string, src []byte) ([]targets.Symbol, error) {
	return ParseKotlinFile(filePath, src)
}

// codeState tracks comments and string literals across lines for JVM languages.
type codeState struct {
	inBlockComment bool
	inRawString    bool
}

// processLine removes comments and string contents from line, replacing them with
// spaces to preserve column indexes, and computes the net brace change.
func (s *codeState) processLine(line string) (string, int, bool) {
	var clean strings.Builder
	clean.Grow(len(line))
	delta := 0
	sawOpen := false

	i := 0
	n := len(line)

	for i < n {
		if s.inBlockComment {
			if i+1 < n && line[i] == '*' && line[i+1] == '/' {
				s.inBlockComment = false
				clean.WriteString("  ")
				i += 2
			} else {
				clean.WriteByte(' ')
				i++
			}
			continue
		}

		if s.inRawString {
			if i+2 < n && line[i] == '"' && line[i+1] == '"' && line[i+2] == '"' {
				s.inRawString = false
				clean.WriteString("   ")
				i += 3
			} else {
				clean.WriteByte(' ')
				i++
			}
			continue
		}

		// Triple-quoted raw string literal: """
		if i+2 < n && line[i] == '"' && line[i+1] == '"' && line[i+2] == '"' {
			clean.WriteString("   ")
			i += 3
			// Check if closed on same line
			closed := false
			for j := i; j+2 < n; j++ {
				if line[j] == '"' && line[j+1] == '"' && line[j+2] == '"' {
					for k := i; k <= j+2; k++ {
						clean.WriteByte(' ')
					}
					i = j + 3
					closed = true
					break
				}
			}
			if !closed {
				s.inRawString = true
			}
			continue
		}

		// Single-line comment: //
		if i+1 < n && line[i] == '/' && line[i+1] == '/' {
			for k := i; k < n; k++ {
				clean.WriteByte(' ')
			}
			break
		}

		// Block comment: /*
		if i+1 < n && line[i] == '/' && line[i+1] == '*' {
			clean.WriteString("  ")
			i += 2
			closed := false
			for j := i; j+1 < n; j++ {
				if line[j] == '*' && line[j+1] == '/' {
					for k := i; k <= j+1; k++ {
						clean.WriteByte(' ')
					}
					i = j + 2
					closed = true
					break
				}
			}
			if !closed {
				s.inBlockComment = true
			}
			continue
		}

		// Double-quoted string literal: "..."
		if line[i] == '"' {
			clean.WriteByte(' ')
			i++
			for i < n {
				if line[i] == '\\' {
					clean.WriteString("  ")
					i += 2
					continue
				}
				if line[i] == '"' {
					clean.WriteByte(' ')
					i++
					break
				}
				clean.WriteByte(' ')
				i++
			}
			continue
		}

		// Character literal: '...'
		if line[i] == '\'' {
			clean.WriteByte(' ')
			i++
			for i < n {
				if line[i] == '\\' {
					clean.WriteString("  ")
					i += 2
					continue
				}
				if line[i] == '\'' {
					clean.WriteByte(' ')
					i++
					break
				}
				clean.WriteByte(' ')
				i++
			}
			continue
		}

		ch := line[i]
		clean.WriteByte(ch)
		if ch == '{' {
			delta++
			sawOpen = true
		} else if ch == '}' {
			delta--
		}
		i++
	}

	return clean.String(), delta, sawOpen
}

func findMatchingBrace(lines []string, startIdx int) int {
	braceCount := 0
	foundFirst := false
	var state codeState

	for i := startIdx; i < len(lines); i++ {
		_, delta, sawOpen := state.processLine(lines[i])
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

var jvmCallRegex = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*(?:\.[A-Za-z_$][A-Za-z0-9_$]*)*)\s*\(`)

var jvmIgnoredCallees = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"synchronized": true, "super": true, "this": true, "new": true, "return": true,
	"throw": true, "assert": true, "when": true, "is": true, "as": true, "in": true,
	"try": true, "finally": true, "else": true, "do": true, "typeof": true,
}

func extractJVMCalls(body string, self string) []string {
	var calls []string
	seen := make(map[string]bool)
	var state codeState

	for _, line := range strings.Split(body, "\n") {
		clean, _, _ := state.processLine(line)
		trimmed := strings.TrimSpace(clean)
		if trimmed == "" {
			continue
		}

		for _, m := range jvmCallRegex.FindAllStringSubmatch(clean, -1) {
			if len(m) < 2 {
				continue
			}
			callee := m[1]
			lastPart := callee
			if idx := strings.LastIndex(callee, "."); idx >= 0 {
				lastPart = callee[idx+1:]
			}
			if callee == self || lastPart == self || jvmIgnoredCallees[callee] || jvmIgnoredCallees[lastPart] || seen[callee] {
				continue
			}
			seen[callee] = true
			calls = append(calls, callee)
		}
	}
	return calls
}

func extractJVMTypeReferences(text string) []string {
	var refs []string
	seen := make(map[string]bool)

	typeIdentRegex := regexp.MustCompile(`\b([A-Z][A-Za-z0-9_$]*)\b`)
	for _, m := range typeIdentRegex.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if seen[name] || name == "String" || name == "Object" || name == "Class" {
			continue
		}
		seen[name] = true
		refs = append(refs, name)
	}
	return refs
}

// ExtractJVMCase handles AST switch-case branch extraction for Java switches
// and Kotlin when-expressions.
func ExtractJVMCase(filePath, content, fnName, caseTarget string) (string, int, int, error) {
	lines := strings.Split(content, "\n")
	target := strings.Trim(strings.TrimSpace(caseTarget), "\"'`")
	if target == "" {
		return "", 0, 0, targets.ErrCaseNotFound(caseTarget)
	}

	startSearch := 0
	endSearch := len(lines)

	if fnName != "" {
		cleanFn := strings.TrimPrefix(strings.TrimSpace(fnName), "*")
		if idx := strings.LastIndex(cleanFn, "."); idx >= 0 {
			cleanFn = cleanFn[idx+1:]
		}
		for i, l := range lines {
			if strings.Contains(l, cleanFn) && strings.Contains(l, "(") {
				startSearch = i
				endSearch = findMatchingBrace(lines, i)
				break
			}
		}
	}

	matchIdx := -1
	targetPattern := regexp.QuoteMeta(target)
	isDefault := strings.EqualFold(target, "default") || strings.EqualFold(target, "else")

	// Regex for Java switch case and Kotlin when branch. The Kotlin pattern
	// tolerates quoted targets such as `"sub" ->` and type checks like
	// `is Target ->`.
	javaCaseRegex := regexp.MustCompile(`(?i)^\s*case\s+.*` + targetPattern + `.*(?::|->)`)
	javaDefaultRegex := regexp.MustCompile(`(?i)^\s*default\s*(?::|->)`)
	kotlinWhenRegex := regexp.MustCompile(`(?i)^\s*(?:is\s+)?.*` + targetPattern + "[\"'`]?\\s*->")
	kotlinElseRegex := regexp.MustCompile(`(?i)^\s*else\s*->`)

	for i := startSearch; i < endSearch && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isDefault {
			if javaDefaultRegex.MatchString(trimmed) || kotlinElseRegex.MatchString(trimmed) {
				matchIdx = i
				break
			}
		} else {
			if javaCaseRegex.MatchString(trimmed) || kotlinWhenRegex.MatchString(trimmed) {
				matchIdx = i
				break
			}
		}
	}

	if matchIdx == -1 {
		return "", 0, 0, targets.ErrCaseNotFound(caseTarget)
	}

	startLine := matchIdx + 1
	endLine := startLine

	if strings.Contains(lines[matchIdx], "{") {
		endLine = findMatchingBrace(lines, matchIdx)
	} else {
		for j := matchIdx + 1; j < endSearch && j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "case ") || strings.HasPrefix(trimmed, "default:") ||
				strings.HasPrefix(trimmed, "default ->") || strings.HasPrefix(trimmed, "else ->") ||
				strings.Contains(trimmed, "->") || trimmed == "}" {
				endLine = j
				break
			}
			endLine = j + 1
		}
	}

	if endLine < startLine {
		endLine = startLine
	}

	body := targets.SliceLines(lines, startLine, endLine)
	return body, startLine, endLine, nil
}
