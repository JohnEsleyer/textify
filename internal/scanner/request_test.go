package scanner

import (
	"strings"
	"testing"
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