package parser

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// ParseGoFile parses a Go source file using the standard go/ast and go/parser.
func ParseGoFile(filePath string, src []byte) ([]Symbol, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(src), "\n")
	var symbols []Symbol

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

func parseGoFunc(fset *token.FileSet, fn *ast.FuncDecl, filePath string, lines []string) Symbol {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	receiver := ""
	kind := KindFunction
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = KindMethod
		var buf bytes.Buffer
		_ = printer.Fprint(&buf, fset, fn.Recv.List[0].Type)
		receiver = strings.TrimSpace(buf.String())
	}

	var sigBuf bytes.Buffer
	origBody := fn.Body
	fn.Body = nil // Temporarily clear body to print signature only
	_ = printer.Fprint(&sigBuf, fset, fn)
	fn.Body = origBody
	sig := strings.TrimSpace(sigBuf.String())

	// Collect calls made inside this function.
	var calls []string
	callSet := make(map[string]bool)
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

	body := sliceLines(lines, startPos.Line, endPos.Line)

	return Symbol{
		Name:      fn.Name.Name,
		Receiver:  receiver,
		Kind:      kind,
		File:      filePath,
		StartLine: startPos.Line,
		EndLine:   endPos.Line,
		Signature: sig,
		Body:      body,
		Calls:     calls,
	}
}

func parseGoType(fset *token.FileSet, genDecl *ast.GenDecl, typeSpec *ast.TypeSpec, filePath string, lines []string) Symbol {
	// In a grouped type declaration ("type ( A struct{}; B struct{} )")
	// use the individual spec bounds instead of the whole group.
	start := genDecl.Pos()
	if len(genDecl.Specs) > 1 {
		start = typeSpec.Pos()
	}
	startPos := fset.Position(start)
	endPos := fset.Position(typeSpec.End())

	kind := KindType
	var refs []string
	refSet := make(map[string]bool)

	switch t := typeSpec.Type.(type) {
	case *ast.StructType:
		kind = KindStruct
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				collectGoTypeRefs(field.Type, refSet, &refs)
			}
		}
	case *ast.InterfaceType:
		kind = KindInterface
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

	body := sliceLines(lines, startPos.Line, endPos.Line)

	return Symbol{
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

// collectGoTypeRefs walks a Go type expression and records user types it
// depends on. Built-in types are intentionally skipped so dependency traces
// stay readable.
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

func sliceLines(lines []string, start, end int) string {
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
