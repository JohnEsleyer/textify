package rust

import (
	"strings"
	"testing"

	"github.com/JohnEsleyer/textify/internal/targets"
)

const rustSample = `use std::sync::Arc;

#[derive(Debug, Clone)]
pub struct ServerConfig {
    pub host: String,
    pub port: u16,
}

pub enum Protocol {
    Http,
    Https,
}

pub trait Service {
    fn serve(&self) -> bool;
}

impl ServerConfig {
    pub fn new(host: String, port: u16) -> Self {
        Self { host, port }
    }

    pub fn start(&self) {
        println!("listening on {}:{}", self.host, self.port);
    }
}

pub fn run_server(config: ServerConfig) {
    config.start();
}
`

func TestRustTargetMatchAndExtractSignatures(t *testing.T) {
	target := &RustTarget{}
	if !target.Match("src/main.rs") {
		t.Errorf("RustTarget should match .rs files")
	}
	if target.Match("main.go") {
		t.Errorf("RustTarget should not match .go files")
	}

	sigs := target.ExtractSignatures("src/main.rs", rustSample)

	// Preserves use statements and attributes
	if !strings.Contains(sigs, "use std::sync::Arc;") {
		t.Errorf("signatures missing use statement: %s", sigs)
	}
	if !strings.Contains(sigs, "#[derive(Debug, Clone)]") {
		t.Errorf("signatures missing derive attribute: %s", sigs)
	}

	// Preserves full struct and enum definitions with fields/variants
	if !strings.Contains(sigs, "pub struct ServerConfig {") || !strings.Contains(sigs, "pub port: u16,") {
		t.Errorf("signatures missing struct fields: %s", sigs)
	}
	if !strings.Contains(sigs, "pub enum Protocol {") || !strings.Contains(sigs, "Http,") {
		t.Errorf("signatures missing enum variants: %s", sigs)
	}

	// Collapses function and method bodies into stubs
	if !strings.Contains(sigs, "pub fn new(host: String, port: u16) -> Self { /* ... */ }") {
		t.Errorf("method body not collapsed: %s", sigs)
	}
	if !strings.Contains(sigs, "pub fn run_server(config: ServerConfig) { /* ... */ }") {
		t.Errorf("function body not collapsed: %s", sigs)
	}
	if strings.Contains(sigs, `println!("listening`) {
		t.Errorf("function body statements should have been stripped: %s", sigs)
	}
}

func TestRustTargetParse(t *testing.T) {
	target := &RustTarget{}
	syms, err := target.Parse("src/main.rs", []byte(rustSample))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(syms) < 5 {
		t.Fatalf("expected at least 5 symbols, got %d", len(syms))
	}

	findSym := func(name string, kind targets.SymbolKind) *targets.Symbol {
		for _, s := range syms {
			if s.Name == name && s.Kind == kind {
				return &s
			}
		}
		return nil
	}

	cfgStruct := findSym("ServerConfig", targets.KindStruct)
	if cfgStruct == nil {
		t.Fatalf("ServerConfig struct not found")
	}

	protocolEnum := findSym("Protocol", targets.KindType)
	if protocolEnum == nil {
		t.Fatalf("Protocol enum not found")
	}

	serviceTrait := findSym("Service", targets.KindInterface)
	if serviceTrait == nil {
		t.Fatalf("Service trait not found")
	}

	newMethod := findSym("new", targets.KindMethod)
	if newMethod == nil || newMethod.Receiver != "ServerConfig" {
		t.Fatalf("new method on ServerConfig not found: %+v", newMethod)
	}

	runFunc := findSym("run_server", targets.KindFunction)
	if runFunc == nil {
		t.Fatalf("run_server function not found")
	}
	if len(runFunc.Calls) == 0 || runFunc.Calls[0] != "start" {
		t.Errorf("run_server should have call to start: %+v", runFunc.Calls)
	}
}

const rustMatchSample = `pub fn handle_action(action: &str) -> i32 {
    match action {
        "start" => {
            init_system();
            1
        }
        "stop" => {
            shutdown();
            0
        }
        _ => -1,
    }
}
`

func TestExtractRustCase(t *testing.T) {
	body, start, end, err := ExtractRustCase("src/lib.rs", rustMatchSample, "handle_action", "start")
	if err != nil {
		t.Fatalf("ExtractRustCase failed: %v", err)
	}

	if start != 3 || end != 6 {
		t.Errorf("expected lines 3-6, got %d-%d", start, end)
	}
	if !strings.Contains(body, "init_system()") {
		t.Errorf("body missing init_system(): %s", body)
	}

	bodyDefault, _, _, errDefault := ExtractRustCase("src/lib.rs", rustMatchSample, "handle_action", "_")
	if errDefault != nil {
		t.Fatalf("ExtractRustCase default failed: %v", errDefault)
	}
	if !strings.Contains(bodyDefault, "_ => -1") {
		t.Errorf("default arm missing: %s", bodyDefault)
	}
}
