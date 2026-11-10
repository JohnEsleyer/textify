package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets"
)

func init() {
	targets.RegisterTarget(&GoTarget{})
}

// GoTarget implements processing for Go source files.
type GoTarget struct{}

func (g *GoTarget) Name() string {
	return "golang"
}

func (g *GoTarget) Match(filePath string) bool {
	return strings.ToLower(filepath.Ext(filePath)) == ".go"
}

func (g *GoTarget) ExtractSignatures(filePath string, content string) string {
	return ExtractSignatures(content)
}

func (g *GoTarget) Parse(filePath string, src []byte) ([]targets.Symbol, error) {
	return ParseGoFile(filePath, src)
}

// ParseGoFile parses a Go source file using the standard go/ast and go/parser.
func ParseGoFile(filePath string, src []byte) ([]targets.Symbol, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(src), "\n")
	var symbols []targets.Symbol

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := parseGoFunc(fset, d, filePath, lines)
			symbols = append(symbols, sym)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					sym := parseGoType(fset, d, typeSpec, filePath, lines)
					symbols = append(symbols, sym)
				}
			}
		}
	}

	return symbols, nil
}

func parseGoFunc(fset *token.FileSet, fn *ast.FuncDecl, filePath string, lines []string) targets.Symbol {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	receiver := ""
	kind := targets.KindFunction
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = targets.KindMethod
		var buf bytes.Buffer
		_ = printer.Fprint(&buf, fset, fn.Recv.List[0].Type)
		receiver = strings.TrimSpace(buf.String())
	}

	var sigBuf bytes.Buffer
	origBody := fn.Body
	fn.Body = nil // Temporarily clear body to print signature only.
	_ = printer.Fprint(&sigBuf, fset, fn)
	fn.Body = origBody
	sig := strings.Join(strings.Fields(sigBuf.String()), " ")

	var calls []string
	callSet := make(map[string]bool)
	var refs []string
	refSet := make(map[string]bool)

	if fn.Type != nil {
		if fn.Type.Params != nil {
			collectGoFieldListRefs(fn.Type.Params, refSet, &refs)
		}
		if fn.Type.Results != nil {
			collectGoFieldListRefs(fn.Type.Results, refSet, &refs)
		}
	}

	if fn.Body != nil {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				callee := exprToString(call.Fun)
				if callee != "" && !isGoBuiltin(callee) && !callSet[callee] {
					callSet[callee] = true
					calls = append(calls, callee)
				}
			}
			return true
		})
	}

	body := targets.SliceLines(lines, startPos.Line, endPos.Line)

	return targets.Symbol{
		Name:       fn.Name.Name,
		Receiver:   receiver,
		Kind:       kind,
		File:       filePath,
		StartLine:  startPos.Line,
		EndLine:    endPos.Line,
		Signature:  sig,
		Body:       body,
		Calls:      calls,
		References: refs,
	}
}

func parseGoType(fset *token.FileSet, genDecl *ast.GenDecl, typeSpec *ast.TypeSpec, filePath string, lines []string) targets.Symbol {
	start := genDecl.Pos()
	if len(genDecl.Specs) > 1 {
		start = typeSpec.Pos()
	}
	startPos := fset.Position(start)
	endPos := fset.Position(typeSpec.End())

	kind := targets.KindType
	var refs []string
	refSet := make(map[string]bool)

	switch t := typeSpec.Type.(type) {
	case *ast.StructType:
		kind = targets.KindStruct
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				collectGoTypeRefs(field.Type, refSet, &refs)
			}
		}
	case *ast.InterfaceType:
		kind = targets.KindInterface
		if t.Methods != nil {
			for _, field := range t.Methods.List {
				collectGoTypeRefs(field.Type, refSet, &refs)
			}
		}
	default:
		collectGoTypeRefs(typeSpec.Type, refSet, &refs)
	}

	sig := "type " + typeSpec.Name.Name
	switch typeSpec.Type.(type) {
	case *ast.StructType:
		sig += " struct"
	case *ast.InterfaceType:
		sig += " interface"
	default:
		var buf bytes.Buffer
		_ = printer.Fprint(&buf, fset, typeSpec.Type)
		rendered := strings.TrimSpace(buf.String())
		if idx := strings.IndexByte(rendered, '\n'); idx >= 0 {
			rendered = rendered[:idx] + " ..."
		}
		if rendered != "" {
			sig += " " + rendered
		}
	}

	body := targets.SliceLines(lines, startPos.Line, endPos.Line)

	return targets.Symbol{
		Name:       typeSpec.Name.Name,
		Kind:       kind,
		File:       filePath,
		StartLine:  startPos.Line,
		EndLine:    endPos.Line,
		Signature:  sig,
		Body:       body,
		References: refs,
	}
}

func collectGoTypeRefs(expr ast.Expr, seen map[string]bool, refs *[]string) {
	switch e := expr.(type) {
	case *ast.Ident:
		addGoTypeRef(e.Name, seen, refs)
	case *ast.SelectorExpr:
		addGoTypeRef(exprToString(e), seen, refs)
	case *ast.StarExpr:
		collectGoTypeRefs(e.X, seen, refs)
	case *ast.ArrayType:
		collectGoTypeRefs(e.Elt, seen, refs)
	case *ast.MapType:
		collectGoTypeRefs(e.Key, seen, refs)
		collectGoTypeRefs(e.Value, seen, refs)
	case *ast.ChanType:
		collectGoTypeRefs(e.Value, seen, refs)
	case *ast.Ellipsis:
		collectGoTypeRefs(e.Elt, seen, refs)
	case *ast.FuncType:
		collectGoFieldListRefs(e.Params, seen, refs)
		collectGoFieldListRefs(e.Results, seen, refs)
	case *ast.InterfaceType:
		if e.Methods != nil {
			for _, field := range e.Methods.List {
				collectGoTypeRefs(field.Type, seen, refs)
			}
		}
	case *ast.StructType:
		if e.Fields != nil {
			for _, field := range e.Fields.List {
				collectGoTypeRefs(field.Type, seen, refs)
			}
		}
	}
}

func collectGoFieldListRefs(list *ast.FieldList, seen map[string]bool, refs *[]string) {
	if list == nil {
		return
	}
	for _, field := range list.List {
		collectGoTypeRefs(field.Type, seen, refs)
	}
}

func addGoTypeRef(name string, seen map[string]bool, refs *[]string) {
	name = strings.TrimSpace(name)
	if name == "" || isGoBuiltin(name) || seen[name] {
		return
	}
	seen[name] = true
	*refs = append(*refs, name)
}

func isGoBuiltin(name string) bool {
	switch name {
	case "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string", "uint",
		"uint8", "uint16", "uint32", "uint64", "uintptr", "any",
		"append", "cap", "clear", "close", "complex", "copy", "delete",
		"imag", "len", "make", "max", "min", "new", "panic", "print",
		"println", "real", "recover":
		return true
	}
	return false
}

func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		x := exprToString(e.X)
		if x != "" {
			return x + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.StarExpr:
		return exprToString(e.X)
	case *ast.IndexExpr:
		return exprToString(e.X)
	case *ast.IndexListExpr:
		return exprToString(e.X)
	case *ast.ParenExpr:
		return exprToString(e.X)
	}
	return ""
}

type signatureChunk struct {
	startLine int
	text      string
}

// ExtractSignatures produces clean Go signatures with original 1-based line
// numbers. Type, import, const, and var declarations keep their full bodies
// (which carry useful type information); function and method bodies are
// collapsed into a one-line stub.
func ExtractSignatures(content string) string {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", []byte(content), parser.ParseComments)
	if err != nil {
		// Keep the scanner useful on incomplete or generated files.
		return targets.ExtractGenericSignatures(content, ".go")
	}

	lines := strings.Split(content, "\n")
	var chunks []signatureChunk

	// Preserve the package clause and any leading comments before the first decl.
	firstDeclLine := len(lines) + 1
	if len(node.Decls) > 0 {
		firstDeclLine = fset.Position(node.Decls[0].Pos()).Line
	}
	for i := 0; i < firstDeclLine-1 && i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			chunks = append(chunks, signatureChunk{startLine: i + 1, text: lines[i]})
		}
	}

	for _, decl := range node.Decls {
		startLine := fset.Position(decl.Pos()).Line
		endLine := fset.Position(decl.End()).Line

		switch d := decl.(type) {
		case *ast.FuncDecl:
			var buf bytes.Buffer
			origBody := d.Body
			d.Body = nil
			_ = printer.Fprint(&buf, fset, d)
			d.Body = origBody

			sig := strings.Join(strings.Fields(buf.String()), " ")
			if d.Body != nil {
				sig += " { /* ... */ }"
			}
			chunks = append(chunks, signatureChunk{startLine: startLine, text: sig})

		case *ast.GenDecl:
			chunks = append(chunks, signatureChunk{
				startLine: startLine,
				text:      targets.SliceLines(lines, startLine, endLine),
			})
		}
	}

	sort.SliceStable(chunks, func(i, j int) bool {
		return chunks[i].startLine < chunks[j].startLine
	})

	var out strings.Builder
	for _, chunk := range chunks {
		textLines := strings.Split(chunk.text, "\n")
		for i, line := range textLines {
			out.WriteString(fmt.Sprintf("%5d | %s\n", chunk.startLine+i, line))
		}
	}
	return out.String()
}

// CleanCaseTarget strips boilerplate from switch-case queries.
func CleanCaseTarget(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "case ")
	s = strings.TrimSuffix(s, ":")
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "*")
	// String and rune literals print with quotes through go/printer; normalize
	// them so both `case="add"` and `case='"add"'` match the same clause.
	s = strings.Trim(s, "\"'`")
	return strings.TrimSpace(s)
}

// ExtractGoCase inspects the Go AST to slice a specific CaseClause.
func ExtractGoCase(filePath, content, fnName, caseTarget string) (string, int, int, error) {
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, filePath, []byte(content), parser.ParseComments)
	if err != nil {
		return "", 0, 0, fmt.Errorf("parse Go AST: %w", err)
	}

	cleanTarget := CleanCaseTarget(caseTarget)
	cleanFn := strings.TrimSpace(fnName)
	cleanFn = strings.TrimPrefix(cleanFn, "*")

	lines := strings.Split(content, "\n")
	type caseMatch struct {
		startLine int
		endLine   int
	}
	var found *caseMatch

	for _, decl := range fileNode.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if cleanFn != "" {
			receiver := ""
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				var buf bytes.Buffer
				_ = printer.Fprint(&buf, fset, fn.Recv.List[0].Type)
				receiver = strings.TrimSpace(buf.String())
			}
			fullName := fn.Name.Name
			if receiver != "" {
				fullName = strings.TrimPrefix(receiver, "*") + "." + fn.Name.Name
			}
			if fn.Name.Name != cleanFn && fullName != cleanFn && !strings.HasSuffix(fullName, "."+cleanFn) {
				continue
			}
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if found != nil {
				return false
			}

			var bodyList []ast.Stmt
			var switchEnd token.Pos

			switch sw := n.(type) {
			case *ast.SwitchStmt:
				if sw.Body != nil {
					bodyList = sw.Body.List
					switchEnd = sw.Body.End()
				}
			case *ast.TypeSwitchStmt:
				if sw.Body != nil {
					bodyList = sw.Body.List
					switchEnd = sw.Body.End()
				}
			}

			if len(bodyList) == 0 {
				return true
			}

			for idx, stmt := range bodyList {
				cc, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}

				matched := false
				if cleanTarget == "default" && len(cc.List) == 0 {
					matched = true
				} else {
					for _, expr := range cc.List {
						var buf bytes.Buffer
						_ = printer.Fprint(&buf, fset, expr)
						exprStr := CleanCaseTarget(buf.String())
						if exprStr == cleanTarget || strings.HasSuffix(exprStr, "."+cleanTarget) || strings.EqualFold(exprStr, cleanTarget) {
							matched = true
							break
						}
					}
				}

				if matched {
					sLine := fset.Position(cc.Pos()).Line
					eLine := 0
					if idx+1 < len(bodyList) {
						eLine = fset.Position(bodyList[idx+1].Pos()).Line - 1
					} else {
						eLine = fset.Position(switchEnd).Line - 1
					}
					if eLine < sLine {
						eLine = sLine
					}

					found = &caseMatch{
						startLine: sLine,
						endLine:   eLine,
					}
					return false
				}
			}

			return true
		})

		if found != nil {
			break
		}
	}

	if found == nil {
		return "", 0, 0, fmt.Errorf("case target %q not found in AST of %s", caseTarget, filePath)
	}

	body := targets.SliceLines(lines, found.startLine, found.endLine)
	return body, found.startLine, found.endLine, nil
}

// ExtractGenericCase is a line-based regex fallback for case clause extraction.
func ExtractGenericCase(content, fnName, caseTarget string) (string, int, int, error) {
	lines := strings.Split(content, "\n")
	target := CleanCaseTarget(caseTarget)

	casePattern := regexp.MustCompile(`(?i)^\s*(?:case\s+.*` + regexp.QuoteMeta(target) + `.*:|default\s*:)`)
	matchIdx := -1

	for i, l := range lines {
		if casePattern.MatchString(l) {
			matchIdx = i
			break
		}
	}

	if matchIdx == -1 {
		return "", 0, 0, fmt.Errorf("case target %q not found", caseTarget)
	}

	endIdx := len(lines)
	for i := matchIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "case ") || strings.HasPrefix(trimmed, "default:") || trimmed == "}" {
			endIdx = i
			break
		}
	}

	body := targets.SliceLines(lines, matchIdx+1, endIdx)
	return body, matchIdx + 1, endIdx, nil
}
