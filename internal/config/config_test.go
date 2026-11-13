package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestYamlErrorGuidanceForIndentedList(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "textify.yaml")

	badYaml := `output_file: codebase.txt
dirs:
    internal:
        enabled: true
            - .
            - /config
            - /fileutil
`
	if err := os.WriteFile(filePath, []byte(badYaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(filePath)
	if err == nil {
		t.Fatalf("expected error on invalid yaml indentation, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "include:") {
		t.Errorf("expected error message to suggest 'include:', got:\n%s", errMsg)
	}
	if !strings.Contains(errMsg, "enabled") {
		t.Errorf("expected error message to mention 'enabled', got:\n%s", errMsg)
	}
}

func TestFlexibleScalarsAndLists(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "textify.yaml")

	yamlData := `output_file: codebase.txt
dirs:
    /cmd:
        enabled: "true"
        extensions: go
        include: /main.go
    /internal:
        enabled:
            - config
            - fileutil
        exclude: obsolete.go
`
	if err := os.WriteFile(filePath, []byte(yamlData), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Slashes in dir keys should be normalized
	if _, ok := cfg.Dirs["cmd"]; !ok {
		t.Errorf("expected cmd key to be normalized from /cmd")
	}
	if _, ok := cfg.Dirs["internal"]; !ok {
		t.Errorf("expected internal key to be normalized from /internal")
	}

	cmdRule := cfg.Dirs["cmd"]
	if len(cmdRule.Extensions) != 1 || cmdRule.Extensions[0] != "go" {
		t.Errorf("expected scalar extension 'go' to be parsed: %v", cmdRule.Extensions)
	}
	if len(cmdRule.Include) != 1 || cmdRule.Include[0] != "main.go" {
		t.Errorf("expected include '/main.go' to be normalized to 'main.go': %v", cmdRule.Include)
	}

	internalRule := cfg.Dirs["internal"]
	if !internalRule.Enabled {
		t.Errorf("internal should be enabled")
	}
	if len(internalRule.Include) != 2 || internalRule.Include[0] != "config" || internalRule.Include[1] != "fileutil" {
		t.Errorf("expected enabled sequence to populate include: %v", internalRule.Include)
	}
	if len(internalRule.Exclude) != 1 || internalRule.Exclude[0] != "obsolete.go" {
		t.Errorf("expected exclude 'obsolete.go': %v", internalRule.Exclude)
	}
}
