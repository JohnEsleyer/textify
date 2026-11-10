package targets

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
	KindComponent SymbolKind = "component" // React functional or class component
	KindHook      SymbolKind = "hook"      // React custom hook (use...)
)

// Symbol represents an AST-parsed entity (function, struct, component, method, etc.).
type Symbol struct {
	Name       string     `json:"name"`
	Receiver   string     `json:"receiver,omitempty"` // Receiver for methods / components
	Kind       SymbolKind `json:"kind"`
	File       string     `json:"file"`
	StartLine  int        `json:"start_line"`
	EndLine    int        `json:"end_line"`
	Signature  string     `json:"signature"`
	Body       string     `json:"body,omitempty"`
	Calls      []string   `json:"calls,omitempty"`      // Identifiers called inside this entity
	References []string   `json:"references,omitempty"` // Types/props referenced
}

// FullName returns the qualified name (e.g., (Receiver).Method or Function).
func (s Symbol) FullName() string {
	if s.Receiver != "" {
		return fmt.Sprintf("(%s).%s", s.Receiver, s.Name)
	}
	return s.Name
}

// IsTypeKind reports whether the symbol is a type-like declaration.
func (s Symbol) IsTypeKind() bool {
	switch s.Kind {
	case KindStruct, KindClass, KindInterface, KindType:
		return true
	}
	return false
}

// SliceLines extracts 1-based lines from start to end inclusive.
func SliceLines(lines []string, start, end int) string {
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}
