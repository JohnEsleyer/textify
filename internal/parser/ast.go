package parser

import (
	"fmt"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

// Re-export target types for seamless backward compatibility. Parser consumers
// continue to use parser.Symbol and parser.KindFunction while the canonical
// definitions live in internal/targets.
type SymbolKind = targets.SymbolKind
type Symbol = targets.Symbol

const (
	KindFunction  = targets.KindFunction
	KindMethod    = targets.KindMethod
	KindStruct    = targets.KindStruct
	KindClass     = targets.KindClass
	KindInterface = targets.KindInterface
	KindType      = targets.KindType
	KindComponent = targets.KindComponent
	KindHook      = targets.KindHook
)

// Language represents supported parsing languages.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangJava       Language = "java"
	LangKotlin     Language = "kotlin"
	LangJVM        Language = "jvm"
	LangRust       Language = "rust"
	LangAll        Language = "all"
)

// ParseLanguages parses a comma-separated list like "go,ts", "rust", "java", "kotlin", "jvm", or "all".
func ParseLanguages(langStr string) map[Language]bool {
	langs := make(map[Language]bool)
	if langStr == "" || strings.EqualFold(strings.TrimSpace(langStr), "all") {
		langs[LangGo] = true
		langs[LangTypeScript] = true
		langs[LangJava] = true
		langs[LangKotlin] = true
		langs[LangRust] = true
		return langs
	}
	for _, l := range strings.Split(langStr, ",") {
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "go", "golang":
			langs[LangGo] = true
		case "ts", "tsx", "typescript", "js", "jsx", "javascript":
			langs[LangTypeScript] = true
		case "java":
			langs[LangJava] = true
		case "kt", "kts", "kotlin":
			langs[LangKotlin] = true
		case "jvm":
			langs[LangJava] = true
			langs[LangKotlin] = true
		case "rust", "rs":
			langs[LangRust] = true
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

// XRefResult describes a cross-reference site.
type XRefResult struct {
	File     string
	Line     int
	Action   string // "write", "read", "call", "all"
	LineText string
	Context  string
}

// DeepTraceNode represents a symbol node in a deep contextual trace.
type DeepTraceNode struct {
	Symbol Symbol
	Depth  int
}

// DeepTraceResult contains the structured deep execution flow trace.
type DeepTraceResult struct {
	RootSymbol Symbol
	CallGraph  []string
	Callees    []DeepTraceNode
	Types      []Symbol
}

// Render renders the deep trace result into formatted code blocks.
func (d *DeepTraceResult) Render(forPatch bool) string {
	var sb strings.Builder
	sb.WriteString("================================================================================\n")
	sb.WriteString(fmt.Sprintf("Deep Contextual Trace: %s %q (%s:%d-%d)\n", d.RootSymbol.Kind, d.RootSymbol.FullName(), d.RootSymbol.File, d.RootSymbol.StartLine, d.RootSymbol.EndLine))
	sb.WriteString("================================================================================\n\n")

	if len(d.CallGraph) > 0 {
		sb.WriteString("### Execution Flow Call Graph:\n")
		sb.WriteString(strings.Join(d.CallGraph, " -> "))
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("### Primary Definition: %s %s [%s:%d-%d]\n", d.RootSymbol.Kind, d.RootSymbol.FullName(), d.RootSymbol.File, d.RootSymbol.StartLine, d.RootSymbol.EndLine))
	if forPatch {
		sb.WriteString(d.RootSymbol.Body)
		sb.WriteString("\n\n")
	} else {
		for i, line := range strings.Split(d.RootSymbol.Body, "\n") {
			sb.WriteString(fmt.Sprintf("%5d | %s\n", d.RootSymbol.StartLine+i, line))
		}
		sb.WriteString("\n")
	}

	if len(d.Callees) > 0 {
		sb.WriteString("### Implementation Bodies of Callees:\n\n")
		for _, callee := range d.Callees {
			s := callee.Symbol
			sb.WriteString(fmt.Sprintf("--- Callee: %s %s [%s:%d-%d] (depth %d) ---\n", s.Kind, s.FullName(), s.File, s.StartLine, s.EndLine, callee.Depth))
			if forPatch {
				sb.WriteString(s.Body)
				sb.WriteString("\n\n")
			} else {
				for i, line := range strings.Split(s.Body, "\n") {
					sb.WriteString(fmt.Sprintf("%5d | %s\n", s.StartLine+i, line))
				}
				sb.WriteString("\n")
			}
		}
	}

	if len(d.Types) > 0 {
		sb.WriteString("### Referenced Struct / Interface Definitions:\n\n")
		for _, t := range d.Types {
			sb.WriteString(fmt.Sprintf("--- Type: %s %s [%s:%d-%d] ---\n", t.Kind, t.FullName(), t.File, t.StartLine, t.EndLine))
			if forPatch {
				sb.WriteString(t.Body)
				sb.WriteString("\n\n")
			} else {
				for i, line := range strings.Split(t.Body, "\n") {
					sb.WriteString(fmt.Sprintf("%5d | %s\n", t.StartLine+i, line))
				}
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}
