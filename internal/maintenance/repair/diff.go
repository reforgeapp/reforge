package repair

import (
	"fmt"
	"strings"

	"github.com/reforgeapp/reforge/internal/sandbox"
)

func SourceDiff(original map[string][]byte, patches []sandbox.Patch) string {
	var out strings.Builder
	lines := func(body []byte) []string {
		if len(body) == 0 {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	}
	write := func(prefix string, body []byte) {
		for _, line := range lines(body) {
			out.WriteString(prefix + line + "\n")
		}
		if len(body) > 0 && body[len(body)-1] != '\n' {
			out.WriteString("\\ No newline at end of file\n")
		}
	}
	for _, patch := range patches {
		before, exists := original[patch.Path]
		oldPath, newPath := "a/"+patch.Path, "b/"+patch.Path
		if !exists {
			oldPath = "/dev/null"
		}
		if patch.Delete {
			newPath = "/dev/null"
		}
		oldStart, newStart := 1, 1
		if len(before) == 0 {
			oldStart = 0
		}
		if len(patch.Content) == 0 {
			newStart = 0
		}
		fmt.Fprintf(&out, "--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", oldPath, newPath, oldStart, len(lines(before)), newStart, len(lines(patch.Content)))
		write("-", before)
		write("+", patch.Content)
	}
	return out.String()
}
