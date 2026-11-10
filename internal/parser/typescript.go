package parser

import (
	"github.com/JohnEsleyer/textify/internal/targets/react"
)

// ParseTypeScriptFile delegates TypeScript, JavaScript, JSX, and React parsing
// to the react target package.
func ParseTypeScriptFile(filePath string, src []byte) ([]Symbol, error) {
	return react.ParseTypeScriptFile(filePath, src)
}
