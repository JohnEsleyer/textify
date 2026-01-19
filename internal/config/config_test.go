package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cwd := "/tmp/test"
	cfg := DefaultConfig(cwd)

	if cfg.OutputFile != "codebase.txt" {
		t.Errorf("Expected default output codebase.txt, got %s", cfg.OutputFile)
	}

	if _, ok := cfg.Dirs["."]; !ok {
		t.Error("Default config should have a root '.' rule")
	}
}

func TestLoadAndSave(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "test_config.toml")

	// Create a dummy config
	originalCfg := Config{
		OutputFile: "output.txt",
		Dirs: map[string]DirRule{
			".": {
				Extensions: []string{"go", "md"},
				Include:    []string{".env"},
			},
			"frontend": {
				Extensions: []string{"ts", "tsx"},
			},
		},
	}

	// 1. Test Save
	if err := originalCfg.Save(filePath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// 2. Test Load
	loadedCfg, err := Load(filePath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 3. Compare
	if loadedCfg.OutputFile != originalCfg.OutputFile {
		t.Errorf("Expected output %s, got %s", originalCfg.OutputFile, loadedCfg.OutputFile)
	}

	if !reflect.DeepEqual(loadedCfg.Dirs, originalCfg.Dirs) {
		t.Errorf("Loaded dirs do not match saved dirs.\nExpected: %+v\nGot: %+v", originalCfg.Dirs, loadedCfg.Dirs)
	}
}
