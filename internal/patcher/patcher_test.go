package patcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyCreateFile(t *testing.T) {
	dir := t.TempDir()

	patch := `<file path="hello.txt">
Hello World!
</file>`

	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(res.Changes) != 1 || res.Changes[0].Type != ChangeCreate {
		t.Fatalf("Expected 1 CREATE change, got %+v", res.Changes)
	}

	content, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if strings.TrimSpace(string(content)) != "Hello World!" {
		t.Errorf("got %q, want 'Hello World!'", string(content))
	}
}

func TestApplyReplaceSearch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	orig := `package main

func main() {
	println("old")
}
`
	if err := os.WriteFile(filePath, []byte(orig), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `<replace path="main.go">
<search>
	println("old")
</search>
<with>
	println("new")
</with>
</replace>`

	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(res.Changes) != 1 || res.Changes[0].Type != ChangeUpdate {
		t.Fatalf("Expected 1 UPDATE change, got %+v", res.Changes)
	}

	content, _ := os.ReadFile(filePath)
	if !strings.Contains(string(content), `println("new")`) {
		t.Errorf("content does not contain updated line:\n%s", string(content))
	}
}

func TestApplyReplaceASTCase(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "calc.go")
	orig := `package calc

func eval(op string) int {
	switch op {
	case "add":
		return 1 + 1
	case "sub":
		return 1 - 1
	default:
		return 0
	}
}
`
	if err := os.WriteFile(filePath, []byte(orig), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `<replace path="calc.go" fn="eval" case="add">
<with>
	case "add":
		return 2 + 2
</with>
</replace>`

	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(res.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(res.Changes))
	}

	content, _ := os.ReadFile(filePath)
	if !strings.Contains(string(content), "return 2 + 2") {
		t.Errorf("AST case replacement not applied:\n%s", string(content))
	}
}

func TestApplyScopedReplacement(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "scope.go")
	orig := `package main

func first() {
	println("hello")
}

func second() {
	println("hello")
}
`
	if err := os.WriteFile(filePath, []byte(orig), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `<replace path="scope.go" within="second">
<search>
	println("hello")
</search>
<with>
	println("world")
</with>
</replace>`

	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(res.Changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(res.Changes))
	}

	content, _ := os.ReadFile(filePath)
	cStr := string(content)
	if !strings.Contains(cStr, "func first() {\n\tprintln(\"hello\")") {
		t.Errorf("first() was modified unexpectedly:\n%s", cStr)
	}
	if !strings.Contains(cStr, "func second() {\n\tprintln(\"world\")") {
		t.Errorf("second() was not updated:\n%s", cStr)
	}
}

func TestApplyDeleteFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "obsolete.go")
	if err := os.WriteFile(filePath, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `<delete path="obsolete.go" />`
	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(res.Changes) != 1 || res.Changes[0].Type != ChangeDelete {
		t.Fatalf("Expected 1 DELETE change, got %+v", res.Changes)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("File obsolete.go was not deleted")
	}
}

func TestApplyDryRun(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "dry.txt")
	if err := os.WriteFile(filePath, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `<replace path="dry.txt">
<search>
initial
</search>
<with>
modified
</with>
</replace>`

	res, err := Apply(dir, patch, PatchOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Dry run apply failed: %v", err)
	}

	if len(res.Changes) != 1 {
		t.Fatalf("Expected 1 validated change, got %d", len(res.Changes))
	}

	content, _ := os.ReadFile(filePath)
	if string(content) != "initial" {
		t.Errorf("File was modified during dry run: %q", string(content))
	}
}
