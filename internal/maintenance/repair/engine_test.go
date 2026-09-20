package repair

import (
	"context"
	"strings"
	"testing"
	"time"

	"reforge/internal/model"
	"reforge/internal/sandbox"
	"reforge/internal/sandbox/guest"
)

type retryRuntime struct {
	patches  map[string][]sandbox.Patch
	files    map[string][]byte
	requests []sandbox.WorkspaceRequest
	sequence int
}

func (r *retryRuntime) PreparePinnedWorkspace(_ context.Context, in sandbox.WorkspaceRequest) (sandbox.Workspace, error) {
	r.sequence++
	r.requests = append(r.requests, in)
	return sandbox.Workspace{ID: string(rune(r.sequence)), CommitSHA: in.CommitSHA}, nil
}
func (r *retryRuntime) ExecuteBoundedCommand(_ context.Context, w sandbox.Workspace, _ sandbox.Command) (sandbox.CommandResult, error) {
	patches := r.patches[w.ID]
	if len(patches) == 0 {
		return sandbox.CommandResult{ExitCode: 1, Output: []byte("not ok 1 - adds\n")}, nil
	}
	first := strings.Contains(string(patches[len(patches)-1].Content), "first")
	if first && w.CommitSHA == strings.Repeat("c", 40) {
		return sandbox.CommandResult{ExitCode: 1, Output: []byte("not ok 1 - adds\n")}, nil
	}
	return sandbox.CommandResult{Output: []byte("ok 1 - adds\n")}, nil
}
func (r *retryRuntime) ApplyPatch(_ context.Context, w sandbox.Workspace, patches []sandbox.Patch) error {
	r.patches[w.ID] = append([]sandbox.Patch(nil), patches...)
	return nil
}
func (r *retryRuntime) CollectArtifact(_ context.Context, _ sandbox.Workspace, name string) (sandbox.Artifact, error) {
	return sandbox.Artifact{Name: name, Data: append([]byte(nil), r.files[name]...)}, nil
}
func (r *retryRuntime) Destroy(context.Context, sandbox.Workspace) error { return nil }

func TestEngineRetriesAfterTargetCompatibilityFailure(t *testing.T) {
	plan, files := testPlan(t)
	baseSHA, targetSHA := strings.Repeat("b", 40), strings.Repeat("c", 40)
	plan.BaselineSHA, plan.TargetSHA = baseSHA, targetSHA
	plan.Recipe.MaxTurns = 2
	plan.Digest = planDigest(plan)
	runtime := &retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}
	turns := 0
	progress := []string{}
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", MaxOutputTokens: 128, TurnTimeout: time.Second, Progress: func(_ context.Context, state string) error { progress = append(progress, state); return nil }, Turn: func(_ context.Context, in model.Turn) (model.TurnResult, error) {
		turns++
		if turns == 1 {
			return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "read-target", Name: "read_target_file", Arguments: []byte(`{"path":"target.txt"}`)}, {ID: "first", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.kind = 'first'; exports.add = (a,b) => a+b"}`)}}}, nil
		}
		for _, stage := range progress {
			if stage == "validating" {
				t.Fatal("advanced to validation before compatible target checks")
			}
		}
		found := false
		for _, message := range in.Messages {
			if message.ToolCallID == "read-target" && message.Text == "target-only evidence" {
				found = true
			}
		}
		if !found {
			t.Fatal("model could not inspect pinned target content")
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "read-target-2", Name: "read_target_file", Arguments: []byte(`{"path":"target.txt"}`)}, {ID: "compatible", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.kind = 'compatible'; exports.add = (a,b) => a+b"}`)}}}, nil
	}}
	base := snapshotForEngine(t, baseSHA, files)
	targetFiles := map[string][]byte{}
	for name, body := range files {
		targetFiles[name] = body
	}
	targetFiles["target.txt"] = []byte("target-only evidence")
	target := snapshotForEngine(t, targetSHA, targetFiles)
	report, err := engine.Run(context.Background(), plan, base, target)
	if err != nil || report.State != "validated" || turns != 2 || len(report.Patches) != 1 || !strings.Contains(string(report.Patches[0].Content), "compatible") {
		t.Fatalf("retry report=%+v turns=%d error=%v", report, turns, err)
	}
	if len(progress) == 0 || progress[len(progress)-1] != "validating" {
		t.Fatalf("validation phase ordering=%v", progress)
	}
	if string(files["value.test.js"]) != "const test = require('node:test'); const assert = require('node:assert'); test('adds',()=>assert.equal(require('./value').add(2,3),5))" || len(files["package.json"]) == 0 {
		t.Fatal("protected test or manifest changed")
	}
}

func TestEngineHandsOffChangedTargetProtection(t *testing.T) {
	plan, files := testPlan(t)
	baseSHA, targetSHA := strings.Repeat("b", 40), strings.Repeat("c", 40)
	plan.BaselineSHA, plan.TargetSHA = baseSHA, targetSHA
	plan.Recipe.MaxTurns = 2
	plan.Digest = planDigest(plan)
	targetFiles := map[string][]byte{}
	for name, body := range files {
		targetFiles[name] = append([]byte(nil), body...)
	}
	targetFiles["value.test.js"] = append(targetFiles["value.test.js"], []byte(" changed")...)
	runtime := &retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.kind = 'compatible'; exports.add = (a,b) => a+b"}`)}}}, nil
	}}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, baseSHA, files), snapshotForEngine(t, targetSHA, targetFiles))
	if err == nil || report.State != "handoff" || !strings.Contains(report.Reason, "Target validation differs") {
		t.Fatalf("changed target protection report=%+v error=%v", report, err)
	}
}

func snapshotForEngine(t *testing.T, commit string, files map[string][]byte) sandbox.Snapshot {
	t.Helper()
	entries := make([]guest.File, 0, len(files))
	for name, body := range files {
		entries = append(entries, guest.File{Path: name, Content: append([]byte(nil), body...)})
	}
	digest, err := sandbox.SnapshotDigest(entries)
	if err != nil {
		t.Fatal(err)
	}
	return sandbox.Snapshot{CommitSHA: commit, Complete: true, ManifestSHA256: digest, Files: entries}
}
