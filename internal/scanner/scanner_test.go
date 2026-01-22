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
	err = Scan(tempDir, cfg, &buf)
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
				Enabled: true,
				Extensions: []string{"js", "css"},
				Exclude:    []string{"style.css"}, // Exclude specific file despite matching extension
			},
		},
	}

	var buf bytes.Buffer
	Scan(tempDir, cfg, &buf)
	output := buf.String()

	assertContains(t, output, "FILE: backend/main.go")
	assertNotContains(t, output, "FILE: backend/test.js")
	assertContains(t, output, "FILE: frontend/app.js")
	assertNotContains(t, output, "FILE: frontend/style.css")
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