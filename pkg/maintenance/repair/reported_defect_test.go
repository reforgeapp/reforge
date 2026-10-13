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
			} else if !errors.Is(err, ErrHandoff) || report.State == "validated" {
				t.Fatalf("unreproduced repair validated: %+v %v", report, err)
			}
		})
	}
}
