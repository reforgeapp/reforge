package repair

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/artifact"
	"github.com/reforgeapp/reforge/internal/maintenance/recipes"
	"github.com/reforgeapp/reforge/internal/sandbox"
)

func testPlan(t *testing.T) (Plan, map[string][]byte) {
	t.Helper()
	files := map[string][]byte{"value.js": []byte("exports.add = (a,b) => a-b"), "value.test.js": []byte("const test = require('node:test'); const assert = require('node:assert'); test('adds',()=>assert.equal(require('./value').add(2,3),5))"), "package.json": []byte(`{"scripts":{"test":"node --test"}}`)}
	p, err := Freeze("javascript", "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 40), strings.Repeat("c", 40), files, []string{"private/**"})
	if err != nil {
		t.Fatal(err)
	}
	return p, files
}
func TestPatchCannotChangeValidationOrSensitivePaths(t *testing.T) {
	p, files := testPlan(t)
	for _, patch := range []sandbox.Patch{
		{Path: "value.test.js", Content: []byte("pass")}, {Path: "package.json", Content: []byte("{}")}, {Path: "value.js", Delete: true}, {Path: "scripts/check.js", Content: []byte("pass")}, {Path: "private/source.js", Content: []byte("pass")}, {Path: "value.js", Content: []byte("process.exit(0)")}, {Path: ".git/config", Content: []byte("x")}, {Path: "value.js", Content: []byte(strings.Repeat("x", p.Recipe.MaxPatchBytes+1))},
	} {
		if CheckPatch(p, files, []sandbox.Patch{patch}) == nil {
			t.Fatalf("unsafe patch accepted: %s", patch.Path)
		}
	}
	if err := CheckPatch(p, files, []sandbox.Patch{{Path: "latest.js", Content: []byte("exports.add = (a,b) => a+b")}}); err != nil {
		t.Fatal(err)
	}
	files["value.test.js"] = []byte("changed")
	if Protected(p, files) {
		t.Fatal("changed test accepted")
	}
	p.Recipe.Commands[0].Args = []string{"true"}
	if p.Valid() {
		t.Fatal("altered plan accepted")
	}
}
func TestValidationRejectsNoopSkippedLostAndUnreproducedChecks(t *testing.T) {
	p, _ := testPlan(t)
	command := p.Recipe.Commands[0]
	failed := Interpret(command, sandbox.CommandResult{ExitCode: 1, Output: []byte("not ok 1 - adds\n")})
	passed := Interpret(command, sandbox.CommandResult{ExitCode: 0, Output: []byte("ok 1 - adds\n")})
	if !Reproduced([]CheckResult{failed}) || !Verified(p, []CheckResult{failed}, []CheckResult{passed}) {
		t.Fatal("real failure/pass rejected")
	}
	for _, raw := range []sandbox.CommandResult{
		{Output: []byte("done")}, {Output: []byte("ok 1 - adds # SKIP disabled\n")}, {Output: []byte("ok 1 - different\n")}, {Output: []byte("ok 1 - adds\n"), Truncated: true}, {Output: []byte("ok 1 - adds\n"), TimedOut: true}, {ExitCode: 127, Output: []byte("not found")},
	} {
		out := Interpret(command, raw)
		if Verified(p, []CheckResult{failed}, []CheckResult{out}) {
			t.Fatalf("invalid validation accepted: %+v", raw)
		}
	}
	if Reproduced([]CheckResult{passed}) || Reproduced([]CheckResult{Interpret(command, sandbox.CommandResult{ExitCode: 127})}) {
		t.Fatal("unreproduced environment accepted")
	}
}
func TestGoAndPythonReportsRetainCaseIdentity(t *testing.T) {
	checks := []struct{ format, body, key string }{
		{"json", "{\"Action\":\"pass\",\"Package\":\"fixture\",\"Test\":\"TestAdd\"}\n", "fixture/TestAdd"},
		{"text", "test_add (test_value.ValueTest.test_add) ... ok\n", "test_add (test_value.ValueTest.test_add)"},
	}
	for _, check := range checks {
		r := Interpret(recipes.Command{ID: "check", ReportFormat: check.format}, sandbox.CommandResult{Output: []byte(check.body)})
		if !r.Complete || r.Cases[check.key] != "pass" {
			t.Fatalf("case lost: %+v", r)
		}
	}
}

type ansiCommandRuntime struct{ retryRuntime }

func (r *ansiCommandRuntime) ExecuteBoundedCommand(context.Context, sandbox.Workspace, sandbox.Command) (sandbox.CommandResult, error) {
	return sandbox.CommandResult{ExitCode: 0, Output: []byte("\x1b[32m RUN v3.0.0 \x1b[0m\nok 1 - math.test.js > adds\n")}, nil
}

func TestANSIJavaScriptCheckBindsUploadedArtifactDigest(t *testing.T) {
	plan, files := testPlan(t)
	raw := []byte("\x1b[32m RUN v3.0.0 \x1b[0m\nok 1 - math.test.js > adds\n")
	runtime := &ansiCommandRuntime{retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}}
	var uploaded []byte
	engine := Engine{Runtime: runtime, JobID: "ansi-test", AttemptID: "attempt-1", Trust: "fixture", Artifact: func(_ context.Context, _ string, data []byte) (string, string, error) {
		uploaded = artifact.SanitizeTextLog(data)
		digest := hashBytes(uploaded)
		return "check-log", digest, nil
	}}
	report := Report{}
	checks, err := engine.validate(context.Background(), plan, plan.BaselineSHA, nil, "baseline", &report)
	if err != nil || len(checks) != 1 {
		t.Fatalf("validation checks=%+v err=%v", checks, err)
	}
	if checks[0].Cases["1:math.test.js > adds"] != "pass" {
		t.Fatalf("raw ANSI output lost TAP status: %+v", checks[0])
	}
	if checks[0].OutputSHA256 != hashBytes(uploaded) || checks[0].OutputSHA256 == hashBytes(raw) || len(report.Artifacts) != 1 || report.Artifacts[0] != "check-log" {
		t.Fatalf("report digest does not bind sanitized upload: check=%+v artifacts=%v", checks[0], report.Artifacts)
	}
}

func TestCheckSummaryKeepsEveryCommandBeyondLargeCaseOutput(t *testing.T) {
	goCases := make(map[string]string, 1200)
	for i := 0; i < 1200; i++ {
		goCases[fmt.Sprintf("Test%04d-%s", i, strings.Repeat("x", 40))] = "pass"
	}
	goCases["TestSkippedA"], goCases["TestSkippedB"] = "skip", "skip"
	baseline := []CheckResult{
		{CommandID: "go-01", ExitCode: 0, Complete: true, Cases: goCases, Excerpt: strings.Repeat("pass output ", 1000)},
		{CommandID: "js-01", ExitCode: 1, Complete: true, Cases: map[string]string{"frontend.test.ts > expected result": "fail"}, Reason: "test failed", Excerpt: "expected 2 to equal 3"},
	}
	full, err := json.Marshal(baseline)
	if err != nil || len(full) <= 32<<10 {
		t.Fatalf("fixture must exceed old prompt bound: bytes=%d err=%v", len(full), err)
	}
	baselineJSON := summarizeChecks(baseline)
	if len(baselineJSON) >= 32<<10 {
		t.Fatalf("compact baseline too large: %d bytes", len(baselineJSON))
	}
	var summarized []checkSummary
	if err := json.Unmarshal(baselineJSON, &summarized); err != nil {
		t.Fatalf("summary is not valid JSON: %v", err)
	}
	if len(summarized) != 2 || summarized[0].CommandID != "go-01" || summarized[0].Pass != 1200 || summarized[0].Skip != 2 || summarized[0].Fail != 0 || summarized[0].ExitCode != 0 || !summarized[0].Complete || summarized[0].Excerpt != "" {
		t.Fatalf("large Go cases obscured command summary: %+v", summarized)
	}
	if summarized[1].CommandID != "js-01" || summarized[1].Fail != 1 || len(summarized[1].FailureCases) != 1 || summarized[1].Excerpt != "expected 2 to equal 3" {
		t.Fatalf("frontend failure omitted from summary: %+v", summarized[1])
	}
	for i := 0; i < 9; i++ {
		baseline[1].Cases[fmt.Sprintf("failure-%d", i)] = "fail"
	}
	summarized = nil
	err = json.Unmarshal(summarizeChecks(baseline), &summarized)
	if err != nil || len(summarized[1].FailureCases) != 5 || summarized[1].Fail != 10 {
		t.Fatalf("failure sample not bounded while count retained: %+v err=%v", summarized[1], err)
	}
	candidate := []CheckResult{
		{CommandID: "go-01", ExitCode: 0, Complete: true, Cases: goCases},
		{CommandID: "js-01", ExitCode: 0, Complete: true, Cases: map[string]string{"frontend.test.ts > expected result": "pass"}},
	}
	var candidateSummary []checkSummary
	if err = json.Unmarshal(summarizeChecks(candidate), &candidateSummary); err != nil || len(candidateSummary) != 2 || candidateSummary[1].CommandID != "js-01" || candidateSummary[1].Pass != 1 || candidateSummary[1].ExitCode != 0 {
		t.Fatalf("frontend pass omitted from summary: %+v err=%v", candidateSummary, err)
	}
}
