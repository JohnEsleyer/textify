package scanner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"github.com/JohnEsleyer/textify/internal/config"
)

func TestScanWithConfig(t *testing.T) {
	// 1. Setup Filesystem
	tempDir, err := os.MkdirTemp("", "scanner_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	/*
		Structure:
		/root
		  - main.go       (Should include: matches root extension)
		  - readme.md     (Should include: matches root extension)
		  - script.py     (Should skip: not in root extensions)
		  - ignored.log   (Should skip: .gitignore)
		  - secret.env    (Should include: Force Include)
		  /api
		    - api.go      (Should include: Inherits root extensions)
		  /frontend
		    - app.ts      (Should include: frontend has specific rule for 'ts')
		    - util.go     (Should skip: frontend rule replaces parent, only 'ts' allowed)
	*/

	// Create Dirs
	os.Mkdir(filepath.Join(tempDir, "api"), 0755)
	os.Mkdir(filepath.Join(tempDir, "frontend"), 0755)

	// Create Files
	createFile(t, tempDir, "main.go", "package main")
	createFile(t, tempDir, "readme.md", "# Readme")
	createFile(t, tempDir, "script.py", "print('hello')")
	createFile(t, tempDir, "ignored.log", "log data")
	createFile(t, tempDir, "secret.env", "API_KEY=123")
	createFile(t, tempDir, "api/api.go", "package api")
	createFile(t, tempDir, "frontend/app.ts", "console.log('hi')")
	createFile(t, tempDir, "frontend/util.go", "package util") // Should be skipped by frontend rule

	// Create .gitignore
	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.log\n*.env"), 0644)

	// 2. Setup Configuration
	cfg := &config.Config{
		OutputFile: "codebase.txt",
		Dirs: map[string]config.DirRule{
			".": {
				Extensions: []string{"go", "md"},
				Include:    []string{"secret.env"}, // Force include despite gitignore
			},
			"frontend": {
				Extensions: []string{"ts"}, // Override: only TS files here
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

	// Positive Assertions (Should exist)
	assertContains(t, output, "FILE: main.go")
	assertContains(t, output, "FILE: readme.md")
	assertContains(t, output, "FILE: secret.env")
	assertContains(t, output, "FILE: api/api.go")     // Inherited 'go' from root
	assertContains(t, output, "FILE: frontend/app.ts") // specific rule

	// Negative Assertions (Should NOT exist)
	assertNotContains(t, output, "FILE: script.py")     // 'py' not in root extensions
	assertNotContains(t, output, "FILE: ignored.log")   // .gitignore
	assertNotContains(t, output, "FILE: frontend/util.go") // 'go' not allowed in frontend rule
}

func createFile(t *testing.T, dir, name, content string) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, output, substr string) {
	// Normalize path separators for Windows compatibility in tests
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
