package targets_test

import (
	"strings"
	"testing"

	"github.com/JohnEsleyer/textify/internal/targets"
	_ "github.com/JohnEsleyer/textify/internal/targets/css"
	_ "github.com/JohnEsleyer/textify/internal/targets/golang"
	_ "github.com/JohnEsleyer/textify/internal/targets/html"
	_ "github.com/JohnEsleyer/textify/internal/targets/react"
)

func TestRegistryDispatchesByFileTarget(t *testing.T) {
	cases := []struct {
		path    string
		content string
		want    string
	}{
		{
			path:    "main.go",
			content: "package main\n\nfunc Run() {\n\tprintln(\"x\")\n}\n",
			want:    "func Run() { /* ... */ }",
		},
		{
			path:    "App.tsx",
			content: "export const App = () => {\n\treturn null;\n};\n",
			want:    "/* component */",
		},
		{
			path:    "index.html",
			content: "<div id=\"app\">hello</div>\n<script>\nconsole.log(1);\n</script>\n",
			want:    "/* script content omitted */",
		},
		{
			path:    "theme.css",
			content: ":root {\n--brand: red;\n}\n.btn {\ncolor: red;\n}\n",
			want:    ".btn { /* ... */ }",
		},
	}

	for _, tc := range cases {
		if targets.FindTarget(tc.path) == nil {
			t.Fatalf("no target registered for %s", tc.path)
		}
		got := targets.ExtractSignatures(tc.path, tc.content)
		if !strings.Contains(got, tc.want) {
			t.Errorf("ExtractSignatures(%s) missing %q:\n%s", tc.path, tc.want, got)
		}
	}
}

func TestRegistryGenericFallback(t *testing.T) {
	content := "fn main() {\n    let x = 1;\n}\n"
	got := targets.ExtractSignatures("main.rs", content)
	if !strings.Contains(got, "fn main() { /* ... */ }") {
		t.Errorf("generic fallback did not collapse Rust function: %s", got)
	}
	if strings.Contains(got, "let x = 1") {
		t.Errorf("generic fallback should strip function body: %s", got)
	}
}
