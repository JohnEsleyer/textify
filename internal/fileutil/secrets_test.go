package fileutil

import (
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	input := `
AWS_KEY=AKIAIOSFODNN7EXAMPLE
GITHUB_PAT=ghp_0123456789abcdefghijklmnopqrstuvwxyz
api_key: "super_secret_token_123456"
normal_var = "hello world"
`
	cleaned, count := RedactSecrets(input)

	if count < 3 {
		t.Fatalf("Expected at least 3 redacted secrets, got %d", count)
	}

	if containsString(cleaned, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("AWS Key was not redacted")
	}

	if containsString(cleaned, "ghp_0123456789abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("GitHub token was not redacted")
	}

	if !containsString(cleaned, "normal_var = \"hello world\"") {
		t.Errorf("Non-secret code was modified unexpectedly")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && stringIndex(s, substr) >= 0))
}

func stringIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
