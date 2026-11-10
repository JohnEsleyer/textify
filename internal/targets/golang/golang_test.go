package golang

import (
	"strings"
	"testing"

	"github.com/JohnEsleyer/textify/internal/targets"
)

const goSample = `package main

type Config struct {
	Port int
	Host string
}

func Connect(c Config) bool {
	return true
}
`

func TestGoTarget(t *testing.T) {
	target := &GoTarget{}
	if !target.Match("cmd/app/main.go") {
		t.Errorf("expected GoTarget to match .go file")
	}
	if target.Match("src/App.tsx") {
		t.Errorf("GoTarget should not match .tsx file")
	}

	syms, err := target.Parse("main.go", []byte(goSample))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(syms) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(syms))
	}

	if syms[0].Kind != targets.KindStruct || syms[0].Name != "Config" {
		t.Errorf("unexpected symbol 0: %+v", syms[0])
	}
	if syms[1].Kind != targets.KindFunction || syms[1].Name != "Connect" {
		t.Errorf("unexpected symbol 1: %+v", syms[1])
	}

	sigs := target.ExtractSignatures("main.go", goSample)
	if !strings.Contains(sigs, "type Config struct") {
		t.Errorf("signatures missing Config: %s", sigs)
	}
	if !strings.Contains(sigs, "func Connect(c Config) bool") {
		t.Errorf("signatures missing Connect: %s", sigs)
	}
	if strings.Contains(sigs, "return true") {
		t.Errorf("signatures should not contain function body: %s", sigs)
	}
}

const goCaseSample = `package sample

func Handle(kind string) {
	switch kind {
	case "create":
		doCreate()
	case "update":
		doUpdate()
	default:
		doDefault()
	}
}
`

func TestExtractGoCase(t *testing.T) {
	body, start, end, err := ExtractGoCase("sample.go", goCaseSample, "Handle", "update")
	if err != nil {
		t.Fatalf("ExtractGoCase failed: %v", err)
	}
	if start != 7 || end != 8 {
		t.Errorf("case lines = %d-%d, want 7-8", start, end)
	}
	if !strings.Contains(body, "doUpdate()") || strings.Contains(body, "doCreate()") {
		t.Errorf("unexpected case body: %q", body)
	}
}
