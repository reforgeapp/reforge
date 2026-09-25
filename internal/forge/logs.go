package forge

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxLogBytes = 24 << 10

type CheckLogReader interface {
	ReadCheckLog(context.Context, RepoRef, string) (string, error)
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,})\b`),
	regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(authorization:\s*(bearer|basic|token)\s+)\S+`),
	regexp.MustCompile(`(?i)((password|passwd|secret|token|api[_-]?key)\s*[:=]\s*)\S+`),
}

func LogTail(body []byte) string {
	if len(body) > MaxLogBytes {
		body = body[len(body)-MaxLogBytes:]
	}
	text := strings.ToValidUTF8(string(body), "")
	for !utf8.ValidString(text) {
		text = text[1:]
	}
	for i, pattern := range secretPatterns {
		if i >= 2 {
			text = pattern.ReplaceAllString(text, "${1}[redacted]")
		} else {
			text = pattern.ReplaceAllString(text, "[redacted]")
		}
	}
	return text
}
