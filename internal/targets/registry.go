package targets

import (
	"path/filepath"
	"strings"
)

// Target defines the contract for language/framework-specific processing.
type Target interface {
	// Name returns a short, stable identifier (e.g. "golang", "react").
	Name() string
	// Match reports whether this target handles the given file path.
	Match(filePath string) bool
	// ExtractSignatures renders a compact, line-numbered signature view.
	ExtractSignatures(filePath string, content string) string
	// Parse extracts structured symbols from the source.
	Parse(filePath string, src []byte) ([]Symbol, error)
}

var registeredTargets []Target

// RegisterTarget adds a target processor to the global registry.
// Targets register themselves from their package init function so the
// scanner, parser, and CLI all share the same dispatch table.
func RegisterTarget(t Target) {
	registeredTargets = append(registeredTargets, t)
}

// FindTarget locates the matching target processor for a given file path.
func FindTarget(filePath string) Target {
	for _, t := range registeredTargets {
		if t.Match(filePath) {
			return t
		}
	}
	return nil
}

// ExtractSignatures dispatches signature extraction to the appropriate target,
// falling back to the generic line scanner when no specialized target matches.
func ExtractSignatures(filePath string, content string) string {
	if t := FindTarget(filePath); t != nil {
		return t.ExtractSignatures(filePath, content)
	}
	return ExtractGenericSignatures(content, strings.ToLower(filepath.Ext(filePath)))
}

// ParseFile parses symbols via the matching target processor.
func ParseFile(filePath string, src []byte) ([]Symbol, error) {
	if t := FindTarget(filePath); t != nil {
		return t.Parse(filePath, src)
	}
	return nil, nil
}
