package parser

import (
	"fmt"
	"strings"
)

// SymbolKind indicates the category of a parsed syntax element.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindStruct    SymbolKind = "struct"
	KindClass     SymbolKind = "class"
	KindInterface SymbolKind = "interface"
	KindType      SymbolKind = "type"
)

// Symbol represents an AST-parsed entity (function, struct, method, etc.).
type Symbol struct {
	Name       string     `json:"name"`
	Receiver   string     `json:"receiver,omitempty"` // For Go methods or class methods
	Kind       SymbolKind `json:"kind"`
	File       string     `json:"file"`
	StartLine  int        `json:"start_line"`
	EndLine    int        `json:"end_line"`
	Signature  string     `json:"signature"`
	Body       string     `json:"body,omitempty"`
	Calls      []string   `json:"calls,omitempty"`      // Identifiers called inside this function
	References []string   `json:"references,omitempty"` // Types/structs referenced
}

// FullName returns the qualified name (e.g., (Receiver).Method or Function).
func (s Symbol) FullName() string {
	if s.Receiver != "" {
		return fmt.Sprintf("(%s).%s", s.Receiver, s.Name)
	}
	return s.Name
}

// IsTypeKind reports whether the symbol is a type-like declaration whose
// dependencies can be traced instead of an execution flow.
func (s Symbol) IsTypeKind() bool {
	switch s.Kind {
	case KindStruct, KindClass, KindInterface:
		return true
	}
	return false
}

// Language represents supported parsing languages.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangAll        Language = "all"
)

// ParseLanguages parses a comma-separated list like "go,ts" or "typescript".
func ParseLanguages(langStr string) map[Language]bool {
	langs := make(map[Language]bool)
	if langStr == "" || strings.EqualFold(strings.TrimSpace(langStr), "all") {
		langs[LangGo] = true
		langs[LangTypeScript] = true
		return langs
	}
	for _, l := range strings.Split(langStr, ",") {
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "go", "golang":
			langs[LangGo] = true
		case "ts", "tsx", "typescript", "js", "jsx", "javascript":
			langs[LangTypeScript] = true
		}
	}
	return langs
}

// cleanName normalizes identifiers such as "*Service", "(*Service)" and
// "Service.Method()" so user-provided symbol names match the index.
func cleanName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, "()")
	name = strings.Trim(name, "()")
	name = strings.TrimPrefix(name, "*")
	return strings.TrimSpace(name)
}
