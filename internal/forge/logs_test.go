package forge

import (
	"strings"
	"testing"
)

func TestLogTailRedactsAndBounds(t *testing.T) {
	body := strings.Repeat("x", MaxLogBytes) + "\nAuthorization: Bearer abc123\ntoken=ghp_" + strings.Repeat("a", 36) + "\njs-yaml 5.2.1 HIGH\n"
	got := LogTail([]byte(body))
	if len(got) > MaxLogBytes || strings.Contains(got, "abc123") || strings.Contains(got, "ghp_") || !strings.Contains(got, "js-yaml 5.2.1 HIGH") {
		t.Fatalf("tail=%q", got[len(got)-200:])
	}
}
