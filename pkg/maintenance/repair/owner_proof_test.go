package repair

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/maintenance/recipes"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestLostPassingCase(t *testing.T) {
	base := []CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "pass", "3:flaky": "fail"}}}
	for name, tc := range map[string]struct {
		candidate []CheckResult
		lost      bool
	}{
		"kept":        {[]CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "pass", "3:flaky": "pass"}}}, false},
		"renumbered":  {[]CheckResult{{Cases: map[string]string{"1:new": "pass", "2:adds": "pass", "3:subtracts": "pass"}}}, false},
		"deleted":     {[]CheckResult{{Cases: map[string]string{"1:adds": "pass"}}}, true},
		"now-failing": {[]CheckResult{{Cases: map[string]string{"1:adds": "pass", "2:subtracts": "fail"}}}, true},
	} {
		if got := lostPassingCase(base, tc.candidate); got != tc.lost {
			t.Errorf("%s: lost=%t", name, got)
		}
	}
}

func TestOwnerVerificationCountsPassesAndAllowsOptionalSkips(t *testing.T) {
	plan, _ := testPlan(t)
	plan.Owner = true
	secondCommand := plan.Recipe.Commands[0]
	secondCommand.ID = "second-check"
	plan.Recipe.Commands = append(plan.Recipe.Commands, secondCommand)
	plan.Digest = planDigest(plan)
	commandID, secondID := plan.Recipe.Commands[0].ID, plan.Recipe.Commands[1].ID
	baseline := []CheckResult{{CommandID: commandID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestExisting": "pass"}}, {CommandID: secondID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestSecond": "pass"}}}
	mixed := []CheckResult{{CommandID: commandID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestExisting": "pass", "TestOptional": "skip"}}, {CommandID: secondID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestSecond": "pass"}}}
	if !Verified(plan, baseline, mixed) {
		t.Fatal("optional skip invalidated named passing verification")
	}
	allSkipped := []CheckResult{{CommandID: commandID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestOptional": "skip"}}, {CommandID: secondID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestSecond": "skip"}}}
	if Verified(plan, nil, allSkipped) {
		t.Fatal("all-skipped command counted as verified")
	}
	lostPass := []CheckResult{{CommandID: commandID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestExisting": "skip", "TestNew": "pass"}}, {CommandID: secondID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestSecond": "pass"}}}
	if Verified(plan, baseline, lostPass) {
		t.Fatal("skipped baseline pass was not detected")
	}
	regression := []CheckResult{{CommandID: commandID, Complete: true, ExitCode: 1, Cases: map[string]string{"TestDefect": "fail"}}, {CommandID: secondID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestSecond": "pass"}}}
	regressionSkipped := []CheckResult{{CommandID: commandID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestDefect": "skip", "TestOther": "pass"}}, {CommandID: secondID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestSecond": "pass"}}}
	if BehaviourFixed(plan, regression, regressionSkipped) {
		t.Fatal("skipped regression was accepted as fixed")
	}
}

func TestVerifiedReportAcceptsStoredProofAndRequiresDefectReproduction(t *testing.T) {
	plan, _ := testPlan(t)
	plan.Owner = true
	plan.Digest = planDigest(plan)
	command := plan.Recipe.Commands[0]
	passing := []CheckResult{{CommandID: command.ID, Complete: true, ExitCode: 0, Cases: map[string]string{"TestAdd": "pass"}}}
	report := Report{State: "unverified", PlanDigest: plan.Digest, Baseline: passing, Candidate: passing}
	if !VerifiedReport(ExecutionContext{Plan: plan}, report, passing) {
		t.Fatal("stored owner proof was not recognized")
	}
	stale := report
	stale.PlanDigest = "stale"
	if VerifiedReport(ExecutionContext{Plan: plan}, stale, passing) {
		t.Fatal("stale plan digest was accepted")
	}
	defect := discovery.Finding{Observation: discovery.Observation{Source: "user_report", Category: "code_defect"}}
	withoutReproduction := ExecutionContext{Plan: plan, Finding: defect}
	if VerifiedReport(withoutReproduction, report, passing) {
		t.Fatal("reported defect passed without original behavior reproduction")
	}
	reproduction := []CheckResult{{CommandID: command.ID, Complete: true, ExitCode: 1, Cases: map[string]string{"TestAdd": "fail"}}}
	fixedReport := Report{State: "unverified", PlanDigest: plan.Digest, Baseline: reproduction, Candidate: passing, Reproduction: reproduction}
	if !VerifiedReport(withoutReproduction, fixedReport, passing) {
		t.Fatal("reproduced defect with passing checks was not recognized")
	}
	providerPlan := plan
	providerPlan.Recipe.MinProof = recipes.ProofProvider
	providerPlan.Digest = planDigest(providerPlan)
	providerReport := Report{State: "unverified", PlanDigest: providerPlan.Digest, Baseline: passing, Candidate: passing}
	if VerifiedReport(ExecutionContext{Plan: providerPlan}, providerReport, passing) {
		t.Fatal("provider-only proof was accepted as validated report proof")
	}
}

type nativeGoProofRuntime struct {
	retryRuntime
	output []byte
}

func (r *nativeGoProofRuntime) ExecuteBoundedCommand(context.Context, sandbox.Workspace, sandbox.Command) (sandbox.CommandResult, error) {
	return sandbox.CommandResult{Output: r.output}, nil
}

func TestNativeGoChecksPreserveOwnerEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, state, module, test, result string
		wantError                         bool
	}{
		{"validated", "validated", "old.example/repo", "TestAdd", "pass", false},
		{"proposal", "unverified", "old.example/repo", "TestAdd", "pass", false},
		{"module change", "unverified", "new.example/repo", "TestAdd", "pass", false},
		{"skipped baseline pass", "unverified", "old.example/repo", "TestAdd", "skip", true},
		{"failed baseline pass", "unverified", "old.example/repo", "TestAdd", "fail", true},
		{"missing baseline pass", "unverified", "old.example/repo", "TestOther", "pass", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{"go.mod": []byte("module old.example/repo\n"), "main_test.go": []byte("package main\n")}
			plan, err := freeze("go", "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 40), strings.Repeat("c", 40), files, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			plan.Owner = true
			plan.Recipe.Commands = []recipes.Command{{ID: "go-01", Directory: ".", Args: []string{"go", "test", "-json", "./..."}, ReportFormat: "json"}}
			plan.Digest = planDigest(plan)
			report := Report{Mode: "owner", State: tc.state, Baseline: []CheckResult{{CommandID: "go-01", Complete: true, Cases: map[string]string{"TestAdd": "pass"}}}, Patches: []sandbox.Patch{{Path: "go.mod", Content: []byte("module " + tc.module + "\n")}}}
			nativeFiles := map[string][]byte{"go.mod": report.Patches[0].Content, "main_test.go": files["main_test.go"]}
			runtime := &nativeGoProofRuntime{retryRuntime: retryRuntime{files: nativeFiles}, output: []byte(`{"Action":"` + tc.result + `","Package":"` + tc.module + `","Test":"` + tc.test + `"}` + "\n")}
			engine := Engine{Runtime: runtime}
			native, err := engine.ValidateNative(context.Background(), plan, strings.Repeat("d", 40), snapshotForEngine(t, plan.TargetSHA, files), report)
			if errors.Is(err, ErrValidation) != tc.wantError || err != nil && !tc.wantError {
				t.Fatalf("native checks=%+v error=%v", native.Checks, err)
			}
			if !tc.wantError && !publicationReady(Run{Context: ExecutionContext{Plan: plan}, Report: &report}, native.Checks) {
				t.Fatal("native checks cannot be published")
			}
		})
	}
}

func TestGoCasesRemainComparableAcrossModulePathChange(t *testing.T) {
	command := recipes.Command{Directory: ".", Args: []string{"go", "test", "-json", "./..."}, ReportFormat: "json"}
	baseline := []CheckResult{{Cases: map[string]string{
		"old.example/repo/TestAdd":               "pass",
		"old.example/repo/internal/math/TestAdd": "pass",
		"old.example/repo/TestRoutes/a/../b":     "pass",
		"old.example/repo/TestRoutes/b":          "pass",
	}}}
	candidate := []CheckResult{{Cases: map[string]string{
		"new.example/repo/TestAdd":               "pass",
		"new.example/repo/internal/math/TestAdd": "pass",
		"new.example/repo/TestRoutes/a/../b":     "pass",
		"new.example/repo/TestRoutes/b":          "pass",
	}}}
	baseline = canonicalizeGoCases(baseline, []recipes.Command{command}, map[string][]byte{"go.mod": []byte("module old.example/repo\n")})
	candidate = canonicalizeGoCases(candidate, []recipes.Command{command}, map[string][]byte{"go.mod": []byte("module new.example/repo\n")})
	if lostPassingCase(baseline, candidate) {
		t.Fatal("module path change made passing tests appear lost")
	}
	if len(baseline[0].Cases) != 4 || baseline[0].Cases["TestRoutes/a/../b"] != "pass" || baseline[0].Cases["TestRoutes/b"] != "pass" {
		t.Fatalf("test name suffix was altered or collapsed: %+v", baseline[0].Cases)
	}
	candidate[0].Cases["TestAdd"] = "skip"
	if !lostPassingCase(baseline, candidate) {
		t.Fatal("module path normalization accepted a skipped baseline pass")
	}
}
