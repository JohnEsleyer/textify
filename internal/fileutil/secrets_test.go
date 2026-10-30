package fileutil

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	input := `
AWS_KEY="AKIA1234567890ABCDEF"
GITHUB_PAT="ghp_123456789012345678901234567890123456"
api_key="abcdef1234567890_secret_token"
normal_var = "hello world"
`
	cleaned, count := RedactSecrets(input)

	if count < 3 {
		t.Fatalf("Expected at least 3 redacted secrets, got %d", count)
	}

	// Verify the real secrets were removed
	if strings.Contains(cleaned, "AKIA1234567890ABCDEF") {
		t.Errorf("AWS Key was not redacted")
	}
	if strings.Contains(cleaned, "ghp_123456789012345678901234567890123456") {
		t.Errorf("GitHub token was not redacted")
	}
	if strings.Contains(cleaned, "abcdef1234567890_secret_token") {
		t.Errorf("Generic API key was not redacted")
	}

	// Verify replacement token exists
	if !strings.Contains(cleaned, "[REDACTED SECRET]") {
		t.Errorf("Cleaned output does not contain [REDACTED SECRET]")
	}

	// Verify harmless text is preserved
	if !strings.Contains(cleaned, `normal_var = "hello world"`) {
		t.Errorf("Non-secret code was modified unexpectedly")
	}
}
