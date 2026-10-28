package fileutil

import (
	"regexp"
)

var secretPatterns = []*regexp.Regexp{
	// Private Keys
	regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z0-9_-]+ )?PRIVATE KEY-----.*?-----END (?:[A-Z0-9_-]+ )?PRIVATE KEY-----`),
	// AWS Access Key ID
	regexp.MustCompile(`\b(AKIA[0-9A-Z]{16})\b`),
	// GitHub Personal Access Tokens
	regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9_]{36,255})\b`),
	// Generic API keys and tokens in key-value pairs
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|auth[_-]?token)\s*[:=]\s*["']([a-zA-Z0-9_\-]{16,})["']`),
}

// RedactSecrets scans text for known sensitive patterns and replaces them.
// Returns the sanitized string and the number of secrets redacted.
func RedactSecrets(content string) (string, int) {
	redactedCount := 0
	result := content

	for _, pat := range secretPatterns {
		matches := pat.FindAllStringIndex(result, -1)
		if len(matches) > 0 {
			redactedCount += len(matches)
			result = pat.ReplaceAllString(result, "[REDACTED SECRET]")
		}
	}

	return result, redactedCount
}
