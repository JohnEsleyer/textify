package patcher

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileCreateAndOverwrite(t *testing.T) {
	dir := t.TempDir()

	patch := `
<file path="nested/app/main.go">
package main

import "fmt"

func main() {
    fmt.Println("Created")
}
</file>
`
	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(res.Changes) != 1 || res.Changes[0].Type != ChangeCreate {
		t.Fatalf("unexpected changes: %+v", res.Changes)
	}

	data, err := os.ReadFile(filepath.Join(dir, "nested/app/main.go"))
	if err != nil || !strings.Contains(string(data), `fmt.Println("Created")`) {
		t.Fatalf("file not written properly: %v", err)
	}

	// Overwrite
	patchOverwrite := `
<file path="nested/app/main.go">
package main

func main() {}
</file>
`
	res2, err := Apply(dir, patchOverwrite, PatchOptions{})
	if err != nil || res2.Changes[0].Type != ChangeUpdate {
		t.Fatalf("Overwrite failed: %v", err)
	}

	data2, _ := os.ReadFile(filepath.Join(dir, "nested/app/main.go"))
	if strings.Contains(string(data2), "Created") {
		t.Errorf("file should have been overwritten")
	}
}

func TestFileDelete(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "obsolete.go")
	os.WriteFile(target, []byte("package main"), 0644)

	patch := `<delete path="obsolete.go" />`
	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(res.Changes) != 1 || res.Changes[0].Type != ChangeDelete {
		t.Fatalf("unexpected changes: %+v", res.Changes)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file was not deleted")
	}
}

func TestReplaceSymbolGo(t *testing.T) {
	dir := t.TempDir()
	code := `package main

func OldFunc() int {
    return 1
}

func Other() {}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(code), 0644)

	patch := `
<replace path="main.go" symbol="OldFunc">
<with>
func OldFunc() int {
    return 42
}
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(updated), "return 42") || !strings.Contains(string(updated), "func Other()") {
		t.Errorf("Symbol replacement failed:\n%s", string(updated))
	}
}

func TestReplaceSymbolTypeScript(t *testing.T) {
	dir := t.TempDir()
	tsCode := `export class Service {
  greet(name: string): string {
    return "hi " + name;
  }
}
`
	os.WriteFile(filepath.Join(dir, "service.ts"), []byte(tsCode), 0644)

	patch := `
<replace path="service.ts" symbol="Service.greet">
<with>
  greet(name: string): string {
    return "hello " + name;
  }
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "service.ts"))
	if !strings.Contains(string(updated), "hello ") {
		t.Errorf("TypeScript symbol replacement failed:\n%s", string(updated))
	}
}

func TestReplaceSearchFuzzyAndEllipsis(t *testing.T) {
	dir := t.TempDir()
	code := `package config

type Config struct {
    Host string
    Timeout int
    Port int
}
`
	os.WriteFile(filepath.Join(dir, "config.go"), []byte(code), 0644)

	patch := `
<replace path="config.go">
<search>
type Config struct {
    ...
    Timeout int
    ...
}
</search>
<with>
type Config struct {
    ...
    Timeout int
    Retries int
    ...
}
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply with ellipsis failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "config.go"))
	out := string(updated)

	for _, want := range []string{"Host string", "Timeout int", "Retries int", "Port int"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in updated content:\n%s", want, out)
		}
	}
}

func TestReplaceAnchorAfterAndBefore(t *testing.T) {
	dir := t.TempDir()
	code := `line 1
anchor line
line 3
`
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(code), 0644)

	patch := `
<replace path="test.txt" after="anchor line">
<with>
inserted after
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Anchor after failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "test.txt"))
	if !strings.Contains(string(updated), "anchor line\ninserted after\nline 3") {
		t.Errorf("unexpected output after anchor:\n%s", string(updated))
	}
}

func TestReplaceFromTo(t *testing.T) {
	dir := t.TempDir()
	code := `case *A:
    handleA()
case *Promote:
    oldLogic()
case *B:
    handleB()
`
	os.WriteFile(filepath.Join(dir, "switch.go"), []byte(code), 0644)

	patch := `
<replace path="switch.go" from="case *Promote:" to="case *B:">
<with>
case *Promote:
    newLogic()
case *B:
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("From-To failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "switch.go"))
	if !strings.Contains(string(updated), "newLogic()") || strings.Contains(string(updated), "oldLogic()") {
		t.Errorf("unexpected output:\n%s", string(updated))
	}
}

func TestAtomicRollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "f1.txt")
	os.WriteFile(f1, []byte("original f1"), 0644)

	patch := `
<file path="f1.txt">
modified f1
</file>

<replace path="f2.txt" symbol="NonExistent">
<with>
foo
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{})
	if err == nil {
		t.Fatal("expected failure due to missing f2.txt")
	}

	f1Data, _ := os.ReadFile(f1)
	if string(f1Data) != "original f1" {
		t.Errorf("atomic rollback failed! f1 was modified on disk: %s", string(f1Data))
	}
}

func TestSanitizeLineNumbersAndCodeFences(t *testing.T) {
	dir := t.TempDir()

	patch := "   \n" +
		"<file path=\"clean.go\">\n" +
		"```go\n" +
		"    1 | package main\n" +
		"    2 | \n" +
		"    3 | func Clean() {}\n" +
		"```\n" +
		"</file>\n"

	_, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "clean.go"))
	if strings.Contains(string(data), "|") || strings.Contains(string(data), "```") {
		t.Errorf("line numbers or fences not stripped:\n%s", string(data))
	}
	if !strings.Contains(string(data), "package main") || !strings.Contains(string(data), "func Clean() {}") {
		t.Errorf("clean code missing:\n%s", string(data))
	}
}

func TestReplaceASTCaseGo(t *testing.T) {
	dir := t.TempDir()
	source := `package compiler

func inferExpr(expr interface{}) string {
	switch e := expr.(type) {
	case *PromoteExpr:
		return "promote"
	case *MemberExpr:
		return "old member logic"
	default:
		return "unknown"
	}
}
`
	os.WriteFile(filepath.Join(dir, "typechecker.go"), []byte(source), 0644)

	patch := `
<replace path="typechecker.go" fn="inferExpr" case="*MemberExpr">
<with>
	case *MemberExpr:
		return "new AST replaced logic"
</with>
</replace>
`
	res, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("AST case replacement failed: %v", err)
	}
	if len(res.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(res.Changes))
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "typechecker.go"))
	out := string(updated)

	if !strings.Contains(out, "new AST replaced logic") {
		t.Errorf("AST case replacement missing new logic:\n%s", out)
	}
	if strings.Contains(out, "old member logic") {
		t.Errorf("Old case logic was not removed:\n%s", out)
	}
	if !strings.Contains(out, "case *PromoteExpr:") || !strings.Contains(out, "default:") {
		t.Errorf("Unrelated cases were corrupted:\n%s", out)
	}
}

func TestReplaceWithinScope(t *testing.T) {
	dir := t.TempDir()
	source := `package main

func funcA() {
	if x > 0 {
		doSomething()
	}
}

func funcB() {
	if x > 0 {
		doSomething()
	}
}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0644)

	patch := `
<replace path="main.go" within="funcB">
<search>
	if x > 0 {
		doSomething()
	}
</search>
<with>
	if x > 0 {
		doSomethingB()
	}
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{})
	if err != nil {
		t.Fatalf("Scoped within replacement failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	out := string(updated)

	// funcA must remain untouched
	if !strings.Contains(out, "func funcA() {\n\tif x > 0 {\n\t\tdoSomething()\n\t}\n}") {
		t.Errorf("funcA was incorrectly modified:\n%s", out)
	}

	// funcB must have the update
	if !strings.Contains(out, "func funcB() {\n\tif x > 0 {\n\t\tdoSomethingB()\n\t}\n}") {
		t.Errorf("funcB was not updated:\n%s", out)
	}
}

func TestTokenNormalizedFuzzyAutoHealing(t *testing.T) {
	dir := t.TempDir()
	source := `package main

func process() {
	start()
	if variants, isEnum := tc.Global.Enums[id.Name]; isEnum {
		handleEnum(variants)
	}
	finish()
}
`
	os.WriteFile(filepath.Join(dir, "process.go"), []byte(source), 0644)

	// Notice the LLM wrote "variants, ok" instead of "variants, isEnum"
	patch := `
<replace path="process.go">
<search>
	start()
	if variants, ok := tc.Global.Enums[id.Name]; ok {
		handleEnum(variants)
	}
	finish()
</search>
<with>
	start()
	if variants, isEnum := tc.Global.Enums[id.Name]; isEnum {
		handleEnumV2(variants)
	}
	finish()
</with>
</replace>
`
	// Should auto-heal cleanly
	_, err := Apply(dir, patch, PatchOptions{AutoFix: true})
	if err != nil {
		t.Fatalf("Fuzzy auto-healing failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "process.go"))
	if !strings.Contains(string(updated), "handleEnumV2(variants)") {
		t.Errorf("Auto-healed replacement not applied:\n%s", string(updated))
	}
}

func TestUnifiedDiffOnFailure(t *testing.T) {
	dir := t.TempDir()
	source := `package main

func check() {
	var a = 1
	var b = 2
	var c = 3
}
`
	os.WriteFile(filepath.Join(dir, "check.go"), []byte(source), 0644)

	patch := `
<replace path="check.go">
<search>
	var a = 1
	var b = 999
	var c = 3
</search>
<with>
	var a = 10
</with>
</replace>
`
	_, err := Apply(dir, patch, PatchOptions{AutoFix: false})
	if err == nil {
		t.Fatal("expected failure due to mismatch")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "Closest match found near line") {
		t.Errorf("error missing near line hint: %s", errStr)
	}
	if !strings.Contains(errStr, "--- Search Block") || !strings.Contains(errStr, "+++ Buffer near Line") {
		t.Errorf("error missing unified diff headers: %s", errStr)
	}
	if !strings.Contains(errStr, "-   \tvar b = 999") || !strings.Contains(errStr, "+   \tvar b = 2") {
		t.Errorf("error missing character diff lines: %s", errStr)
	}
}

func TestInteractiveAutoFixPrompt(t *testing.T) {
	dir := t.TempDir()
	source := `package main

func run() {
	count := getCount()
	if count > 0 {
		execute()
	}
}
`
	os.WriteFile(filepath.Join(dir, "run.go"), []byte(source), 0644)

	patch := `
<replace path="run.go">
<search>
	count := getCount()
	if cnt > 0 {
		execute()
	}
</search>
<with>
	count := getCount()
	if count > 0 {
		executeUpdated()
	}
</with>
</replace>
`
	var outBuf bytes.Buffer
	inBuf := bytes.NewBufferString("y\n")

	opts := PatchOptions{
		AutoFix: true,
		Stdin:   inBuf,
		Stdout:  &outBuf,
	}

	_, err := Apply(dir, patch, opts)
	if err != nil {
		t.Fatalf("Interactive prompt fix failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(dir, "run.go"))
	if !strings.Contains(string(updated), "executeUpdated()") {
		t.Errorf("Substitution was not applied after accepting interactive prompt:\n%s", string(updated))
	}
}
