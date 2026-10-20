package artifact

import (
	"strings"
	"testing"
)

func TestSanitizeTextLogRemovesTerminalControlsAndSensitiveLines(t *testing.T) {
	input := []byte("\x1b[32m✓ src/unit.test.ts (3 tests)\x1b[39m\r\n\x1b[2K PASS src/unit.test.ts\nAuthorization: Bearer abcdef0123456789\nkept output\n")
	got := SanitizeTextLog(input)
	text := string(got)
	if strings.ContainsRune(text, '\x1b') || strings.Contains(text, "abcdef0123456789") || strings.Contains(text, "Authorization") || !strings.Contains(text, "[redacted]") {
		t.Fatalf("terminal controls or credential line remain: %q", text)
	}
	if !strings.Contains(text, "✓ src/unit.test.ts (3 tests)") || !strings.Contains(text, "PASS src/unit.test.ts") || !strings.Contains(text, "kept output") {
		t.Fatalf("safe test output was lost: %q", text)
	}
	if !validContent("text/plain", got) {
		t.Fatalf("sanitized log rejected by artifact validator: %q", text)
	}
}

func TestSanitizeTextLogRemovesCompletePrivateKeyBlock(t *testing.T) {
	input := []byte("before\n-----BEGIN RSA PRIVATE KEY-----\nprivate-body-line\n-----END RSA PRIVATE KEY-----\nafter\n")
	got := string(SanitizeTextLog(input))
	for _, secret := range []string{"BEGIN RSA PRIVATE KEY", "private-body-line", "END RSA PRIVATE KEY"} {
		if strings.Contains(got, secret) {
			t.Fatalf("private key block was partially retained: %q", got)
		}
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") || strings.Count(got, "[redacted]") != 1 || !validContent("text/plain", []byte(got)) {
		t.Fatalf("safe log lines missing or invalid: %q", got)
	}
}

func TestSanitizeTextLogStaysWithinArtifactLimit(t *testing.T) {
	input := strings.Repeat("API_KEY=secret-value\n", int(MaxSize/21)+1)
	got := SanitizeTextLog([]byte(input))
	if len(got) > int(MaxSize) || !validContent("text/plain", got) || !strings.Contains(string(got), "[redacted]") {
		t.Fatalf("sanitized log exceeds artifact bounds: %d", len(got))
	}
}
