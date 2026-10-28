package scanner

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseLineRanges(t *testing.T) {
	tests := []struct {
		input    string
		expected []LineRange
	}{
		{"50-120", []LineRange{{Start: 50, End: 120}}},
		{"10-20, 50-80", []LineRange{{Start: 10, End: 20}, {Start: 50, End: 80}}},
		{"100:", []LineRange{{Start: 100, End: 0}}},
		{":50", []LineRange{{Start: 1, End: 50}}},
		{"42", []LineRange{{Start: 42, End: 42}}},
	}

	for _, tt := range tests {
		got := ParseLineRanges(tt.input)
		if !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("ParseLineRanges(%q) = %v, expected %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseRequestXML(t *testing.T) {
	input := `
	<request>
		<file path="pkg/compiler/ast.go" lines="50-120" />
		<file path="pkg/compiler/codegen.go" search="promote" context="4" />
		<file path="pkg/compiler/types.go" symbols="SliceType,ArrayType" />
		<file glob="tests/*.ube" />
	</request>
	`
	reqs := ParseRequest(input)
	if len(reqs) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(reqs))
	}

	if reqs[0].Path != "pkg/compiler/ast.go" || reqs[0].Start != 50 || reqs[0].End != 120 {
		t.Errorf("unexpected req 0: %+v", reqs[0])
	}
	if reqs[1].Query != "promote" || reqs[1].Context != 4 {
		t.Errorf("unexpected req 1: %+v", reqs[1])
	}
	if len(reqs[2].Symbols) != 2 || reqs[2].Symbols[0] != "SliceType" {
		t.Errorf("unexpected req 2: %+v", reqs[2])
	}
	if reqs[3].Glob != "tests/*.ube" {
		t.Errorf("unexpected req 3: %+v", reqs[3])
	}
}

func TestParseRequestMarkdown(t *testing.T) {
	input := `
	Please provide:
	- ` + "`" + `cmd/ube/main.go` + "`" + `:10-25
	- pkg/compiler/lexer.go:100:
	- pkg/compiler/parser.go (search: "parseExpr")
	`
	reqs := ParseRequest(input)
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}

	if reqs[0].Path != "cmd/ube/main.go" || reqs[0].Start != 10 || reqs[0].End != 25 {
		t.Errorf("unexpected req 0: %+v", reqs[0])
	}
	if reqs[1].Path != "pkg/compiler/lexer.go" || reqs[1].Start != 100 || reqs[1].End != 0 {
		t.Errorf("unexpected req 1: %+v", reqs[1])
	}
	if reqs[2].Path != "pkg/compiler/parser.go" || reqs[2].Query != "parseExpr" {
		t.Errorf("unexpected req 2: %+v", reqs[2])
	}
}

func TestExtractSectionsWithSearch(t *testing.T) {
	content := "line 1\nline 2\nline 3 TARGET\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9 TARGET\nline 10"
	req := FileRequest{
		Path:    "test.txt",
		Query:   "TARGET",
		Context: 1,
	}

	out, minL, maxL := extractSections(content, req)
	if minL != 2 || maxL != 10 {
		t.Errorf("expected min=2, max=10, got %d, %d", minL, maxL)
	}
	if !strings.Contains(out, "skipped lines") {
		t.Errorf("expected skipped lines indicator in multi-window search result:\n%s", out)
	}
	if !strings.Contains(out, "line 3 TARGET") || !strings.Contains(out, "line 9 TARGET") {
		t.Errorf("expected search matches to be present:\n%s", out)
	}
}