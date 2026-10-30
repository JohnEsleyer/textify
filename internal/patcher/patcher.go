package patcher

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JohnEsleyer/textify/internal/parser"
)

// ChangeType represents the kind of operation applied.
type ChangeType string

const (
	ChangeCreate ChangeType = "CREATE"
	ChangeUpdate ChangeType = "UPDATE"
	ChangeDelete ChangeType = "DELETE"
)

// ChangeRecord records a validated modification to a file.
type ChangeRecord struct {
	Type ChangeType
	Path string
	Diff string
}

// PatchOptions configures patch execution.
type PatchOptions struct {
	DryRun bool
	Fix    bool
}

// PatchResult contains results from applying a patch bundle.
type PatchResult struct {
	Changes []ChangeRecord
}

type fileOp struct {
	kind     ChangeType
	path     string
	content  string
	replaces []replaceDirective
}

type replaceDirective struct {
	symbol  string
	fn      string
	caseVal string
	within  string
	search  string
	with    string
	after   string
	before  string
	from    string
	to      string
}

var (
	tagFileRegex    = regexp.MustCompile(`(?is)<file\s+path=["']([^"']+)["']\s*>(.*?)</file>`)
	tagDeleteRegex  = regexp.MustCompile(`(?i)<delete\s+path=["']([^"']+)["']\s*/>`)
	tagReplaceRegex = regexp.MustCompile(`(?is)<replace\s+([^>]+)>(.*?)</replace>`)

	attrPath   = regexp.MustCompile(`(?i)\bpath=["']([^"']+)["']`)
	attrSymbol = regexp.MustCompile(`(?i)\bsymbol=["']([^"']+)["']`)
	attrFn     = regexp.MustCompile(`(?i)\b(?:fn|func)=["']([^"']+)["']`)
	attrCase   = regexp.MustCompile(`(?i)\bcase=["']([^"']+)["']`)
	attrWithin = regexp.MustCompile(`(?i)\bwithin=["']([^"']+)["']`)
	attrAfter  = regexp.MustCompile(`(?i)\bafter=["']([^"']+)["']`)
	attrBefore = regexp.MustCompile(`(?i)\bbefore=["']([^"']+)["']`)
	attrFrom   = regexp.MustCompile(`(?i)\bfrom=["']([^"']+)["']`)
	attrTo     = regexp.MustCompile(`(?i)\bto=["']([^"']+)["']`)

	searchBlockRegex = regexp.MustCompile(`(?is)<search>\s*(.*?)\s*</search>`)
	withBlockRegex   = regexp.MustCompile(`(?is)<with>\s*(.*?)\s*</with>`)
)

// Apply parses and executes a multi-file patch atomically using a two-pass VFS.
func Apply(root string, patchContent string, opts PatchOptions) (*PatchResult, error) {
	ops, err := parsePatchDirectives(patchContent)
	if err != nil {
		return nil, err
	}

	if len(ops) == 0 {
		return nil, fmt.Errorf("no valid patch directives (<file>, <replace>, or <delete>) found in input")
	}

	// PASS 1: Virtual File System (VFS) in-memory evaluation
	vfs := make(map[string]string)
	vfsDeleted := make(map[string]bool)

	// Load existing file contents into VFS
	for _, op := range ops {
		cleanP := filepath.ToSlash(filepath.Clean(op.path))
		if _, ok := vfs[cleanP]; ok {
			continue
		}
		absP := filepath.Join(root, filepath.FromSlash(cleanP))
		data, readErr := os.ReadFile(absP)
		if readErr == nil {
			vfs[cleanP] = string(data)
		}
	}

	var changes []ChangeRecord

	for _, op := range ops {
		cleanP := filepath.ToSlash(filepath.Clean(op.path))
		switch op.kind {
		case ChangeCreate:
			vfs[cleanP] = op.content
			delete(vfsDeleted, cleanP)
			changes = append(changes, ChangeRecord{
				Type: ChangeCreate,
				Path: cleanP,
			})

		case ChangeDelete:
			vfsDeleted[cleanP] = true
			delete(vfs, cleanP)
			changes = append(changes, ChangeRecord{
				Type: ChangeDelete,
				Path: cleanP,
			})

		case ChangeUpdate:
			curr, exists := vfs[cleanP]
			if !exists || vfsDeleted[cleanP] {
				return nil, fmt.Errorf("cannot patch %q: file does not exist", cleanP)
			}

			updated := curr
			for _, rep := range op.replaces {
				newContent, repErr := applyReplace(cleanP, updated, rep, opts.Fix)
				if repErr != nil {
					return nil, fmt.Errorf("failed to apply replace on %q: %w", cleanP, repErr)
				}
				updated = newContent
			}

			// Format Go files if modified
			if strings.HasSuffix(cleanP, ".go") {
				if formatted, err := format.Source([]byte(updated)); err == nil {
					updated = string(formatted)
				}
			}

			vfs[cleanP] = updated
			changes = append(changes, ChangeRecord{
				Type: ChangeUpdate,
				Path: cleanP,
			})
		}
	}

	// PASS 2: Atomic commit to disk (unless DryRun)
	if !opts.DryRun {
		for _, op := range ops {
			cleanP := filepath.ToSlash(filepath.Clean(op.path))
			absP := filepath.Join(root, filepath.FromSlash(cleanP))

			if vfsDeleted[cleanP] {
				_ = os.Remove(absP)
				continue
			}

			if content, ok := vfs[cleanP]; ok {
				if err := os.MkdirAll(filepath.Dir(absP), 0755); err != nil {
					return nil, fmt.Errorf("create dir for %q: %w", cleanP, err)
				}
				if err := os.WriteFile(absP, []byte(content), 0644); err != nil {
					return nil, fmt.Errorf("write %q: %w", cleanP, err)
				}
			}
		}
	}

	return &PatchResult{Changes: changes}, nil
}

func parsePatchDirectives(input string) ([]fileOp, error) {
	var ops []fileOp
	fileReplaces := make(map[string][]replaceDirective)

	// 1. <file path="...">
	for _, m := range tagFileRegex.FindAllStringSubmatch(input, -1) {
		path := filepath.ToSlash(strings.TrimSpace(m[1]))
		content := m[2]
		if strings.HasPrefix(content, "\r\n") {
			content = content[2:]
		} else if strings.HasPrefix(content, "\n") {
			content = content[1:]
		}
		ops = append(ops, fileOp{
			kind:    ChangeCreate,
			path:    path,
			content: content,
		})
	}

	// 2. <delete path="..." />
	for _, m := range tagDeleteRegex.FindAllStringSubmatch(input, -1) {
		path := filepath.ToSlash(strings.TrimSpace(m[1]))
		ops = append(ops, fileOp{
			kind: ChangeDelete,
			path: path,
		})
	}

	// 3. <replace ...>
	for _, m := range tagReplaceRegex.FindAllStringSubmatch(input, -1) {
		attrs := m[1]
		body := m[2]

		pMatch := attrPath.FindStringSubmatch(attrs)
		if len(pMatch) < 2 {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(pMatch[1]))

		rep := replaceDirective{}
		if sm := attrSymbol.FindStringSubmatch(attrs); len(sm) >= 2 {
			rep.symbol = strings.TrimSpace(sm[1])
		}
		if fm := attrFn.FindStringSubmatch(attrs); len(fm) >= 2 {
			rep.fn = strings.TrimSpace(fm[1])
		}
		if cm := attrCase.FindStringSubmatch(attrs); len(cm) >= 2 {
			rep.caseVal = strings.TrimSpace(cm[1])
		}
		if wm := attrWithin.FindStringSubmatch(attrs); len(wm) >= 2 {
			rep.within = strings.TrimSpace(wm[1])
		}
		if am := attrAfter.FindStringSubmatch(attrs); len(am) >= 2 {
			rep.after = strings.TrimSpace(am[1])
		}
		if bm := attrBefore.FindStringSubmatch(attrs); len(bm) >= 2 {
			rep.before = strings.TrimSpace(bm[1])
		}
		if fromM := attrFrom.FindStringSubmatch(attrs); len(fromM) >= 2 {
			rep.from = strings.TrimSpace(fromM[1])
		}
		if toM := attrTo.FindStringSubmatch(attrs); len(toM) >= 2 {
			rep.to = strings.TrimSpace(toM[1])
		}

		if sMatch := searchBlockRegex.FindStringSubmatch(body); len(sMatch) >= 2 {
			rep.search = sMatch[1]
		}
		if wMatch := withBlockRegex.FindStringSubmatch(body); len(wMatch) >= 2 {
			rep.with = wMatch[1]
		} else {
			// Content of replace without with tags
			rep.with = strings.TrimSpace(body)
		}

		fileReplaces[path] = append(fileReplaces[path], rep)
	}

	for p, reps := range fileReplaces {
		ops = append(ops, fileOp{
			kind:     ChangeUpdate,
			path:     p,
			replaces: reps,
		})
	}

	return ops, nil
}

func applyReplace(filePath, content string, rep replaceDirective, fix bool) (string, error) {
	// 1. AST Case Clause Replacement
	if rep.caseVal != "" {
		body, start, end, err := parser.ExtractCase(filePath, content, rep.fn, rep.caseVal)
		if err != nil {
			return "", fmt.Errorf("case target %q not found: %w", rep.caseVal, err)
		}
		_ = body
		lines := strings.Split(content, "\n")
		var out []string
		out = append(out, lines[:start-1]...)
		out = append(out, rep.with)
		if end < len(lines) {
			out = append(out, lines[end:]...)
		}
		return strings.Join(out, "\n"), nil
	}

	// 2. AST Symbol Replacement
	if rep.symbol != "" {
		syms, err := parser.ParseGoFile(filePath, []byte(content))
		if err == nil {
			for _, s := range syms {
				if s.Name == rep.symbol || s.FullName() == rep.symbol {
					lines := strings.Split(content, "\n")
					var out []string
					out = append(out, lines[:s.StartLine-1]...)
					out = append(out, rep.with)
					if s.EndLine < len(lines) {
						out = append(out, lines[s.EndLine:]...)
					}
					return strings.Join(out, "\n"), nil
				}
			}
		}
	}

	// 3. Anchor Insertions (after / before)
	if rep.after != "" {
		idx := strings.Index(content, rep.after)
		if idx == -1 {
			return "", fmt.Errorf("anchor after=%q not found", rep.after)
		}
		insertPos := idx + len(rep.after)
		return content[:insertPos] + "\n" + rep.with + content[insertPos:], nil
	}
	if rep.before != "" {
		idx := strings.Index(content, rep.before)
		if idx == -1 {
			return "", fmt.Errorf("anchor before=%q not found", rep.before)
		}
		return content[:idx] + rep.with + "\n" + content[idx:], nil
	}

	// 4. Block Replacement (from ... to)
	if rep.from != "" && rep.to != "" {
		lines := strings.Split(content, "\n")
		startIdx := -1
		endIdx := -1
		for i, l := range lines {
			if startIdx == -1 && strings.Contains(l, rep.from) {
				startIdx = i
			} else if startIdx != -1 && strings.Contains(l, rep.to) {
				endIdx = i
				break
			}
		}
		if startIdx == -1 || endIdx == -1 {
			return "", fmt.Errorf("from/to block [%q ... %q] not found", rep.from, rep.to)
		}
		var out []string
		out = append(out, lines[:startIdx]...)
		out = append(out, rep.with)
		if endIdx+1 < len(lines) {
			out = append(out, lines[endIdx+1:]...)
		}
		return strings.Join(out, "\n"), nil
	}

	// 5. Search Block with Scope Bounding (within)
	targetText := content
	prefixOffset := 0
	if rep.within != "" {
		syms, err := parser.ParseGoFile(filePath, []byte(content))
		if err == nil {
			for _, s := range syms {
				if s.Name == rep.within || s.FullName() == rep.within || strings.HasSuffix(s.FullName(), "."+rep.within) {
					lines := strings.Split(content, "\n")
					scoped := strings.Join(lines[s.StartLine-1:s.EndLine], "\n")
					updatedScoped, err := replaceSearchText(scoped, rep.search, rep.with, fix)
					if err != nil {
						return "", fmt.Errorf("within=%q: %w", rep.within, err)
					}
					var out []string
					out = append(out, lines[:s.StartLine-1]...)
					out = append(out, updatedScoped)
					if s.EndLine < len(lines) {
						out = append(out, lines[s.EndLine:]...)
					}
					return strings.Join(out, "\n"), nil
				}
			}
		}
	}

	// 6. Direct Search and Replace (exact, wildcard ellipsis, or fuzzy)
	_ = prefixOffset
	return replaceSearchText(targetText, rep.search, rep.with, fix)
}

func replaceSearchText(content, search, with string, fix bool) (string, error) {
	searchTrim := strings.TrimSpace(search)
	if searchTrim == "" {
		return content, nil
	}

	// Exact match
	if strings.Contains(content, search) {
		return strings.Replace(content, search, with, 1), nil
	}

	// Wildcard / ellipsis (...) matching
	if strings.Contains(search, "...") {
		parts := strings.Split(search, "...")
		var reStr bytes.Buffer
		for i, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			if i > 0 {
				reStr.WriteString(`(?s:.*?)`)
			}
			reStr.WriteString(regexp.QuoteMeta(trimmed))
		}
		re, err := regexp.Compile(reStr.String())
		if err == nil && re.MatchString(content) {
			return re.ReplaceAllString(content, with), nil
		}
	}

	// Normalized whitespace / indentation tolerance
	cLines := strings.Split(content, "\n")
	sLines := strings.Split(search, "\n")

	cleanSLines := filterEmpty(sLines)
	if len(cleanSLines) == 0 {
		return content, nil
	}

	for i := 0; i <= len(cLines)-len(cleanSLines); i++ {
		matched := true
		for j := 0; j < len(cleanSLines); j++ {
			if strings.TrimSpace(cLines[i+j]) != strings.TrimSpace(cleanSLines[j]) {
				matched = false
				break
			}
		}
		if matched {
			var out []string
			out = append(out, cLines[:i]...)
			out = append(out, with)
			if i+len(cleanSLines) < len(cLines) {
				out = append(out, cLines[i+len(cleanSLines):]...)
			}
			return strings.Join(out, "\n"), nil
		}
	}

	if fix {
		// Fuzzy search fallback
		for i := 0; i < len(cLines); i++ {
			if strings.Contains(strings.TrimSpace(cLines[i]), strings.TrimSpace(cleanSLines[0])) {
				var out []string
				out = append(out, cLines[:i]...)
				out = append(out, with)
				endIdx := i + len(cleanSLines)
				if endIdx > len(cLines) {
					endIdx = len(cLines)
				}
				out = append(out, cLines[endIdx:]...)
				return strings.Join(out, "\n"), nil
			}
		}
	}

	return "", fmt.Errorf("search block not matched:\n%s", search)
}

func filterEmpty(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
