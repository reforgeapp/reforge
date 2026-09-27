package repair

import (
	"context"
	"strings"
	"testing"
	"time"

	"reforge/internal/model"
	"reforge/internal/sandbox"
)

func TestOwnerRunChecksRetargetsRestoredProtectedFiles(t *testing.T) {
	plan, files := testPlan(t)
	files[".github/workflows/ci.yml"] = []byte("name: CI\n")
	plan, err := freeze("javascript", plan.Image, plan.BaselineSHA, plan.TargetSHA, files, plan.ForbiddenPaths, true)
	if err != nil {
		t.Fatal(err)
	}
	plan.Owner = true
	plan.MaxChangedLines = 1 << 20
	plan.Recipe.MaxFiles = 20
	plan.Recipe.MaxPatchBytes = 768 << 10
	plan.Digest = planDigest(plan)
	checkpoint := &Checkpoint{
		PlanDigest: plan.Digest,
		Patches: []sandbox.Patch{
			{Path: "package.json", Content: []byte(`{"name":"owner-update","scripts":{"test":"node --test"}}`)},
			{Path: "value.test.js", Content: []byte("const test = require('node:test'); const assert = require('node:assert'); test('adds',()=>assert.equal(require('./value').add(2,3),5))\n")},
			{Path: ".github/workflows/ci.yml", Content: []byte("name: CI\njobs:\n  checks:\n    runs-on: ubuntu-latest\n")},
		},
	}
	runtime := &passRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, target: files, targetSHA: plan.TargetSHA}
	turn := 0
	engine := Engine{Runtime: runtime, Restore: checkpoint, Model: "fixture", JobID: "retarget", AttemptID: "retarget", Trust: "fixture", MaxOutputTokens: 256, TurnTimeout: time.Second, Progress: func(context.Context, string) error { return nil }}
	engine.Turn = func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		turn++
		if turn == 1 {
			if !strings.Contains(in.Messages[0].Text, "Saved progress restored") {
				t.Fatal("checkpoint was not restored")
			}
			return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "checks", Name: "run_checks", Arguments: []byte(`{}`)}}}, nil
		}
		for _, message := range in.Messages {
			if message.ToolCallID == "checks" && !strings.Contains(message.Text, `"complete":true`) {
				t.Fatalf("staged protected edits did not pass candidate checks: %s", message.Text)
			}
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Validate restored owner changes"}`)}}}, nil
	}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if err != nil || report.State != "validated" || turn != 2 || len(report.Patches) != len(checkpoint.Patches) {
		t.Fatalf("owner report=%+v turns=%d error=%v", report, turn, err)
	}
}
