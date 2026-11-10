package css

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

func init() {
	targets.RegisterTarget(&CSSTarget{})
}

// CSSTarget implements processing for CSS, SCSS, SASS, and LESS stylesheets.
type CSSTarget struct{}

func (c *CSSTarget) Name() string {
	return "css"
}

func (c *CSSTarget) Match(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".css", ".scss", ".sass", ".less":
		return true
	}
	return false
}

func (c *CSSTarget) ExtractSignatures(filePath string, content string) string {
	return ExtractSignatures(content)
}

func (c *CSSTarget) Parse(filePath string, src []byte) ([]targets.Symbol, error) {
	return nil, nil
}

// ExtractSignatures preserves :root custom properties, top-level variables,
// @import/@use statements, media queries, keyframes, and other structural
// at-rules while collapsing ordinary rule property lists into compact stubs.
// This typically removes 85%+ of a stylesheet's tokens while keeping the
// selectors and design tokens needed to understand it.
func ExtractSignatures(content string) string {
	lines := strings.Split(content, "\n")
	var out strings.Builder

	inRoot := false
	rootDepth := 0

	inRule := false
	ruleDepth := 0

	for idx, line := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if inRoot {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			rootDepth += braceDelta(line)
			if rootDepth <= 0 {
				inRoot = false
			}
			continue
		}

		if inRule {
			ruleDepth += braceDelta(line)
			if ruleDepth <= 0 {
				inRule = false
			}
			continue
		}

		// Preserve :root CSS custom variable blocks in full.
		if strings.HasPrefix(trimmed, ":root") {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			rootDepth = braceDelta(line)
			if rootDepth > 0 {
				inRoot = true
			}
			continue
		}

		// Preserve top-level SCSS/LESS variable declarations.
		if isVariableDeclaration(trimmed) {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			continue
		}

		// Preserve structural at-rules and their headers.
		if strings.HasPrefix(trimmed, "@") {
			if isCollapsibleAtRule(trimmed) {
				header := strings.TrimSpace(strings.SplitN(trimmed, "{", 2)[0])
				out.WriteString(fmt.Sprintf("%5d | %s { /* ... */ }\n", lineNum, header))
				if delta := braceDelta(trimmed); delta > 0 {
					inRule = true
					ruleDepth = delta
				}
				continue
			}

			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			continue
		}

		// Collapse ordinary selector rule blocks.
		if braceIdx := strings.Index(trimmed, "{"); braceIdx >= 0 {
			selector := strings.TrimSpace(trimmed[:braceIdx])
			if selector == "" {
				continue
			}
			out.WriteString(fmt.Sprintf("%5d | %s { /* ... */ }\n", lineNum, selector))
			if delta := braceDelta(trimmed); delta > 0 {
				inRule = true
				ruleDepth = delta
			}
			continue
		}

		// Preserve structural closing braces for nested at-rules.
		if trimmed == "}" {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
		}
	}

	return out.String()
}

func isVariableDeclaration(trimmed string) bool {
	if strings.HasPrefix(trimmed, "$") {
		return strings.Contains(trimmed, ":")
	}
	if strings.HasPrefix(trimmed, "@") {
		lower := strings.ToLower(trimmed)
		for _, keyword := range []string{"@media", "@supports", "@keyframes", "@-webkit-keyframes", "@font-face", "@import", "@charset", "@use", "@forward", "@namespace", "@layer", "@container", "@page", "@document"} {
			if strings.HasPrefix(lower, keyword) {
				return false
			}
		}
		return strings.Contains(trimmed, ":")
	}
	return false
}

func isCollapsibleAtRule(trimmed string) bool {
	lower := strings.ToLower(trimmed)
	for _, keyword := range []string{"@font-face", "@page", "@counter-style", "@property"} {
		if strings.HasPrefix(lower, keyword) {
			return true
		}
	}
	return false
}

// braceDelta counts net brace depth while ignoring quoted strings.
func braceDelta(line string) int {
	delta := 0
	var quote byte

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		if ch == '/' && i+1 < len(line) && line[i+1] == '*' {
			end := strings.Index(line[i+2:], "*/")
			if end == -1 {
				break
			}
			i += 2 + end + 1
			continue
		}

		switch ch {
		case '\'', '"':
			quote = ch
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}
