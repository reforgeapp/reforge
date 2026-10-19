package repair

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/model"
	"github.com/reforgeapp/reforge/internal/sandbox"
	"github.com/reforgeapp/reforge/internal/skills"
)

func TestRepairAndCILoadTrustedSkillsAcrossTurns(t *testing.T) {
	const path = "vendor/addyosmani/skills/debugging-and-error-recovery/SKILL.md"
	resource, err := skills.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	mandatory, err := skills.Read(skills.CavemanPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"repair", "ci"} {
		t.Run(mode, func(t *testing.T) {
			plan, files := testPlan(t)
			files[path] = []byte("untrusted repository replacement")
			plan.Recipe.MaxTurns = 2
			plan.Digest = planDigest(plan)
			base := retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}
			var runtime sandbox.SandboxRuntime = &base
			if mode == "ci" {
				runtime = &passRuntime{retryRuntime: base, target: files, targetSHA: plan.TargetSHA}
			}
			turns := 0
			continuation := json.RawMessage(`[{"opaque":"skill-call-history"}]`)
			engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", Turn: func(_ context.Context, in model.Turn) (model.TurnResult, error) {
				turns++
				if !strings.Contains(in.System, mandatory) {
					t.Fatal("mandatory skill absent from turn")
				}
				schemas, err := model.CompileTools(in.Tools)
				if err != nil || schemas[skills.ToolName] == nil {
					t.Fatal("skill loading tool unavailable", err)
				}
				if turns == 1 {
					args, _ := json.Marshal(map[string]string{"path": path})
					return model.TurnResult{Continuation: continuation, ToolCalls: []model.ToolCall{
						{ID: "skill", Name: skills.ToolName, Arguments: args},
						{ID: "escape", Name: skills.ToolName, Arguments: json.RawMessage(`{"path":"../outside.md"}`)},
					}}, nil
				}
				if string(in.Continuation) != string(continuation) || len(in.Messages) != 2 || in.Messages[0].ToolCallID != "skill" || in.Messages[0].Text != resource || in.Messages[1].ToolCallID != "escape" || !strings.Contains(in.Messages[1].Text, "unavailable") {
					t.Fatal("trusted skill result, denied path or continuation lost")
				}
				calls := []model.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: json.RawMessage(`{"path":"value.js","content":"exports.add = (a,b) => a+b"}`)}}
				if mode == "ci" {
					calls = append(calls, model.ToolCall{ID: "check", Name: "run_checks", Arguments: json.RawMessage(`{}`)}, model.ToolCall{ID: "finish", Name: "finish", Arguments: json.RawMessage(`{"summary":"Fixed addition"}`)})
				}
				return model.TurnResult{ToolCalls: calls}, nil
			}}
			if mode == "ci" {
				engine.CILogs = []CILog{{Name: "CI", Log: "source compatibility failure"}}
			}
			report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
			if err != nil || report.State != "validated" || turns != 2 || len(report.Patches) != 1 || report.Patches[0].Path != "value.js" {
				t.Fatalf("skill-assisted repair did not retain validation: state=%s turns=%d err=%v", report.State, turns, err)
			}
		})
	}
}
