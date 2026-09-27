package repair

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"reforge/internal/model"
	"reforge/internal/sandbox"
)

func TestOwnerResumesSavedEditsAndRerunsChecks(t *testing.T) {
	plan, files := testPlan(t)
	plan.Owner, plan.Recipe.MaxTurns = true, 6
	plan.Digest = planDigest(plan)
	runtime := &passRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, target: files, targetSHA: plan.TargetSHA}
	var saved *Checkpoint
	stopped := errors.New("runner interrupted")
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "first", Trust: "fixture", MaxOutputTokens: 128, TurnTimeout: time.Second, Progress: func(context.Context, string) error { return nil }}
	engine.SaveCheckpoint = func(_ context.Context, in Checkpoint) error {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &saved)
	}
	calls := 0
	engine.Turn = func(context.Context, model.Turn) (model.TurnResult, error) {
		calls++
		if calls > 1 {
			return model.TurnResult{}, stopped
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "edit", Name: "edit_file", Arguments: []byte(`{"path":"value.js","old":"a-b","new":"a+b"}`)}}}, nil
	}
	_, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if !errors.Is(err, stopped) || saved == nil || saved.Turns != 1 || len(saved.Patches) != 1 {
		t.Fatalf("checkpoint not saved before interruption: err=%v checkpoint=%+v", err, saved)
	}
	engine.Restore = saved
	engine.AttemptID = "second"
	calls = 0
	engine.Turn = func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		calls++
		switch calls {
		case 1:
			if len(in.Continuation) != 0 || !strings.Contains(in.Messages[0].Text, "staged value.js") || !strings.Contains(in.Messages[0].Text, "Run checks again") {
				t.Fatal("resume lost staged context or retained provider history")
			}
			return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "early-finish", Name: "finish", Arguments: []byte(`{"summary":"Try earlier checks"}`)}}}, nil
		case 2:
			if !strings.Contains(in.Messages[len(in.Messages)-1].Text, "Not finished") {
				t.Fatal("resume accepted old validation")
			}
			return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "read", Name: "read_file", Arguments: []byte(`{"path":"value.js"}`)}, {ID: "checks", Name: "run_checks", Arguments: []byte(`{}`)}}}, nil
		case 3:
			found := false
			for _, m := range in.Messages {
				if m.ToolCallID == "read" && strings.Contains(m.Text, "a+b") {
					found = true
				}
			}
			if !found {
				t.Fatal("restored file contents missing")
			}
			return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Finish resumed repair"}`)}}}, nil
		}
		t.Fatal("unexpected model turn")
		return model.TurnResult{}, stopped
	}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if err != nil || report.State != "validated" || report.Turns != 4 || len(report.Candidate) == 0 || len(report.Patches) != 1 || !strings.Contains(string(report.Patches[0].Content), "a+b") {
		t.Fatalf("resumed repair=%+v err=%v", report, err)
	}
}

func TestOwnerRejectsInvalidCheckpointAndKeepsTurnLimit(t *testing.T) {
	plan, files := testPlan(t)
	plan.Owner, plan.Recipe.MaxTurns = true, 3
	plan.Digest = planDigest(plan)
	for _, tc := range []struct {
		name       string
		checkpoint Checkpoint
		want       error
	}{
		{"different plan", Checkpoint{PlanDigest: "other"}, ErrValidation},
		{"sensitive patch", Checkpoint{PlanDigest: plan.Digest, Patches: []sandbox.Patch{{Path: ".env", Content: []byte("secret")}}}, ErrValidation},
		{"turn limit", Checkpoint{PlanDigest: plan.Digest, Turns: 3}, ErrHandoff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &passRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, target: files, targetSHA: plan.TargetSHA}
			engine := Engine{Runtime: runtime, Restore: &tc.checkpoint, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", MaxOutputTokens: 128, TurnTimeout: time.Second, Progress: func(context.Context, string) error { return nil }, Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
				t.Fatal("invalid or exhausted checkpoint invoked model")
				return model.TurnResult{}, nil
			}}
			_, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestOwnerRecentCheckpointKeepsUnicodeBoundaries(t *testing.T) {
	recent := ""
	for i := 0; i < 10; i++ {
		recent = appendOwnerRecent(recent, "read_file", strings.Repeat("界", 4000))
	}
	if len(recent) > ownerRecentBudget || !utf8.ValidString(recent) {
		t.Fatal("checkpoint context exceeded its byte limit or split Unicode")
	}
}
