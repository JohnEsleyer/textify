package scanner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JohnEsleyer/textify/internal/config"
)

func TestScanWithGranularRules(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_granular")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	createFile(t, tempDir, "normal.go", "package main\n\nfunc Run() {}\n")
	createFile(t, tempDir, "annoying.go", "package main\n")
	createFile(t, tempDir, "data.json", "{\"data\": 1}\n")
	createFile(t, tempDir, "temp.log", "error log\n")
	createFile(t, tempDir, "secrets.env", "API=123\n")
	createFile(t, tempDir, "garbage.tmp", "trash\n")

	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.env\n"), 0644)

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled:           true,
				Extensions:        []string{"go", "json"},
				ExcludeExtensions: []string{"log", "tmp"},
				Include:           []string{"secrets.env"},
				Exclude:           []string{"annoying.go"},
			},
		},
	}

	var buf bytes.Buffer
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	output := buf.String()

	// Should Include with tags and line attributes
	assertContains(t, output, "\x3cfile path=\"normal.go\" lines=\"1-")
	assertContains(t, output, "\x3cfile path=\"data.json\" lines=\"1-")
	assertContains(t, output, "\x3cfile path=\"secrets.env\" lines=\"1-")

	// Should Exclude
	assertNotContains(t, output, "annoying.go")
	assertNotContains(t, output, "temp.log")
	assertNotContains(t, output, "garbage.tmp")
}

func TestMixedDirectoryRules(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_mixed")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.Mkdir(filepath.Join(tempDir, "backend"), 0755)
	os.Mkdir(filepath.Join(tempDir, "frontend"), 0755)

	createFile(t, tempDir, "backend/main.go", "package main\n")
	createFile(t, tempDir, "backend/test.js", "console.log('skip');\n")
	createFile(t, tempDir, "frontend/app.js", "console.log('app');\n")
	createFile(t, tempDir, "frontend/style.css", "body {}\n")

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled: true,
			},
			"backend": {
				Enabled:    true,
				Extensions: []string{"go"},
			},
			"frontend": {
				Enabled:    true,
				Extensions: []string{"js", "css"},
				Exclude:    []string{"style.css"},
			},
		},
	}

	var buf bytes.Buffer
	Scan(tempDir, cfg, &buf, ScanOptions{Tree: false})
	output := buf.String()

	assertContains(t, output, "\x3cfile path=\"backend/main.go\"")
	assertNotContains(t, output, "backend/test.js")
	assertContains(t, output, "\x3cfile path=\"frontend/app.js\"")
	assertNotContains(t, output, "frontend/style.css")
}

func TestSubfolderGitignoreAndDisabledSubdir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_subfolder_ignore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.MkdirAll(filepath.Join(tempDir, "frontend", "src"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "frontend", "node_modules"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "frontend", "dist"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "services", "auth", "dist"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "services", "auth", "src"), 0755)

	createFile(t, tempDir, "frontend/src/App.tsx", "export const App = () => null;\n")
	createFile(t, tempDir, "frontend/node_modules/pkg.js", "module.exports = {};\n")
	createFile(t, tempDir, "frontend/dist/bundle.js", "var x = 1;\n")
	createFile(t, tempDir, "frontend/.gitignore", "node_modules\n")

	createFile(t, tempDir, "services/auth/src/index.ts", "export const auth = true;\n")
	createFile(t, tempDir, "services/auth/dist/index.js", "var y = 2;\n")

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled: true,
			},
			"frontend/dist": {
				Enabled: false,
			},
			"services/auth/dist": {
				Enabled: false,
			},
		},
	}

	var buf bytes.Buffer
	if _, err := Scan(tempDir, cfg, &buf, ScanOptions{Tree: false}); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	output := buf.String()

	assertContains(t, output, "\x3cfile path=\"frontend/src/App.tsx\"")
	assertContains(t, output, "\x3cfile path=\"services/auth/src/index.ts\"")

	// node_modules excluded by frontend/.gitignore
	assertNotContains(t, output, "frontend/node_modules/pkg.js")
	// frontend/dist excluded by Dirs rule
	assertNotContains(t, output, "frontend/dist/bundle.js")
	// services/auth/dist excluded by Dirs rule
	assertNotContains(t, output, "services/auth/dist/index.js")
}

func TestScanSignaturesOnly(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_signatures")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	goCode := `package main

type User struct {
	ID   int
	Name string
}

func Add(a, b int) int {
	return a + b
}
`
	createFile(t, tempDir, "main.go", goCode)

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled:    true,
				Extensions: []string{"go"},
			},
		},
	}

	var buf bytes.Buffer
	if _, err := Scan(tempDir, cfg, &buf, ScanOptions{SignaturesOnly: true, Tree: false}); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	output := buf.String()

	assertContains(t, output, "\x3cfile path=\"main.go\" lines=\"1-")
	assertContains(t, output, "type User struct {")
	assertContains(t, output, "func Add(a, b int) int")
	// Ensure the implementation body statements were stripped out
	assertNotContains(t, output, "return a + b")
}

func TestScanSignaturesIncludesDocs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_sig_docs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	goCode := `package main

func Add(a, b int) int {
	return a + b
}
`
	mdContent := `# Documentation

This is project documentation.
It explains how everything works.
`
	createFile(t, tempDir, "main.go", goCode)
	createFile(t, tempDir, "README.md", mdContent)

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled:    true,
				Extensions: []string{"go", "md"},
			},
		},
	}

	var buf bytes.Buffer
	if _, err := Scan(tempDir, cfg, &buf, ScanOptions{SignaturesOnly: true, Tree: false}); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	output := buf.String()

	// main.go should have signatures stripped
	assertContains(t, output, "\x3cfile path=\"main.go\"")
	assertContains(t, output, "func Add(a, b int) int")
	assertNotContains(t, output, "return a + b")

	// README.md should be fully included
	assertContains(t, output, "\x3cfile path=\"README.md\"")
	assertContains(t, output, "# Documentation")
	assertContains(t, output, "This is project documentation.")
	assertContains(t, output, "It explains how everything works.")
}

func TestTreeAndMarkdownFormatting(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_formatting")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	createFile(t, tempDir, "main.go", "package main\n")

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled:    true,
				Extensions: []string{"go"},
			},
		},
	}

	var buf bytes.Buffer
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Format: "markdown", Tree: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	output := buf.String()

	assertContains(t, output, "PROJECT STRUCTURE:")
	assertContains(t, output, "### File: `main.go`")
	assertContains(t, output, "```go")
}

func TestGitignoredFolderExplicitlyEnabled(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_gitignored_enabled")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.MkdirAll(filepath.Join(tempDir, "local_docs"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "ignored_folder"), 0755)

	createFile(t, tempDir, "local_docs/notes.txt", "Private local project documentation\n")
	createFile(t, tempDir, "ignored_folder/skip.txt", "Should be ignored\n")
	createFile(t, tempDir, ".gitignore", "local_docs/\nignored_folder/\n")

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled: true,
			},
			"local_docs": {
				Enabled: true,
			},
			"ignored_folder": {
				Enabled: false,
			},
		},
	}

	var buf bytes.Buffer
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	output := buf.String()

	assertContains(t, output, "\x3cfile path=\"local_docs/notes.txt\"")
	assertContains(t, output, "Private local project documentation")

	assertNotContains(t, output, "ignored_folder/skip.txt")
}

func createFile(t *testing.T, dir, name, content string) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, output, substr string) {
	output = filepath.ToSlash(output)
	substr = filepath.ToSlash(substr)
	if !strings.Contains(output, substr) {
		t.Errorf("Expected output to contain '%s', but it didn't.", substr)
	}
}

func assertNotContains(t *testing.T, output, substr string) {
	output = filepath.ToSlash(output)
	substr = filepath.ToSlash(substr)
	if strings.Contains(output, substr) {
		t.Errorf("Expected output NOT to contain '%s', but it did.", substr)
	}
}

func TestScanGroupSignaturesAndFullCode(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_group_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.MkdirAll(filepath.Join(tempDir, "controllers"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "models"), 0755)

	ctrlCode := `package controllers

func GetUser() string {
	return "full_ctrl_body"
}
`
	modelCode := `package models

type User struct {
	ID string
}

func (u *User) Save() bool {
	return true
}
`
	createFile(t, tempDir, "controllers/user.go", ctrlCode)
	createFile(t, tempDir, "models/user.go", modelCode)

	cfg := &config.Config{
		OutputFile:       "codebase.txt",
		SignaturesGroups: []string{"db"},
		FullCodeGroups:   []string{"api"},
		Dirs: map[string]config.DirRule{
			".": {
				Enabled: true,
			},
			"controllers": {
				Enabled: true,
				Group:   "api",
			},
			"models": {
				Enabled: true,
				Group:   "db",
			},
		},
	}

	// 1. Scan with SignaturesOnly = false (like textify start)
	var buf bytes.Buffer
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false, SignaturesOnly: false})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	outStart := buf.String()

	// Controllers (api group) should be full code
	assertContains(t, outStart, "return \"full_ctrl_body\"")
	// Models (db group) should be signatures only
	assertContains(t, outStart, "type User struct {")
	assertContains(t, outStart, "func (u *User) Save() bool")
	assertNotContains(t, outStart, "return true")

	// 2. Scan with SignaturesOnly = true (like textify signatures)
	// Must be full signatures only regardless of what textify.yaml says
	buf.Reset()
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false, SignaturesOnly: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	outSigs := buf.String()

	assertContains(t, outSigs, "func GetUser() string")
	assertNotContains(t, outSigs, "return \"full_ctrl_body\"")
	assertContains(t, outSigs, "func (u *User) Save() bool")
	assertNotContains(t, outSigs, "return true")

	// 3. Scan with ForceFull = true (like textify start --full)
	buf.Reset()
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false, ForceFull: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	outFull := buf.String()

	assertContains(t, outFull, "return \"full_ctrl_body\"")
	assertContains(t, outFull, "return true")

	// 4. Scan with TargetGroups filter (like textify start --group api)
	buf.Reset()
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false, TargetGroups: []string{"api"}})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	outFiltered := buf.String()
	assertContains(t, outFiltered, "controllers/user.go")
	assertNotContains(t, outFiltered, "models/user.go")
}

func TestCalculateFolderTokens(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_tokens_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.MkdirAll(filepath.Join(tempDir, "big"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "small"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "ignored"), 0755)

	createFile(t, tempDir, "big/large.go", strings.Repeat("func LongCode() {\n\tprintln(1)\n}\n", 100))
	createFile(t, tempDir, "small/short.go", "package small\nfunc Mini() {}\n")
	createFile(t, tempDir, "ignored/skip.go", "package ignored\n")

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled: true,
			},
			"ignored": {
				Enabled: false,
			},
		},
	}

	stats, err := CalculateFolderTokens(tempDir, cfg, ScanOptions{})
	if err != nil {
		t.Fatalf("CalculateFolderTokens failed: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(stats))
	}

	if stats[0].Path != "big" {
		t.Errorf("expected largest folder to be 'big', got %q", stats[0].Path)
	}
	if stats[1].Path != "small" {
		t.Errorf("expected second folder to be 'small', got %q", stats[1].Path)
	}
	if stats[0].ApproxTokens <= stats[1].ApproxTokens {
		t.Errorf("expected 'big' tokens (%d) > 'small' tokens (%d)", stats[0].ApproxTokens, stats[1].ApproxTokens)
	}
}

func TestSubdirectoryInheritsExtensionsAndExcludes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_inherit_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.MkdirAll(filepath.Join(tempDir, "pkg", "api"), 0755)

	createFile(t, tempDir, "pkg/api/service.go", "package api\nfunc Run() {}\n")
	createFile(t, tempDir, "pkg/api/bad.go", "package api\n")
	createFile(t, tempDir, "pkg/api/temp.log", "some log\n")
	createFile(t, tempDir, "pkg/api/secret.env", "SECRET=1\n")

	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.env\n"), 0644)

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled:           true,
				Extensions:        []string{"go"},
				ExcludeExtensions: []string{"log"},
				Include:           []string{"secret.env"},
				Exclude:           []string{"bad.go"},
			},
			"pkg/api": {
				Enabled: true,
				Group:   "api",
				// Does not specify extensions/include/exclude -> must inherit from root!
			},
		},
	}

	var buf bytes.Buffer
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false, TargetGroups: []string{"api"}})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	out := buf.String()

	// Should include service.go and forced secret.env
	assertContains(t, out, "pkg/api/service.go")
	assertContains(t, out, "pkg/api/secret.env")

	// Should respect inherited exclude and exclude_extensions
	assertNotContains(t, out, "pkg/api/bad.go")
	assertNotContains(t, out, "pkg/api/temp.log")
}
