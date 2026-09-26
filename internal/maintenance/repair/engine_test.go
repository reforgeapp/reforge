package repair

import (
	"context"
	"errors"
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

func TestValidateCustomRequiresBaselineAndTarget(t *testing.T) {
	plan, files := testPlan(t)
	baseSHA, targetSHA := strings.Repeat("b", 40), strings.Repeat("c", 40)
	plan.BaselineSHA, plan.TargetSHA = baseSHA, targetSHA
	plan.Digest = planDigest(plan)
	runtime := &retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}
	engine := Engine{Runtime: runtime, JobID: "job", AttemptID: "attempt", Trust: "fixture", Progress: func(context.Context, string) error { return nil }}
	base := snapshotForEngine(t, baseSHA, files)
	targetFiles := map[string][]byte{}
	for name, body := range files {
		targetFiles[name] = body
	}
	target := snapshotForEngine(t, targetSHA, targetFiles)
	report, err := engine.ValidateCustom(context.Background(), plan, base, target, []sandbox.Patch{{Path: "value.js", Content: []byte("exports.add = (a,b) => a+b")}})
	if err != nil || report.State != "validated" || !Verified(plan, report.Baseline, report.Candidate) || !Verified(plan, report.Baseline, report.Target) {
		t.Fatalf("validated custom report=%+v err=%v", report, err)
	}
	if _, err := engine.ValidateCustom(context.Background(), plan, base, target, []sandbox.Patch{{Path: "value.test.js", Content: []byte("x")}}); !errors.Is(err, ErrValidation) {
		t.Fatalf("protected patch must be rejected, got %v", err)
	}
	if _, err := engine.ValidateCustom(context.Background(), plan, base, target, nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty patch set must be rejected, got %v", err)
	}
}

func TestEngineKeepsValidationAcrossReadOnlyTurns(t *testing.T) {
	plan, files := testPlan(t)
	plan.BaselineSHA, plan.TargetSHA = strings.Repeat("b", 40), strings.Repeat("c", 40)
	plan.Recipe.MaxTurns = 4
	plan.Digest = planDigest(plan)
	runtime := &retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}
	turn := 0
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
		turn++
		call := model.ToolCall{ID: "read", Name: "read_file", Arguments: []byte(`{"path":"value.js"}`)}
		if turn == 1 {
			call = model.ToolCall{ID: "first", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.kind='first'; exports.add=(a,b)=>a+b"}`)}
		}
		if turn == 4 {
			call = model.ToolCall{ID: "revised", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.kind='compatible'; exports.add=(a,b)=>a+b"}`)}
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{call}}, nil
	}}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if err != nil || report.State != "validated" || turn != 4 || len(runtime.requests) != 5 {
		t.Fatalf("read turns repeated frozen validation or reused changed-patch evidence: state=%s turns=%d workspaces=%d err=%v", report.State, turn, len(runtime.requests), err)
	}
	if len(report.Patches) != 1 || !strings.Contains(string(report.Patches[0].Content), "compatible") {
		t.Fatal("stale candidate accepted")
	}
}

func TestEngineStopsRepeatedRejectedTestRewrites(t *testing.T) {
	plan, files := testPlan(t)
	plan.BaselineSHA, plan.TargetSHA = strings.Repeat("b", 40), strings.Repeat("c", 40)
	plan.Recipe.MaxTurns = 16
	plan.Digest = planDigest(plan)
	runtime := &retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}
	turn := 0
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", Turn: func(_ context.Context, input model.Turn) (model.TurnResult, error) {
		turn++
		if turn == 2 && !strings.Contains(input.Messages[len(input.Messages)-1].Text, "immutable") {
			t.Fatal("protected-file refusal was not explained")
		}
		return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "rewrite", Name: "apply_patch", Arguments: []byte(`{"path":"value.test.js","content":"tests disabled"}`)}}}, nil
	}}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
	if err == nil || turn != 2 || len(report.Patches) != 0 || len(report.Candidate) != 0 || !strings.Contains(report.Reason, "rejected patch") || len(runtime.requests) != 1 {
		t.Fatalf("repeated protected edits consumed budget or validated: report=%+v turns=%d workspaces=%d err=%v", report, turn, len(runtime.requests), err)
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

func TestEngineBoundsTextOnlyRetry(t *testing.T) {
	for _, recover := range []bool{false, true} {
		plan, files := testPlan(t)
		plan.Recipe.MaxTurns = 6
		plan.Digest = planDigest(plan)
		runtime := &retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}
		turns := 0
		engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", Turn: func(_ context.Context, in model.Turn) (model.TurnResult, error) {
			turns++
			if turns == 2 && !strings.Contains(in.Messages[len(in.Messages)-1].Text, "No verified repair") {
				t.Fatal("text-only continuation lacks tool guidance")
			}
			if recover && turns == 2 {
				return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: []byte(`{"path":"value.js","content":"exports.add = (a,b) => a+b"}`)}}}, nil
			}
			return model.TurnResult{Text: "The source needs a correction"}, nil
		}}
		report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, files))
		if turns != 2 || recover && (err != nil || report.State != "validated") || !recover && (err != ErrHandoff || report.State != "handoff" || len(report.Patches) != 0) {
			t.Fatalf("text-only retry recover=%t turns=%d state=%s error=%v", recover, turns, report.State, err)
		}
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

type passRuntime struct {
	retryRuntime
	target    map[string][]byte
	targetSHA string
}

func (r *passRuntime) CollectArtifact(ctx context.Context, w sandbox.Workspace, name string) (sandbox.Artifact, error) {
	if w.CommitSHA == r.targetSHA {
		return sandbox.Artifact{Name: name, Data: append([]byte(nil), r.target[name]...)}, nil
	}
	return r.retryRuntime.CollectArtifact(ctx, w, name)
}

func (r *passRuntime) ExecuteBoundedCommand(context.Context, sandbox.Workspace, sandbox.Command) (sandbox.CommandResult, error) {
	return sandbox.CommandResult{Output: []byte("ok 1 - adds\n")}, nil
}

func TestEngineRepairsFromCILogsWithDependencyUpdate(t *testing.T) {
	plan, files := testPlan(t)
	plan.Recipe.MaxTurns = 3
	plan.Digest = planDigest(plan)
	targetFiles := map[string][]byte{}
	for name, body := range files {
		targetFiles[name] = body
	}
	targetFiles["value.test.js"] = append([]byte("// newer target test\n"), files["value.test.js"]...)
	runtime := &passRuntime{retryRuntime{patches: map[string][]sandbox.Patch{}, files: files}, targetFiles, plan.TargetSHA}
	turns := 0
	engine := Engine{Runtime: runtime, Model: "fixture", JobID: "job", AttemptID: "attempt", Trust: "fixture", MaxOutputTokens: 128, TurnTimeout: time.Second, CILogs: []CILog{{Name: "Validate", Log: "js-yaml 5.2.1 HIGH fixed 5.2.2"}}, Progress: func(context.Context, string) error { return nil },
		UpdateDependency: func(_ context.Context, in map[string][]byte, u DependencyUpdate) (map[string][]byte, error) {
			return map[string][]byte{"package.json": in["package.json"], "package-lock.json": []byte(`{"js-yaml":"` + u.Version + `"}`)}, nil
		},
		Turn: func(_ context.Context, in model.Turn) (model.TurnResult, error) {
			turns++
			switch turns {
			case 1:
				if !strings.Contains(in.Messages[0].Text, "js-yaml 5.2.1") {
					t.Fatal("CI log missing from prompt")
				}
				return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "edit", Name: "apply_patch", Arguments: []byte(`{"path":"package.json","content":"{}"}`)}, {ID: "dep", Name: "update_dependency", Arguments: []byte(`{"ecosystem":"npm","directory":".","package":"js-yaml","version":"5.2.2","strategy":"update"}`)}, {ID: "early", Name: "finish", Arguments: []byte(`{"summary":"x"}`)}}}, nil
			case 2:
				return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "check", Name: "run_checks", Arguments: []byte(`{}`)}}}, nil
			}
			return model.TurnResult{ToolCalls: []model.ToolCall{{ID: "done", Name: "finish", Arguments: []byte(`{"summary":"Bump js-yaml to 5.2.2"}`)}}}, nil
		}}
	report, err := engine.Run(context.Background(), plan, snapshotForEngine(t, plan.BaselineSHA, files), snapshotForEngine(t, plan.TargetSHA, targetFiles))
	if err != nil || report.State != "validated" || report.Mode != "ci" || len(report.Patches) != 1 || report.Patches[0].Path != "package-lock.json" || len(report.Dependencies) != 1 {
		t.Fatalf("report=%+v error=%v", report, err)
	}
	if CheckCIPatch(plan, files, report.Patches, nil) == nil || CheckCIPatch(plan, files, report.Patches, report.Dependencies) != nil {
		t.Fatal("dependency files must be admissible only with a declared update")
	}
}

func TestAdvanceKeepsContinuationAfterTruncatedReply(t *testing.T) {
	continuation, messages := advance(nil, []model.Message{{Role: "user", Text: "prompt"}}, model.TurnResult{Continuation: []byte(`["history"]`), ToolCalls: []model.ToolCall{{ID: "1"}}})
	continuation, messages = advance(continuation, messages, model.TurnResult{FinishReason: "length"})
	if string(continuation) != `["history"]` || len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("continuation=%s messages=%+v", continuation, messages)
	}
}

func TestCheckCIPatchAllowsExistingWorkflowOnly(t *testing.T) {
	plan, files := testPlan(t)
	files[".github/workflows/ci.yml"] = []byte("go-version: 1.26.5\n")
	workflow := sandbox.Patch{Path: ".github/workflows/ci.yml", Content: []byte("go-version: 1.26.6\n")}
	if err := CheckCIPatch(plan, files, []sandbox.Patch{workflow}, nil); err != nil {
		t.Fatalf("workflow edit rejected: %v", err)
	}
	if CheckCIPatch(plan, files, []sandbox.Patch{{Path: "value.test.js", Content: []byte("x")}}, nil) == nil {
		t.Fatal("test edit accepted")
	}
	if CheckCIPatch(plan, files, []sandbox.Patch{{Path: ".github/workflows/new.yml", Content: []byte("x")}}, nil) == nil {
		t.Fatal("new workflow accepted")
	}
}
