package repair

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestBrowserArtifactsStayWithinCommandOutputAndBindCheck(t *testing.T) {
	var screenshot bytes.Buffer
	png.Encode(&screenshot, image.NewRGBA(image.Rect(0, 0, 20, 10)))
	file := ".reforge/browser/js-01/checkout.png"
	runtime := &retryRuntime{files: map[string][]byte{file: screenshot.Bytes()}, patches: map[string][]sandbox.Patch{}}
	uploads := 0
	engine := Engine{Runtime: runtime, Artifact: func(_ context.Context, name string, body []byte) (string, string, error) {
		uploads++
		if name != "candidate-2-js-01-checkout.png" || !bytes.Equal(body, screenshot.Bytes()) {
			t.Fatal("attachment upload changed")
		}
		return "artifact", hashBytes(body), nil
	}}
	body := `{"suites":[{"specs":[{"title":"Checkout mobile","file":"checkout.spec.ts","tests":[{"expectedStatus":"passed","results":[{"status":"passed","attachments":[{"path":"/workspace/.reforge/browser/js-01/checkout.png","contentType":"image/png"},{"path":"/workspace/private.png","contentType":"image/png"},{"path":"/workspace/.reforge/browser/js-01/../../private.png","contentType":"image/png"}]}]}]}]}],"stats":{"expected":1}}`
	report := Report{}
	if err := engine.browserArtifacts(context.Background(), sandbox.Workspace{ID: "workspace"}, recipes.Command{ID: "js-01", Directory: ".", ReportFormat: "playwright"}, sandbox.CommandResult{Output: []byte(body)}, "candidate-2", &report); err != nil || uploads != 1 || len(report.BehaviourArtifacts) != 1 {
		t.Fatalf("bounded attachments=%+v uploads=%d err=%v", report, uploads, err)
	}
	plan, _ := testPlan(t)
	plan.Owner = true
	plan.Recipe.MinProof = recipes.ProofTests
	plan.Digest = planDigest(plan)
	command := plan.Recipe.Commands[0]
	for _, output := range []string{"", "ok 1 - checkout # SKIP\n", "ok 1 - empty.test.js\n"} {
		if Verified(plan, nil, []CheckResult{Interpret(command, sandbox.CommandResult{Output: []byte(output)})}) {
			t.Fatal("empty or skipped owner checks accepted")
		}
	}
	if behaviourStage("reported-defect-before-2") != "reproduction" || behaviourStage("ci-candidate-3") != "candidate" || !strings.HasPrefix(report.BehaviourArtifacts[0].Name, "candidate-2-") {
		t.Fatal("evidence stage lost")
	}
}
