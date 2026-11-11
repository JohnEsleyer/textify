package parser

import (
	"github.com/JohnEsleyer/textify/internal/targets/jvm"
)

// ParseJavaFile delegates Java source parsing to the jvm target package.
func ParseJavaFile(filePath string, src []byte) ([]Symbol, error) {
	return jvm.ParseJavaFile(filePath, src)
}

// ParseKotlinFile delegates Kotlin source parsing to the jvm target package.
func ParseKotlinFile(filePath string, src []byte) ([]Symbol, error) {
	return jvm.ParseKotlinFile(filePath, src)
}
