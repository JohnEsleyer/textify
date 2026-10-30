package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CodebaseIndex holds symbols mapped by name, file, and kind.
type CodebaseIndex struct {
	Symbols     []Symbol
	ByName      map[string][]Symbol
	ByFile      map[string][]Symbol
	LoadedFiles int
}

// BuildIndex scans a root directory, filters files by language, and builds the AST index.
func BuildIndex(root string, langs map[Language]bool) (*CodebaseIndex, error) {
	idx := &CodebaseIndex{
		ByName: make(map[string][]Symbol),
		ByFile: make(map[string][]Symbol),
	}

	skipDirs := map[string]bool{
		".git": true, ".hg": true, ".svn": true,
		"node_modules": true, "vendor": true,
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if path != root && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		base := strings.ToLower(info.Name())
		if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.ts") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		var fileSymbols []Symbol

		if ext == ".go" && langs[LangGo] {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				syms, parseErr := ParseGoFile(relPath, data)
				if parseErr == nil {
					fileSymbols = append(fileSymbols, syms...)
				}
			}
		} else if (ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs") && langs[LangTypeScript] {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				syms, parseErr := ParseTypeScriptFile(relPath, data)
				if parseErr == nil {
					fileSymbols = append(fileSymbols, syms...)
				}
			}
		}

		if len(fileSymbols) > 0 {
			idx.LoadedFiles++
			for _, sym := range fileSymbols {
				idx.Symbols = append(idx.Symbols, sym)
				idx.ByName[sym.Name] = append(idx.ByName[sym.Name], sym)
				idx.ByFile[sym.File] = append(idx.ByFile[sym.File], sym)
			}
		}

		return nil
	})

	return idx, err
}

// FindDefinition finds all matching symbols by identifier and optional file restriction.
// It understands qualified names such as "Service.Method", "(*Service).Method",
// "pkg.Type.Method" and symbols written with a trailing "()".
func (idx *CodebaseIndex) FindDefinition(symbolName, filePath string) []Symbol {
	target := cleanName(symbolName)
	if target == "" {
		return nil
	}

	var candidates []Symbol

	// Qualified method lookup: Receiver.Method.
	if strings.Contains(target, ".") {
		parts := strings.Split(target, ".")
		methodName := parts[len(parts)-1]
		receiverName := cleanName(strings.Join(parts[:len(parts)-1], "."))
		for _, sym := range idx.ByName[methodName] {
			if receiverMatches(sym.Receiver, receiverName) {
				candidates = append(candidates, sym)
			}
		}
	}

	// Exact symbol lookup, with a case-insensitive fallback.
	if len(candidates) == 0 {
		candidates = append(candidates, idx.ByName[target]...)
	}
	if len(candidates) == 0 {
		for name, syms := range idx.ByName {
			if strings.EqualFold(name, target) {
				candidates = append(candidates, syms...)
			}
		}
	}

	seen := make(map[string]bool)
	var matches []Symbol
	for _, sym := range candidates {
		if !fileMatches(sym.File, filePath) {
			continue
		}
		key := fmt.Sprintf("%s|%d|%s", sym.File, sym.StartLine, sym.FullName())
		if seen[key] {
			continue
		}
		seen[key] = true
		matches = append(matches, sym)
	}

	return matches
}

// TraceFlow builds an execution call flow starting from a root symbol:
// funcA() -> funcB() -> funcC(). For structs, classes and interfaces it traces
// attached methods and the type dependency hierarchy instead.
func (idx *CodebaseIndex) TraceFlow(rootSymbol string, maxDepth int) string {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	matches := idx.FindDefinition(rootSymbol, "")
	if len(matches) == 0 {
		return fmt.Sprintf("No symbol %q found in codebase.\n", rootSymbol)
	}

	root := matches[0]
	if root.IsTypeKind() {
		return idx.traceTypeFlow(root, maxDepth)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Execution Flow for %s %q (%s:%d-%d) ===\n\n", root.Kind, root.FullName(), root.File, root.StartLine, root.EndLine))

	visited := make(map[string]bool)
	externalSeen := make(map[string]bool)
	var chain []string
	nodes := 0

	var dfs func(current Symbol, depth int)
	dfs = func(current Symbol, depth int) {
		name := current.FullName()
		nodeKey := fmt.Sprintf("%s:%d", current.File, current.StartLine)
		if visited[nodeKey] {
			return
		}
		visited[nodeKey] = true
		nodes++
		chain = append(chain, fmt.Sprintf("%s() [%s:%d]", name, current.File, current.StartLine))

		if depth >= maxDepth || nodes >= 200 {
			return
		}

		for _, call := range current.Calls {
			callName := call
			if strings.HasPrefix(call, "this.") && current.Receiver != "" {
				callName = cleanReceiverName(current.Receiver) + "." + strings.TrimPrefix(call, "this.")
			}

			callees := idx.FindDefinition(callName, "")
			if len(callees) == 0 && current.Receiver != "" && strings.Contains(callName, ".") {
				methodName := callName[strings.LastIndex(callName, ".")+1:]
				callees = idx.FindDefinition(cleanReceiverName(current.Receiver)+"."+methodName, "")
			}
			if len(callees) > 0 {
				dfs(callees[0], depth+1)
			} else if !externalSeen[call] {
				externalSeen[call] = true
				chain = append(chain, fmt.Sprintf("%s() [external/std]", call))
			}
		}
	}

	dfs(root, 1)

	sb.WriteString(strings.Join(chain, " -> "))
	sb.WriteString("\n\n")
	return sb.String()
}

func (idx *CodebaseIndex) traceTypeFlow(root Symbol, maxDepth int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Dependency & Method Hierarchy for %s %q (%s:%d-%d) ===\n\n", root.Kind, root.Name, root.File, root.StartLine, root.EndLine))

	// 1. Methods attached to this type.
	var methods []Symbol
	for _, sym := range idx.Symbols {
		if sym.Kind == KindMethod && receiverMatches(sym.Receiver, root.Name) {
			methods = append(methods, sym)
		}
	}
	sort.Slice(methods, func(i, j int) bool {
		if methods[i].File == methods[j].File {
			return methods[i].StartLine < methods[j].StartLine
		}
		return methods[i].File < methods[j].File
	})
	if len(methods) > 0 {
		sb.WriteString("Attached Methods:\n")
		for _, m := range methods {
			sb.WriteString(fmt.Sprintf("  - %s() [%s:%d]\n", m.FullName(), m.File, m.StartLine))
		}
		sb.WriteString("\n")
	}

	// 2. Recursive type dependency flow.
	var chains []string
	seenChains := make(map[string]bool)

	addChain := func(parts []string) {
		value := strings.Join(parts, " -> ")
		if !seenChains[value] {
			seenChains[value] = true
			chains = append(chains, value)
		}
	}

	var walk func(sym Symbol, chain []string, depth int, seen map[string]bool)
	walk = func(sym Symbol, chain []string, depth int, seen map[string]bool) {
		chain = append(chain, sym.Name)

		if depth >= maxDepth || len(sym.References) == 0 {
			addChain(chain)
			return
		}

		seen[sym.Name] = true
		expanded := false

		for _, ref := range sym.References {
			refName := cleanRefName(ref)
			if refName == "" || seen[refName] {
				continue
			}
			expanded = true

			refs := idx.ByName[refName]
			if len(refs) == 0 {
				addChain(append(copyStrings(chain), refName))
				continue
			}
			walk(refs[0], copyStrings(chain), depth+1, copySeen(seen))
		}

		if !expanded {
			addChain(chain)
		}
	}

	walk(root, nil, 1, make(map[string]bool))

	if len(chains) > 0 {
		sb.WriteString("Type Dependency Flow:\n")
		for _, chain := range chains {
			sb.WriteString(fmt.Sprintf("  %s\n", chain))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// GenerateSignaturesOverview renders a comprehensive list of all indexed signatures with line numbers.
func (idx *CodebaseIndex) GenerateSignaturesOverview() string {
	var sb strings.Builder

	files := make([]string, 0, len(idx.ByFile))
	for f := range idx.ByFile {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, f := range files {
		syms := idx.ByFile[f]
		sort.Slice(syms, func(i, j int) bool {
			return syms[i].StartLine < syms[j].StartLine
		})

		sb.WriteString(fmt.Sprintf("<file path=\"%s\">\n", f))
		for _, s := range syms {
			sb.WriteString(fmt.Sprintf("%5d | %s [%s, lines %d-%d]\n", s.StartLine, s.Signature, s.Kind, s.StartLine, s.EndLine))
		}
		sb.WriteString("</file>\n\n")
	}

	return sb.String()
}

func receiverMatches(receiver, want string) bool {
	receiver = cleanReceiverName(receiver)
	want = cleanReceiverName(want)
	if receiver == "" || want == "" {
		return false
	}
	if receiver == want {
		return true
	}
	return strings.HasSuffix(want, "."+receiver) ||
		strings.HasSuffix(receiver, "."+want) ||
		strings.HasSuffix(want, receiver)
}

func cleanReceiverName(name string) string {
	name = cleanName(name)
	if idx := strings.IndexAny(name, "[<"); idx > 0 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

func cleanRefName(ref string) string {
	ref = cleanReceiverName(ref)
	ref = strings.Trim(ref, "[]{}()*&")
	if idx := strings.LastIndex(ref, "."); idx >= 0 {
		ref = ref[idx+1:]
	}
	return ref
}

func copyStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func copySeen(values map[string]bool) map[string]bool {
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func fileMatches(symbolFile, requested string) bool {
	if requested == "" {
		return true
	}
	sym := filepath.ToSlash(strings.TrimSpace(symbolFile))
	req := filepath.ToSlash(strings.TrimSpace(requested))
	req = strings.TrimPrefix(req, "./")
	if req == "" {
		return true
	}
	if sym == req {
		return true
	}
	return strings.HasSuffix(sym, "/"+strings.TrimPrefix(req, "/")) ||
		strings.HasSuffix(req, "/"+strings.TrimPrefix(sym, "/"))
}

// TraceDeep recursively extracts callee function bodies and referenced types up to maxDepth.
func (idx *CodebaseIndex) TraceDeep(rootSymbol string, maxDepth int) *DeepTraceResult {
	if maxDepth <= 0 {
		maxDepth = 2
	}
	matches := idx.FindDefinition(rootSymbol, "")
	if len(matches) == 0 {
		return nil
	}
	root := matches[0]

	res := &DeepTraceResult{
		RootSymbol: root,
	}

	visitedCalls := make(map[string]bool)
	visitedCallees := make(map[string]bool)
	visitedTypes := make(map[string]bool)

	var dfs func(current Symbol, depth int)
	dfs = func(current Symbol, depth int) {
		name := current.FullName()
		res.CallGraph = append(res.CallGraph, fmt.Sprintf("%s()", name))

		for _, ref := range current.References {
			cRef := cleanRefName(ref)
			if cRef == "" || visitedTypes[cRef] {
				continue
			}
			visitedTypes[cRef] = true
			typeDefs := idx.FindDefinition(cRef, "")
			for _, td := range typeDefs {
				if td.IsTypeKind() {
					res.Types = append(res.Types, td)
				}
			}
		}

		if depth >= maxDepth {
			return
		}

		for _, call := range current.Calls {
			callName := call
			if strings.HasPrefix(call, "this.") && current.Receiver != "" {
				callName = cleanReceiverName(current.Receiver) + "." + strings.TrimPrefix(call, "this.")
			}
			callees := idx.FindDefinition(callName, "")
			if len(callees) == 0 && current.Receiver != "" && strings.Contains(callName, ".") {
				methodName := callName[strings.LastIndex(callName, ".")+1:]
				callees = idx.FindDefinition(cleanReceiverName(current.Receiver)+"."+methodName, "")
			}

			if len(callees) > 0 {
				target := callees[0]
				key := fmt.Sprintf("%s:%d", target.File, target.StartLine)
				if !visitedCallees[key] {
					visitedCallees[key] = true
					res.Callees = append(res.Callees, DeepTraceNode{
						Symbol: target,
						Depth:  depth + 1,
					})
				}
				if !visitedCalls[key] {
					visitedCalls[key] = true
					dfs(target, depth+1)
				}
			}
		}
	}

	visitedCallees[fmt.Sprintf("%s:%d", root.File, root.StartLine)] = true
	dfs(root, 1)
	return res
}

// FindStructMethods retrieves methods attached to a struct or class by name.
func (idx *CodebaseIndex) FindStructMethods(structName string, methodNames []string) []Symbol {
	var methods []Symbol
	wanted := make(map[string]bool)
	for _, m := range methodNames {
		wanted[cleanName(m)] = true
	}

	for _, sym := range idx.Symbols {
		if sym.Kind == KindMethod && receiverMatches(sym.Receiver, structName) {
			if len(wanted) == 0 || wanted[sym.Name] {
				methods = append(methods, sym)
			}
		}
	}

	sort.Slice(methods, func(i, j int) bool {
		if methods[i].File == methods[j].File {
			return methods[i].StartLine < methods[j].StartLine
		}
		return methods[i].File < methods[j].File
	})

	return methods
}

// FindXRefs finds cross-references for a symbol across all indexed files.
func (idx *CodebaseIndex) FindXRefs(symbol, action, rootPath string) []XRefResult {
	cleanSym := cleanRefName(symbol)
	var results []XRefResult
	if cleanSym == "" {
		return results
	}

	seenFiles := make(map[string]bool)
	for _, sym := range idx.Symbols {
		seenFiles[sym.File] = true
	}

	for fileRel := range seenFiles {
		absPath := filepath.Join(rootPath, filepath.FromSlash(fileRel))
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, cleanSym) {
				st := i - 2
				if st < 0 {
					st = 0
				}
				en := i + 3
				if en > len(lines) {
					en = len(lines)
				}
				var ctx strings.Builder
				for j := st; j < en; j++ {
					prefix := "  "
					if j == i {
						prefix = "> "
					}
					ctx.WriteString(fmt.Sprintf("%s%5d | %s\n", prefix, j+1, lines[j]))
				}

				matchAction := "read"
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, ":=") || strings.Contains(trimmed, "=") || strings.Contains(trimmed, "var ") {
					matchAction = "write"
				} else if strings.Contains(trimmed, cleanSym+"(") {
					matchAction = "call"
				}

				if action == "all" || action == "" || action == matchAction {
					results = append(results, XRefResult{
						File:     fileRel,
						Line:     i + 1,
						Action:   matchAction,
						LineText: trimmed,
						Context:  strings.TrimRight(ctx.String(), "\n"),
					})
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].File == results[j].File {
			return results[i].Line < results[j].Line
		}
		return results[i].File < results[j].File
	})

	return results
}
