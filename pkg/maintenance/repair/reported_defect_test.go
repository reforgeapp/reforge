package repair

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	for _, reproduces := range []bool{false, true} {
		t.Run(map[bool]string{false: "unreproduced", true: "reproduced"}[reproduces], func(t *testing.T) {
			plan, files := testPlan(t)
			plan.Owner = true
			plan.Recipe.MaxTurns = 2
			plan.Digest = planDigest(plan)
			runtime := &reportedDefectRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, reproduces: reproduces}
			engine := Engine{Runtime: runtime, RequireReproduction: true, Model: "fixture", JobID: "reported", AttemptID: "reported", Trust: "fixture", Progress: func(context.Context, string) error { return nil }, Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
				return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "fix", Name: "write_file", Arguments: []byte(`{"path":"value.js","content":"exports.add = (a,b) => a+b"}`)}, {ID: "check", Name: "run_checks", Arguments: []byte(`{}`)}, {ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Adds inputs correctly"}`)}}}, nil
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

func TestFrozenDefectRetainsOriginalBehaviourProof(t *testing.T) {
	plan, files := testPlan(t)
	runtime := &reportedDefectRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, reproduces: true}
	engine := Engine{Runtime: runtime, RequireReproduction: true, Model: "fixture", JobID: "frozen-defect", AttemptID: "reported", Trust: "fixture", Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
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
