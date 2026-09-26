package artifact

import (
	"regexp"
	"strings"
	"unicode"
)

var privateKeyStart = regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`)
var privateKeyEnd = regexp.MustCompile(`(?i)-----END [A-Z ]*PRIVATE KEY-----`)

const redactedLogLine = "[redacted]"

func SanitizeTextLog(data []byte) []byte {
	text := strings.ToValidUTF8(string(stripTerminalSequences(data)), "")
	var clean strings.Builder
	clean.Grow(len(text))
	for _, r := range text {
		if r == '\r' {
			clean.WriteByte('\n')
			continue
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		clean.WriteRune(r)
	}

	var out strings.Builder
	writeLine := func(line string) bool {
		if out.Len() >= int(MaxSize) {
			return false
		}
		remaining := int(MaxSize) - out.Len()
		if len(line) >= remaining {
			line = strings.ToValidUTF8(line[:remaining], "")
			out.WriteString(line)
			return false
		}
		out.WriteString(line)
		out.WriteByte('\n')
		return true
	}
	insidePrivateKey := false
	for _, line := range strings.Split(clean.String(), "\n") {
		if insidePrivateKey {
			if privateKeyEnd.MatchString(line) {
				insidePrivateKey = false
			}
			continue
		}
		if privateKeyStart.MatchString(line) {
			insidePrivateKey = !privateKeyEnd.MatchString(line)
			if !writeLine(redactedLogLine) {
				break
			}
			continue
		}
		if sensitive.MatchString(line) {
			if !writeLine(redactedLogLine) {
				break
			}
			continue
		}
		if !writeLine(line) {
			break
		}
	}
	return []byte(strings.TrimSuffix(out.String(), "\n"))
}

func stripTerminalSequences(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] != 0x1b {
			out = append(out, data[i])
			i++
			continue
		}
		i++
		if i == len(data) {
			break
		}
		switch data[i] {
		case '[':
			i++
			for i < len(data) {
				c := data[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
		case ']':
			i++
			for i < len(data) {
				if data[i] == 0x07 {
					i++
					break
				}
				if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default:
			i++
		}
	}
	return out
}
