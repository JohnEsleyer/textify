package html

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

func init() {
	targets.RegisterTarget(&HTMLTarget{})
}

// HTMLTarget implements processing for HTML and web template files.
type HTMLTarget struct{}

func (h *HTMLTarget) Name() string {
	return "html"
}

func (h *HTMLTarget) Match(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".html", ".htm", ".svg", ".vue", ".svelte":
		return true
	}
	return false
}

func (h *HTMLTarget) ExtractSignatures(filePath string, content string) string {
	return ExtractSignatures(content)
}

func (h *HTMLTarget) Parse(filePath string, src []byte) ([]targets.Symbol, error) {
	return nil, nil
}

var (
	htmlTagRegex    = regexp.MustCompile(`(?i)<\s*(/?)\s*([a-zA-Z0-9_\-]+)([^>]*)>`)
	scriptOpenRegex = regexp.MustCompile(`(?i)<\s*script\b([^>]*)>`)
	scriptClose     = regexp.MustCompile(`(?i)</\s*script\s*>`)
	styleOpenRegex  = regexp.MustCompile(`(?i)<\s*style\b([^>]*)>`)
	styleClose      = regexp.MustCompile(`(?i)</\s*style\s*>`)
)

// ExtractSignatures compresses HTML to a structural DOM wireframe containing
// tags, IDs, classes, and component tags while truncating repetitive text
// nodes and collapsing inline <script>/<style> blocks. Line numbers remain
// anchored to the original 1-based source lines.
func ExtractSignatures(content string) string {
	lines := strings.Split(content, "\n")
	var out strings.Builder

	inScript := false
	inStyle := false

	for idx, line := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if inScript {
			if scriptClose.MatchString(line) {
				out.WriteString(fmt.Sprintf("%5d |   /* script content omitted */\n", lineNum))
				out.WriteString(fmt.Sprintf("%5d | </script>\n", lineNum))
				inScript = false
			}
			continue
		}

		if inStyle {
			if styleClose.MatchString(line) {
				out.WriteString(fmt.Sprintf("%5d |   /* styles omitted */\n", lineNum))
				out.WriteString(fmt.Sprintf("%5d | </style>\n", lineNum))
				inStyle = false
			}
			continue
		}

		if m := scriptOpenRegex.FindStringSubmatch(line); len(m) > 0 {
			attrs := strings.TrimSpace(m[1])
			if scriptClose.MatchString(line) {
				out.WriteString(fmt.Sprintf("%5d | %s/* ... */</script>\n", lineNum, openTag("script", attrs)))
			} else {
				out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, openTag("script", attrs)))
				inScript = true
			}
			continue
		}

		if m := styleOpenRegex.FindStringSubmatch(line); len(m) > 0 {
			attrs := strings.TrimSpace(m[1])
			if styleClose.MatchString(line) {
				out.WriteString(fmt.Sprintf("%5d | %s/* ... */</style>\n", lineNum, openTag("style", attrs)))
			} else {
				out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, openTag("style", attrs)))
				inStyle = true
			}
			continue
		}

		if normalized, ok := normalizeMarkupLine(trimmed); ok {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, normalized))
		}
	}

	return out.String()
}

func openTag(name, attrs string) string {
	if attrs == "" {
		return "<" + name + ">"
	}
	return "<" + name + " " + attrs + ">"
}

// normalizeMarkupLine keeps the document's structural markup and strips or
// truncates long text nodes so the result reads like a wireframe.
func normalizeMarkupLine(line string) (string, bool) {
	upper := strings.ToUpper(line)
	if strings.HasPrefix(upper, "<!DOCTYPE") || strings.HasPrefix(line, "<!--") {
		return line, true
	}

	matches := htmlTagRegex.FindAllString(line, -1)
	if len(matches) == 0 {
		// A plain text node: keep only a short preview for context.
		if len(line) > 80 {
			line = line[:77] + "..."
		}
		return line, true
	}

	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		if m = strings.TrimSpace(m); m != "" {
			parts = append(parts, m)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}
