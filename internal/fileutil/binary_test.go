package fileutil

import (
	"os"
	"testing"
)

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{"Empty File", []byte(""), false},
		{"Simple Text", []byte("Hello World"), false},
		{"UTF-8 Text", []byte("Hello 世界"), false},
		{"Binary NUL", []byte("Hello \x00 World"), true},
		{"Invalid UTF-8", []byte{0xff, 0xfe, 0xfd}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp("", "testfile")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.Write(tt.content); err != nil {
				t.Fatal(err)
			}
			tmpfile.Close()

			isBin, err := IsBinary(tmpfile.Name())
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if isBin != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, isBin)
			}
		})
	}
}
