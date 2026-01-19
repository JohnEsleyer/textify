package scanner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "textify_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create structure:
	// /src/main.go
	// /ignored.log
	// /.gitignore
	// /.env (should be ignored by default if ignored, but we will force include it)
	
	os.Mkdir(filepath.Join(tempDir, "src"), 0755)
	os.WriteFile(filepath.Join(tempDir, "src", "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tempDir, "ignored.log"), []byte("log data"), 0644)
	os.WriteFile(filepath.Join(tempDir, ".env"), []byte("SECRET=true"), 0644)
	os.WriteFile(filepath.Join(tempDir, "codebase.txt"), []byte("old scan"), 0644)

	// Create .gitignore
	gitignoreContent := `
*.log
.env
`
	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(gitignoreContent), 0644)

	// Setup Config
	// We want to force include .env despite it being in .gitignore
	outPath := filepath.Join(tempDir, "output.txt")
	config := Config{
		RootPath:        tempDir,
		OutputFilePath:  outPath,
		IncludePatterns: []string{".env"},
	}

	var buf bytes.Buffer
	err = Scan(config, &buf)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	output := buf.String()

	// Assertions
	if !strings.Contains(output, "FILE: src/main.go") {
		t.Error("Expected src/main.go to be included")
	}

	if strings.Contains(output, "FILE: ignored.log") {
		t.Error("Expected ignored.log to be excluded by .gitignore")
	}

	if !strings.Contains(output, "FILE: .env") {
		t.Error("Expected .env to be included due to force include flag")
	}

	if strings.Contains(output, "FILE: codebase.txt") {
		t.Error("Expected codebase.txt to be automatically excluded")
	}
}
