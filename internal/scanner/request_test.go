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

func TestExtractFromToBlock(t *testing.T) {
	code := `
line 1
case *OtherExpr:
    handleOther()
case *PromoteExpr:
    val := e.Value
    genPromote(val)
case *NextExpr:
    handleNext()
line 10
`
	req := FileRequest{
		Path: "test.go",
		From: "case *PromoteExpr:",
		To:   "case *NextExpr:",
	}

	out, minL, maxL := extractSections(code, req)
	if minL != 5 || maxL != 8 {
		t.Errorf("expected lines 5-8, got %d-%d", minL, maxL)
	}
	if !strings.Contains(out, "case *PromoteExpr:") || !strings.Contains(out, "genPromote(val)") {
		t.Errorf("expected block content missing:\n%s", out)
	}
}

func TestParseDefRequests(t *testing.T) {
	input := `
	<request>
		<def name="Config" trace="true" />
		<file path="service.ts" def="UserService" />
		<def symbol="Router" />
	</request>
	`

	reqs := ParseRequest(input)
	if len(reqs) != 3 {
		t.Fatalf("expected 3 def requests, got %d: %+v", len(reqs), reqs)
	}

	byDef := make(map[string]FileRequest)
	for _, req := range reqs {
		byDef[req.Def] = req
	}

	if req := byDef["Config"]; !req.Trace {
		t.Errorf("expected Config trace request: %+v", req)
	}
	if req := byDef["UserService"]; req.Path != "service.ts" || req.Trace {
		t.Errorf("unexpected file def req: %+v", req)
	}
	if _, ok := byDef["Router"]; !ok {
		t.Errorf("expected Router symbol def request: %+v", reqs)
	}
}

func TestLoadRequestsASTDefinitionAndTrace(t *testing.T) {
	dir := t.TempDir()
	source := `package sample

func A() {
	B()
}

func B() {
	C()
}

func C() {}
`
	if err := os.WriteFile(filepath.Join(dir, "flow.go"), []byte(source), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cfg := &config.Config{OutputFile: "out.txt"}
	var buf bytes.Buffer
	result, err := LoadRequests(dir, cfg, []FileRequest{{Def: "A", Trace: true}}, &buf, ScanOptions{Format: "raw"})
	if err != nil {
		t.Fatalf("LoadRequests failed: %v", err)
	}
	if result.FilesCount != 1 {
		t.Errorf("FilesCount = %d, want 1", result.FilesCount)
	}

	output := buf.String()
	for _, want := range []string{"Execution Flow", "A()", "B()", "C()", "/* Definition: function A", "flow.go:3-5"} {
		if !strings.Contains(output, want) {
			t.Errorf("AST output missing %q:\n%s", want, output)
		}
	}
}
