package scanner

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"
)

// IsDocFile reports whether a file path points to a documentation file.
func IsDocFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".md", ".markdown", ".mdown", ".mkdn", ".rst", ".txt", ".adoc":
		return true
	}
	base := strings.ToLower(filepath.Base(filePath))
	return base == "readme" || base == "license" || base == "changelog"
}

// ExtractSignatures takes file contents and strips function/method bodies,
// preserving structs, interfaces, types, and function signatures with their
// original 1-based source line numbers.
// For documentation files (e.g. .md), the full content is preserved.
func ExtractSignatures(filePath string, content string) string {
	if IsDocFile(filePath) {
		lines := strings.Split(content, "\n")
		var out strings.Builder
		for idx, line := range lines {
			out.WriteString(fmt.Sprintf("%5d | %s\n", idx+1, line))
		}
		return out.String()
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	return extractGenericSignatures(content, ext)
}

func extractGenericSignatures(content string, ext string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var out strings.Builder

	inBlock := false
	braceDepth := 0
	isPython := ext == ".py"
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Always keep package declarations and imports for context
		if strings.HasPrefix(trimmed, "package ") || strings.HasPrefix(trimmed, "import ") {
			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			continue
		}

		if isPython {
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "from ") {
				out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
				if strings.HasSuffix(trimmed, ":") {
					indent := getLeadingWhitespace(line)
					out.WriteString(fmt.Sprintf("%5d | %s    ...\n", lineNum, indent))
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
				// If opening a function body on the same line, truncate implementation
				if strings.HasPrefix(trimmed, "func ") ||
					strings.HasPrefix(trimmed, "fn ") ||
					strings.HasPrefix(trimmed, "function ") ||
					strings.HasPrefix(trimmed, "pub fn ") ||
					strings.HasPrefix(trimmed, "export function ") ||
					strings.HasPrefix(trimmed, "export default function ") {
					out.WriteString(fmt.Sprintf("%5d | %s /* ... */ }\n", lineNum, line[:braceIdx+1]))
					inBlock = false
					continue
				}
			}

			out.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))

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
