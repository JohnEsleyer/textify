package scanner

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
	_ "github.com/JohnEsleyer/textify/internal/targets/css"
	_ "github.com/JohnEsleyer/textify/internal/targets/golang"
	_ "github.com/JohnEsleyer/textify/internal/targets/html"
	_ "github.com/JohnEsleyer/textify/internal/targets/jvm"
	_ "github.com/JohnEsleyer/textify/internal/targets/react"
	_ "github.com/JohnEsleyer/textify/internal/targets/rust"
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

// ExtractSignatures delegates to the specialized target package (React, Go,
// HTML, CSS) and preserves full documentation files. The target registry falls
// back to the generic line scanner for unsupported file types, maintaining
// backward compatibility.
func ExtractSignatures(filePath string, content string) string {
	if IsDocFile(filePath) {
		lines := strings.Split(content, "\n")
		var out strings.Builder
		for idx, line := range lines {
			out.WriteString(fmt.Sprintf("%5d | %s\n", idx+1, line))
		}
		return out.String()
	}
	return targets.ExtractSignatures(filePath, content)
}
