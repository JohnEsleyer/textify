package parser

import (
	"github.com/JohnEsleyer/textify/internal/targets/rust"
)

// ParseRustFile delegates Rust source parsing to the rust target package.
func ParseRustFile(filePath string, src []byte) ([]Symbol, error) {
	return rust.ParseRustFile(filePath, src)
}
