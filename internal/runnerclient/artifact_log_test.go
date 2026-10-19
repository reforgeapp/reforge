package runnerclient

import (
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/artifact"
	"github.com/reforgeapp/reforge/internal/maintenance/recipes"
	"github.com/reforgeapp/reforge/internal/maintenance/repair"
	"github.com/reforgeapp/reforge/internal/sandbox"
)

func TestRepairArtifactSanitizationLeavesGoJSONInterpretationRaw(t *testing.T) {
	raw := []byte("{\"Action\":\"pass\",\"Package\":\"example.test/pkg\",\"Test\":\"TestOK\"}\n")
	result := repair.Interpret(recipes.Command{ID: "go-test", ReportFormat: "json"}, sandbox.CommandResult{ExitCode: 0, Output: raw})
	artifactText := artifact.SanitizeTextLog(raw)
	if result.Cases["example.test/pkg/TestOK"] != "pass" || result.OutputSHA256 == "" {
		t.Fatalf("Go JSON output interpretation changed: %+v", result)
	}
	if string(raw) != "{\"Action\":\"pass\",\"Package\":\"example.test/pkg\",\"Test\":\"TestOK\"}\n" {
		t.Fatal("artifact normalization changed raw command output")
	}
	if !strings.Contains(string(artifactText), "TestOK") {
		t.Fatalf("safe Go JSON output was not retained: %s", artifactText)
	}
}
