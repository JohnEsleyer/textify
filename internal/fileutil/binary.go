package fileutil

import (
	"io"
	"os"
	"unicode/utf8"
)

// IsBinary checks if a file is binary by reading its first 512 bytes.
// It checks for NUL bytes or invalid UTF-8 sequences.
func IsBinary(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false, err
	}

	// Empty files are treated as text
	if n == 0 {
		return false, nil
	}

	content := buffer[:n]

	// Check for NUL bytes, common in binary formats
	for _, b := range content {
		if b == 0 {
			return true, nil
		}
	}

	// Check for valid UTF-8
	if !utf8.Valid(content) {
		return true, nil
	}

	return false, nil
}
