package parser

import (
	"path/filepath"
	"strings"

	"github.com/JohnEsleyer/textify/internal/targets/golang"
)

// ParseGoFile delegates Go source parsing to the golang target package.
func ParseGoFile(filePath string, src []byte) ([]Symbol, error) {
	return golang.ParseGoFile(filePath, src)
}

// CleanCaseTarget strips prefix/suffix boilerplate from switch-case queries.
func CleanCaseTarget(s string) string {
	return golang.CleanCaseTarget(s)
}

// ExtractCase extracts the body, start line, and end line of a switch-case
// clause from a Go file (using the AST) or any other file (generic fallback).
func ExtractCase(filePath, content, fnName, caseTarget string) (string, int, int, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".go" {
		body, start, end, err := golang.ExtractGoCase(filePath, content, fnName, caseTarget)
		if err == nil {
			return body, start, end, nil
		}
	}
	return golang.ExtractGenericCase(content, fnName, caseTarget)
}

// ExtractGoCase delegates Go AST case clause extraction.
func ExtractGoCase(filePath, content, fnName, caseTarget string) (string, int, int, error) {
	return golang.ExtractGoCase(filePath, content, fnName, caseTarget)
}

// ExtractGenericCase delegates generic case clause extraction.
func ExtractGenericCase(content, fnName, caseTarget string) (string, int, int, error) {
	return golang.ExtractGenericCase(content, fnName, caseTarget)
}
