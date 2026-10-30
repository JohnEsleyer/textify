package scanner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JohnEsleyer/textify/internal/config"
)

func TestParseRequestFromToAndGrep(t *testing.T) {
	input := `
	<request>
		<grep pattern="PromoteExpr" context="12" />
		<file path="pkg/compiler/codegen_expr.go" from="case *PromoteExpr:" to="case *" />
		<file path="pkg/compiler/typechecker_expr.go" find="case *PromoteExpr:" context="15" />
	</request>
	`
	reqs := ParseRequest(input)
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}

	if reqs[0].Query != "PromoteExpr" || reqs[0].Context != 12 {
		t.Errorf("unexpected grep req: %+v", reqs[0])
	}
	if reqs[1].From != "case *PromoteExpr:" || reqs[1].To != "case *" {
		t.Errorf("unexpected from/to req: %+v", reqs[1])
	}
	if reqs[2].Query != "case *PromoteExpr:" || reqs[2].Context != 15 {
		t.Errorf("unexpected find req: %+v", reqs[2])
	}
}

func TestParseDeepTraceAndCaseRequests(t *testing.T) {
	input := `
	<request>
		<trace symbol="Typechecker.checkStmt" depth="2" for-patch="true" />
		<select path="compiler/expr.go" fn="inferExpr" case="*MemberExpr" for-patch="true" />
		<select struct="Typechecker" methods="Check,resolveType" />
		<xref symbol="FieldDecl" action="write" />
	</request>
	`
	reqs := ParseRequest(input)
	if len(reqs) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(reqs))
	}

	if reqs[0].Def != "Typechecker.checkStmt" || !reqs[0].Trace || reqs[0].Depth != 2 || !reqs[0].ForPatch {
		t.Errorf("unexpected trace req: %+v", reqs[0])
	}
	if reqs[1].Fn != "inferExpr" || reqs[1].Case != "*MemberExpr" || !reqs[1].ForPatch {
		t.Errorf("unexpected select case req: %+v", reqs[1])
	}
	if reqs[2].Struct != "Typechecker" || len(reqs[2].Methods) != 2 {
		t.Errorf("unexpected select struct req: %+v", reqs[2])
	}
	if reqs[3].XRef != "FieldDecl" || reqs[3].XRefAction != "write" {
		t.Errorf("unexpected xref req: %+v", reqs[3])
	}
}

func TestParseFeatureContext(t *testing.T) {
	input := `
	<request>
		<feature-context lang="go">
			<target-type name="compiler.StructDecl" />
			<entrypoint file="pkg/compiler/parser.go" symbol="Parser.parseDecl" depth="2" />
			<touchpoints pattern="TokenStruct" />
		</feature-context>
	</request>
	`
	reqs := ParseRequest(input)
	if len(reqs) != 1 || reqs[0].FeatureCtx == nil {
		t.Fatalf("expected 1 feature context request, got %+v", reqs)
	}

	fc := reqs[0].FeatureCtx
	if fc.Lang != "go" || len(fc.TargetTypes) != 1 || len(fc.Entrypoints) != 1 || len(fc.Touchpoints) != 1 {
		t.Errorf("unexpected feature context: %+v", fc)
	}
}

func TestLoadRequestsDeepTraceAndForPatch(t *testing.T) {
	dir := t.TempDir()
	source := `package sample

func A() {
	B()
}

func B() {
	C()
}

func C() {}

func infer(v interface{}) string {
	switch v.(type) {
	case int:
		return "int"
	default:
		return "other"
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "flow.go"), []byte(source), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cfg := &config.Config{OutputFile: "out.txt"}
	var buf bytes.Buffer

	// 1. Deep Trace Test
	_, err := LoadRequests(dir, cfg, []FileRequest{{Def: "A", Trace: true, Depth: 2}}, &buf, ScanOptions{Format: "raw"})
	if err != nil {
		t.Fatalf("LoadRequests trace failed: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Deep Contextual Trace", "Primary Definition: function A", "Callee: function B", "Callee: function C"} {
		if !strings.Contains(out, want) {
			t.Errorf("deep trace missing %q:\n%s", want, out)
		}
	}

	// 2. Case Selector + for-patch Test
	buf.Reset()
	_, err = LoadRequests(dir, cfg, []FileRequest{{Path: "flow.go", Fn: "infer", Case: "int", ForPatch: true}}, &buf, ScanOptions{Format: "raw"})
	if err != nil {
		t.Fatalf("LoadRequests case failed: %v", err)
	}
	outPatch := buf.String()
	if !strings.Contains(outPatch, `<replace path="flow.go" fn="infer" case="int">`) || strings.Contains(outPatch, " | ") {
		t.Errorf("expected clean patch-ready case replacement:\n%s", outPatch)
	}
}
