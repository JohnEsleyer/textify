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
	// 1. Setup Filesystem
	tempDir, err := os.MkdirTemp("", "scanner_test_granular")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	/*
		Structure:
		/root
		  - normal.go         (Include: standard)
		  - annoying.go       (Exclude: explicitly via file exclude)
		  - data.json         (Include: explicitly via extension)
		  - temp.log          (Exclude: explicitly via extension exclude)
		  - secrets.env       (Include: explicitly via 'include' overrides gitignore)
		  - garbage.tmp       (Exclude: extension exclude)
		  - .gitignore        (ignores *.env)
	*/

	// Create Files
	createFile(t, tempDir, "normal.go", "package main")
	createFile(t, tempDir, "annoying.go", "package main")
	createFile(t, tempDir, "data.json", "{data:1}")
	createFile(t, tempDir, "temp.log", "error log")
	createFile(t, tempDir, "secrets.env", "API=123")
	createFile(t, tempDir, "garbage.tmp", "trash")

	// Create .gitignore
	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.env"), 0644)

	// 2. Setup Configuration
	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled:           true,
				Extensions:        []string{"go", "json"}, // Allow list
				ExcludeExtensions: []string{"log", "tmp"}, // Block list
				Include:           []string{"secrets.env"}, // Force Include
				Exclude:           []string{"annoying.go"}, // Force Exclude
			},
		},
	}

	// 3. Run Scan
	var buf bytes.Buffer
	_, err = Scan(tempDir, cfg, &buf, ScanOptions{Tree: false})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	output := buf.String()

	// 4. Assertions

	// Should Include
	assertContains(t, output, "FILE: normal.go")   // In extensions
	assertContains(t, output, "FILE: data.json")   // In extensions
	assertContains(t, output, "FILE: secrets.env") // Force included (overrides gitignore)

	// Should Exclude
	assertNotContains(t, output, "FILE: annoying.go") // Force excluded
	assertNotContains(t, output, "FILE: temp.log")    // Excluded extension
	assertNotContains(t, output, "FILE: garbage.tmp") // Excluded extension
}

func TestMixedDirectoryRules(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_mixed")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	os.Mkdir(filepath.Join(tempDir, "backend"), 0755)
	os.Mkdir(filepath.Join(tempDir, "frontend"), 0755)

	createFile(t, tempDir, "backend/main.go", "go")
	createFile(t, tempDir, "backend/test.js", "js") // should skip (not in backend ext)
	createFile(t, tempDir, "frontend/app.js", "js")
	createFile(t, tempDir, "frontend/style.css", "css") // should skip (explicit exclude in frontend)

	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Enabled: true,
			},
			"backend": {
				Enabled:    true,
				Extensions: []string{"go"}, // Only Go
			},
			"frontend": {
				Enabled:    true,
				Extensions: []string{"js", "css"},
				Exclude:    []string{"style.css"}, // Exclude specific file despite matching extension
			},
		},
	}

	var buf bytes.Buffer
	Scan(tempDir, cfg, &buf, ScanOptions{Tree: false})
	output := buf.String()

	assertContains(t, output, "FILE: backend/main.go")
	assertNotContains(t, output, "FILE: backend/test.js")
	assertContains(t, output, "FILE: frontend/app.js")
	assertNotContains(t, output, "FILE: frontend/style.css")
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

	createFile(t, tempDir, "frontend/src/App.tsx", "tsx")
	createFile(t, tempDir, "frontend/node_modules/pkg.js", "js")
	createFile(t, tempDir, "frontend/dist/bundle.js", "js")
	createFile(t, tempDir, "frontend/.gitignore", "node_modules\n")

	createFile(t, tempDir, "services/auth/src/index.ts", "ts")
	createFile(t, tempDir, "services/auth/dist/index.js", "js")

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

	assertContains(t, output, "FILE: frontend/src/App.tsx")
	assertContains(t, output, "FILE: services/auth/src/index.ts")

	// node_modules excluded by frontend/.gitignore
	assertNotContains(t, output, "FILE: frontend/node_modules/pkg.js")
	// frontend/dist excluded by Dirs rule
	assertNotContains(t, output, "FILE: frontend/dist/bundle.js")
	// services/auth/dist excluded by Dirs rule
	assertNotContains(t, output, "FILE: services/auth/dist/index.js")
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

	assertContains(t, output, "FILE: main.go")
	assertContains(t, output, "type User struct {")
	assertContains(t, output, "func Add(a, b int) int")
	// Ensure the body statements were stripped out
	assertNotContains(t, output, "return a + b")
}

func TestTreeAndMarkdownFormatting(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_formatting")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	createFile(t, tempDir, "main.go", "package main")

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

	// Create a folder that is in .gitignore
	os.MkdirAll(filepath.Join(tempDir, "local_docs"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "ignored_folder"), 0755)

	createFile(t, tempDir, "local_docs/notes.txt", "Private local project documentation")
	createFile(t, tempDir, "ignored_folder/skip.txt", "Should be ignored")
	createFile(t, tempDir, ".gitignore", "local_docs/\nignored_folder/\n")

	// In textify.yaml, explicitly enable local_docs
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

	// local_docs should be included because it is enabled in config
	assertContains(t, output, "FILE: local_docs/notes.txt")
	assertContains(t, output, "Private local project documentation")

	// ignored_folder should be excluded
	assertNotContains(t, output, "FILE: ignored_folder/skip.txt")
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
