package repair

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

type reportedDefectRuntime struct {
	retryRuntime
	reproduces bool
}

func TestNarratedChangesStopWithoutBecomingRepairEvidence(t *testing.T) {
	plan, files := testPlan(t)
	plan.Owner = true
	plan.Recipe.MaxTurns = 8
	plan.Digest = planDigest(plan)
	runtime := &reportedDefectRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, reproduces: true}
	calls := 0
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "narrated", AttemptID: "narrated", Trust: "fixture", Progress: func(context.Context, string) error { return nil }, Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
		calls++
		return model.TurnResult{Text: "I applied the change and all tests pass.", FinishReason: "stop"}, nil
	}}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if !errors.Is(err, ErrHandoff) || calls != 2 || report.State == "validated" || len(report.Patches) != 0 || !strings.Contains(report.Reason, "qualified for repository work") {
		t.Fatalf("narrated changes became work: calls=%d report=%+v err=%v", calls, report, err)
	}
}

func (r *reportedDefectRuntime) ExecuteBoundedCommand(_ context.Context, w sandbox.Workspace, _ sandbox.Command) (sandbox.CommandResult, error) {
	fixed := false
	for _, patch := range r.patches[w.ID] {
		fixed = fixed || patch.Path == "value.js" && strings.Contains(string(patch.Content), "a+b")
	}
	if r.reproduces && !fixed {
		return sandbox.CommandResult{ExitCode: 1, Output: []byte("not ok 1 - adds\n")}, nil
	}
	return sandbox.CommandResult{Output: []byte("ok 1 - adds\n")}, nil
}

func TestReportedDefectNeedsOriginalFailureAndFixedPass(t *testing.T) {
	for _, source := range []string{"user_report", "forge_issue", "repository_review"} {
		t.Run(source, func(t *testing.T) { reportedDefectNeedsOriginalFailureAndFixedPass(t, source) })
	}
}

func reportedDefectNeedsOriginalFailureAndFixedPass(t *testing.T, source string) {
	for _, reproduces := range []bool{false, true} {
		t.Run(map[bool]string{false: "unreproduced", true: "reproduced"}[reproduces], func(t *testing.T) {
			plan, files := testPlan(t)
			plan.Owner = true
			plan.Recipe.MaxTurns = 2
			plan.Digest = planDigest(plan)
			runtime := &reportedDefectRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, reproduces: reproduces}
			finding := discovery.Finding{Observation: discovery.Observation{Source: source, Category: "code_defect"}}
			engine := Engine{Runtime: runtime, RequireReproduction: RequiresReproduction(finding), Model: "fixture", JobID: "reported", AttemptID: "reported", Trust: "fixture", Progress: func(context.Context, string) error { return nil }, Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
				return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "fix", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.add = (a,b) => a+b"}`)}, {ID: "check", Name: "run_checks", Arguments: []byte(`{}`)}, {ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Adds inputs correctly"}`)}}}, nil
			}}
			report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
			if reproduces {
				if err != nil || report.State != "validated" || !BehaviourReproduced(plan, report.Reproduction) || !Verified(plan, report.Baseline, report.Candidate) {
					t.Fatalf("reproduced repair rejected: %+v %v", report, err)
				}
			} else if err != nil || report.State != "unverified" || !strings.Contains(report.Reason, "Independent CI behaviour proof required") {
				t.Fatalf("unreproduced repair validated: %+v %v", report, err)
			}
		})
	}
}

func TestReproducedOwnerDefectKeepsExistingChecksFrozen(t *testing.T) {
	plan, files := testPlan(t)
	plan.Owner = true
	plan.Digest = planDigest(plan)
	runtime := &reportedDefectRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, reproduces: true}
	engine := Engine{Runtime: runtime, RequireReproduction: true, Model: "fixture", JobID: "source-defect", AttemptID: "reported", Trust: "fixture", Turn: func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		for _, tool := range in.Tools {
			if tool.Name == "write_file" || tool.Name == "delete_file" || tool.Name == "update_dependency" || tool.Name == "run_command" {
				t.Fatal("reproduced defect retained broad maintenance tools")
			}
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "weaken", Name: "apply_patch", Arguments: []byte(`{"path":"value.test.js","content":""}`)}, {ID: "fix", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.add = (a,b) => a+b"}`)}, {ID: "check", Name: "run_checks", Arguments: []byte(`{}`)}, {ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Adds inputs correctly"}`)}}}, nil
	}}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if err != nil || report.State != "validated" || len(report.Patches) != 1 || report.Patches[0].Path != "value.js" || !BehaviourFixed(plan, report.Reproduction, report.Candidate) {
		t.Fatalf("reported defect changed checks or lost proof: %+v %v", report, err)
	}
}

func TestFrozenDefectRetainsOriginalBehaviourProof(t *testing.T) {
	plan, files := testPlan(t)
	runtime := &reportedDefectRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, reproduces: true}
	engine := Engine{Runtime: runtime, RequireReproduction: true, Goal: "Adding inputs returns their sum", Model: "fixture", JobID: "frozen-defect", AttemptID: "reported", Trust: "fixture", Turn: func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		if !strings.Contains(in.Messages[0].Text, "Adding inputs returns their sum") {
			t.Fatal("reported behaviour omitted from frozen repair")
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "fix", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.add = (a,b) => a+b"}`)}, {ID: "check", Name: "run_checks", Arguments: []byte(`{}`)}}}, nil
	}}
	baseline := snapshotForEngine(t, plan.BaselineSHA, files)
	target := snapshotForEngine(t, plan.TargetSHA, files)
	verify := func(report Report, err error) {
		t.Helper()
		if err != nil || report.State != "validated" || !BehaviourFixed(plan, report.Reproduction, report.Candidate) || !BehaviourFixed(plan, report.Reproduction, report.Target) {
			t.Fatalf("frozen defect lost publication proof: %+v %v", report, err)
		}
		changedCase := append([]CheckResult(nil), report.Candidate...)
		changedCase[0].Cases = map[string]string{"1:unrelated": "pass"}
		if BehaviourFixed(plan, report.Reproduction, changedCase) {
			t.Fatal("unrelated passing check replaced reproduced case")
		}
	}
	verify(engine.Run(context.Background(), plan, baseline, target))
	verify(engine.ValidateCustom(context.Background(), plan, baseline, target, []sandbox.Patch{{Path: "value.js", Content: []byte("exports.add = (a,b) => a+b")}}))
}
