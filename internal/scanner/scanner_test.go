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
