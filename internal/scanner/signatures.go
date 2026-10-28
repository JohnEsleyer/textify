package scanner

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strings"
)

// ExtractSignatures takes file contents and strips function/method bodies.
func ExtractSignatures(filePath string, content string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	if ext == ".go" {
		if res, err := extractGoSignatures(filePath, content); err == nil {
			return res
		}
	}

	return extractGenericSignatures(content, ext)
}

func extractGoSignatures(filename string, content string) (string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, content, parser.ParseComments)
	if err != nil {
		return "", err
	}

	// Remove function bodies
	ast.Inspect(node, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil {
				fn.Body.List = nil // Remove statements inside function
			}
		}
		return true
	})

	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&buf, fset, node); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func extractGenericSignatures(content string, ext string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var out strings.Builder

	inBlock := false
	braceDepth := 0
	isPython := ext == ".py"

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if isPython {
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "import ") ||
				strings.HasPrefix(trimmed, "from ") {
				out.WriteString(line + "\n")
				if strings.HasSuffix(trimmed, ":") {
					indent := getLeadingWhitespace(line)
					out.WriteString(indent + "    ...\n")
				}
			}
			continue
		}

		isSig := strings.HasPrefix(trimmed, "func ") ||
			strings.HasPrefix(trimmed, "fn ") ||
			strings.HasPrefix(trimmed, "function ") ||
			strings.HasPrefix(trimmed, "pub fn ") ||
			strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "struct ") ||
			strings.HasPrefix(trimmed, "interface ") ||
			strings.HasPrefix(trimmed, "class ") ||
			strings.HasPrefix(trimmed, "export function ") ||
			strings.HasPrefix(trimmed, "export default function ") ||
			strings.HasPrefix(trimmed, "export class ") ||
			strings.HasPrefix(trimmed, "export interface ") ||
			strings.HasPrefix(trimmed, "export type ")

		if isSig {
			inBlock = true
			braceDepth = 0
		}

		if inBlock {
			if braceDepth == 0 && strings.Contains(trimmed, "{") {
				braceIdx := strings.Index(line, "{")
				out.WriteString(line[:braceIdx+1] + " /* ... */ }\n")
				inBlock = false
				continue
			}

			out.WriteString(line + "\n")

			for _, c := range trimmed {
				if c == '{' {
					braceDepth++
				} else if c == '}' {
					braceDepth--
				}
			}

			if braceDepth <= 0 && (strings.Contains(trimmed, "}") || strings.HasSuffix(trimmed, ";")) {
				inBlock = false
			}
		}
	}

	return out.String()
}

func getLeadingWhitespace(s string) string {
	for i, ch := range s {
		if ch != ' ' && ch != '\t' {
			return s[:i]
		}
	}
	return s
}
