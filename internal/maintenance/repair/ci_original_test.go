package repair

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/model"
	"reforge/internal/sandbox"
)

func TestOwnerReadsOriginalRevisionAgainstChangedTarget(t *testing.T) {
	plan, baseFiles := testPlan(t)
	plan.Owner = true
	plan.Recipe.MaxTurns = 2
	plan.MaxChangedLines = 1 << 20
	plan.Recipe.MaxFiles = 20
	plan.Recipe.MaxPatchBytes = 768 << 10
	plan.Digest = planDigest(plan)
	baseFiles["value.js"] = []byte("exports.source = 'original';")
	baseFiles["src/large.go"] = []byte(strings.Repeat("x", 65530) + "\nORIGINAL-ONLY-CODE\n" + strings.Repeat("z", 2048))
	baseFiles[".env"] = []byte("TOKEN=private")
	targetFiles := make(map[string][]byte, len(baseFiles))
	for name, body := range baseFiles {
		targetFiles[name] = append([]byte(nil), body...)
	}
	targetFiles["value.js"] = []byte("exports.source = 'target';")
	runtime := &passRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: targetFiles}, target: targetFiles, targetSHA: plan.TargetSHA}
	turn := 0
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "original-nav", AttemptID: "original-nav", Trust: "fixture", MaxOutputTokens: 256, TurnTimeout: time.Second, Progress: func(context.Context, string) error { return nil }}
	engine.Turn = func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		turn++
		if turn == 1 {
			prompt := in.Messages[0].Text
			if !strings.Contains(prompt, plan.BaselineSHA) || !strings.Contains(prompt, plan.TargetSHA) || !strings.Contains(prompt, "value.js (changed)") || strings.Contains(prompt, "exports.source = 'original'") {
				t.Fatal("owner prompt omitted source revisions or inlined original source")
			}
			hasOriginalTool := false
			for _, tool := range in.Tools {
				hasOriginalTool = hasOriginalTool || tool.Name == "read_original_file"
			}
			if !hasOriginalTool {
				t.Fatal("owner tool list lacks original-file navigation")
			}
			return model.TurnResult{ToolCalls: []model.ToolCall{
				{ID: "original", Name: "read_original_file", Arguments: []byte(`{"path":"value.js"}`)},
				{ID: "target", Name: "read_file", Arguments: []byte(`{"path":"value.js"}`)},
				{ID: "chunk", Name: "read_original_file", Arguments: []byte(`{"path":"src/large.go","offset":65530,"limit":128}`)},
				{ID: "secret", Name: "read_original_file", Arguments: []byte(`{"path":".env"}`)},
				{ID: "badpath", Name: "read_original_file", Arguments: []byte(`{"path":"../value.js"}`)},
				{ID: "edit", Name: "write_file", Arguments: []byte(`{"path":"value.js","content":"exports.source = 'fixed';"}`)},
				{ID: "checks", Name: "run_checks", Arguments: []byte(`{}`)},
			}}, nil
		}
		replies := map[string]string{}
		for _, message := range in.Messages {
			if message.ToolCallID != "" {
				replies[message.ToolCallID] = message.Text
			}
		}
		var original, target, chunk navigationFileChunk
		if json.Unmarshal([]byte(replies["original"]), &original) != nil || original.Content != "exports.source = 'original';" {
			t.Fatalf("original read returned %q", replies["original"])
		}
		if json.Unmarshal([]byte(replies["target"]), &target) != nil || target.Content != "exports.source = 'target';" {
			t.Fatalf("target read returned %q", replies["target"])
		}
		if json.Unmarshal([]byte(replies["chunk"]), &chunk) != nil || !strings.Contains(chunk.Content, "ORIGINAL-ONLY-CODE") || chunk.Done {
			t.Fatalf("original chunk failed: %+v reply=%q", chunk, replies["chunk"])
		}
		if !strings.Contains(replies["secret"], "unavailable") || !strings.Contains(replies["badpath"], "unavailable") {
			t.Fatalf("unsafe reads were not rejected: secret=%q path=%q", replies["secret"], replies["badpath"])
		}
		if !strings.Contains(replies["checks"], `"complete":true`) {
			t.Fatalf("candidate checks did not pass: %s", replies["checks"])
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "finish", Name: "finish", Arguments: []byte(`{"summary":"Apply remaining repair"}`)}}}, nil
	}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, baseFiles), snapshotForEngine(t, plan.TargetSHA, targetFiles))
	if err != nil || report.State != "validated" || report.Mode != "owner" || len(report.Patches) != 1 || string(report.Patches[0].Content) != "exports.source = 'fixed';" {
		t.Fatalf("owner report=%+v error=%v", report, err)
	}
}

func TestGoalExplainsConflictingRepairReplacement(t *testing.T) {
	repo := forge.RepoRef{NativeID: "repo-1", FullName: "org/project"}
	finding := discovery.Finding{Observation: discovery.Observation{Source: "pull_request", Evidence: discovery.Evidence{Change: &forge.Change{
		ID: "42", Repository: repo, HeadRepository: repo, TargetRepository: repo,
		HeadBranch: "reforge/repair/abc", State: "open", MergeStatus: "dirty",
	}}}}
	goal := Goal(finding)
	for _, expected := range []string{"current default branch", "read_original_file", "Obsolete:", "replacement pull request", "original stays open"} {
		if !strings.Contains(goal, expected) {
			t.Fatalf("conflict goal missing %q: %s", expected, goal)
		}
	}
}

func TestOwnerCanResolveObsoleteConflictingRepair(t *testing.T) {
	plan, files := testPlan(t)
	plan.Owner = true
	plan.Recipe.MaxTurns = 1
	plan.Digest = planDigest(plan)
	runtime := &passRuntime{retryRuntime: retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, target: files, targetSHA: plan.TargetSHA}
	engine := Engine{Runtime: runtime, AllowObsolete: true, Model: "fixture", JobID: "obsolete", AttemptID: "obsolete", Trust: "fixture", MaxOutputTokens: 256, TurnTimeout: time.Second, Progress: func(context.Context, string) error { return nil }}
	engine.Turn = func(context.Context, model.Turn) (model.TurnResult, error) {
		return model.TurnResult{ToolCalls: []model.ToolCall{
			{ID: "original", Name: "read_original_file", Arguments: []byte(`{"path":"value.js"}`)},
			{ID: "skip", Name: "skip", Arguments: []byte(`{"reason":"Obsolete: target already contains the repair"}`)},
		}}, nil
	}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if err != nil || report.Disposition != "superseded" || report.Reason != "Obsolete: target already contains the repair" || len(report.Patches) != 0 {
		t.Fatalf("obsolete report=%+v error=%v", report, err)
	}
}
