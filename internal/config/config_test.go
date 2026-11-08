package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.OutputFile != "codebase.txt" {
		t.Errorf("Expected default output codebase.txt, got %s", cfg.OutputFile)
	}

	if cfg.Dirs == nil {
		t.Error("Default config should have an initialized Dirs map")
	}
}

func TestLoadAndSave(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "test_config.yaml")

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

func TestGroupConfiguration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_group_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "textify.yaml")

	yamlData := `output_file: codebase.txt
signatures_groups:
  - models
  - helpers
full_code_groups:
  - core
groups:
  extra_sigs: signatures
  extra_full: full
dirs:
  cmd:
    enabled: true
    group: core
  internal/models:
    enabled: true
    group: models
`

	if err := os.WriteFile(filePath, []byte(yamlData), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(filePath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Dirs["cmd"].Group != "core" {
		t.Errorf("expected cmd group 'core', got %q", cfg.Dirs["cmd"].Group)
	}
	if cfg.Dirs["internal/models"].Group != "models" {
		t.Errorf("expected internal/models group 'models', got %q", cfg.Dirs["internal/models"].Group)
	}

	if !cfg.IsGroupSignaturesOnly("models") {
		t.Errorf("expected 'models' group to be signatures only")
	}
	if !cfg.IsGroupSignaturesOnly("helpers") {
		t.Errorf("expected 'helpers' group to be signatures only")
	}
	if !cfg.IsGroupSignaturesOnly("extra_sigs") {
		t.Errorf("expected 'extra_sigs' group to be signatures only via groups map")
	}

	if !cfg.IsGroupFullCode("core") {
		t.Errorf("expected 'core' group to be full code")
	}
	if !cfg.IsGroupFullCode("extra_full") {
		t.Errorf("expected 'extra_full' group to be full code via groups map")
	}

	if cfg.IsGroupSignaturesOnly("unknown") {
		t.Errorf("'unknown' group should not be signatures only")
	}
	if cfg.IsGroupFullCode("unknown") {
		t.Errorf("'unknown' group should not be full code")
	}
}
