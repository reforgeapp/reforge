package recipes

import (
	"strings"
	"testing"
)

func TestOwnerRunsIndependentUnitAndBrowserChecks(t *testing.T) {
	files := map[string][]byte{"package.json": []byte(`{"scripts":{"build":"vite build","test:unit":"vitest run","test:e2e":"playwright test"}}`)}
	recipe, err := BuildOwner("javascript", files)
	if err != nil {
		t.Fatal(err)
	}
	unit, browser := false, false
	for _, command := range recipe.Commands {
		if command.ReportFormat == "jest" {
			unit = true
			if !strings.Contains(strings.Join(command.Args, " "), "--reporter=json") {
				t.Fatal("unit cases not counted")
			}
		}
		if command.ReportFormat == "playwright" {
			browser = true
			joined := strings.Join(command.Args, " ")
			for _, arg := range []string{"test:e2e", "--workers=1", "--retries=0", "--max-failures=0", "--forbid-only", "--trace=on", "--global-timeout=120000"} {
				if !strings.Contains(joined, arg) {
					t.Fatalf("browser bound missing %s: %v", arg, command.Args)
				}
			}
		}
	}
	if !unit || !browser {
		t.Fatalf("existing checks omitted: %+v", recipe.Commands)
	}
	unsupported, err := BuildOwner("javascript", map[string][]byte{"package.json": []byte(`{"scripts":{"test":"node scripts/unknown-runner.js"}}`)})
	if err != nil || len(unsupported.Commands) != 1 || !strings.Contains(strings.Join(unsupported.Commands[0].Args, " "), "run test") {
		t.Fatalf("unknown original test command omitted: %+v %v", unsupported, err)
	}
}
