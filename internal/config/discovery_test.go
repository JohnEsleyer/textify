package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverWithSubfolderGitignore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "discovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create directories
	os.MkdirAll(filepath.Join(tempDir, "frontend", "src"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "frontend", "node_modules"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "frontend", "dist"), 0755)

	// Create files
	os.WriteFile(filepath.Join(tempDir, "frontend", "src", "App.tsx"), []byte("tsx"), 0644)
	os.WriteFile(filepath.Join(tempDir, "frontend", "node_modules", "index.js"), []byte("js"), 0644)
	os.WriteFile(filepath.Join(tempDir, "frontend", "dist", "bundle.js"), []byte("js"), 0644)

	// Create subfolder .gitignore in frontend
	os.WriteFile(filepath.Join(tempDir, "frontend", ".gitignore"), []byte("node_modules\ndist\n"), 0644)

	cfg, err := Discover(tempDir, nil)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Verify frontend is enabled
	if rule, ok := cfg.Dirs["frontend"]; !ok || !rule.Enabled {
		t.Errorf("Expected frontend directory rule to be enabled")
	}

	// Verify subfolders ignored by frontend/.gitignore are added with enabled: false
	if rule, ok := cfg.Dirs["frontend/node_modules"]; !ok || rule.Enabled {
		t.Errorf("Expected frontend/node_modules to be explicitly disabled in config")
	}

	if rule, ok := cfg.Dirs["frontend/dist"]; !ok || rule.Enabled {
		t.Errorf("Expected frontend/dist to be explicitly disabled in config")
	}
}
