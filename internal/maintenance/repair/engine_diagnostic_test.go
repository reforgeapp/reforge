package repair

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"reforge/internal/maintenance/recipes"
	"reforge/internal/model"
	"reforge/internal/sandbox"
)

type diagnosticRuntime struct {
	files        map[string][]byte
	patches      map[string][]sandbox.Patch
	executed     map[string]bool
	sequence     int
	readErr      error
	baselineFail bool
}

func (r *diagnosticRuntime) PreparePinnedWorkspace(_ context.Context, in sandbox.WorkspaceRequest) (sandbox.Workspace, error) {
	r.sequence++
	return sandbox.Workspace{ID: fmt.Sprint(r.sequence), CommitSHA: in.CommitSHA}, nil
}

func (r *diagnosticRuntime) ApplyPatch(_ context.Context, w sandbox.Workspace, patches []sandbox.Patch) error {
	r.patches[w.ID] = append([]sandbox.Patch(nil), patches...)
	return nil
}

func (r *diagnosticRuntime) ExecuteBoundedCommand(_ context.Context, w sandbox.Workspace, _ sandbox.Command) (sandbox.CommandResult, error) {
	r.executed[w.ID] = true
	if r.baselineFail && len(r.patches[w.ID]) == 0 {
		return sandbox.CommandResult{ExitCode: 1, Output: []byte("not ok 1 - check\n")}, nil
	}
	return sandbox.CommandResult{Output: []byte("ok 1 - check\n")}, nil
}

func (r *diagnosticRuntime) CollectArtifact(_ context.Context, w sandbox.Workspace, name string) (sandbox.Artifact, error) {
	if len(r.patches[w.ID]) > 0 && r.executed[w.ID] && name == "value.test.js" {
		if r.readErr != nil {
			return sandbox.Artifact{}, r.readErr
		}
		return sandbox.Artifact{Name: name, Data: []byte("changed protected test")}, nil
	}
	return sandbox.Artifact{Name: name, Data: append([]byte(nil), r.files[name]...)}, nil
}

func (r *diagnosticRuntime) Destroy(context.Context, sandbox.Workspace) error { return nil }

func TestReportIdentifiesProtectedValidationFailureSafely(t *testing.T) {
	base, target := strings.Repeat("b", 40), strings.Repeat("c", 40)
	files := map[string][]byte{
		"value.js":      []byte("exports.add = (a,b) => a+b"),
		"value.test.js": []byte("test fixture"),
	}
	plan := Plan{
		MaxChangedLines: 100,
		Version:         1,
		BaselineSHA:     base,
		TargetSHA:       target,
		Image:           "sha256:" + strings.Repeat("a", 64),
		Recipe: recipes.Recipe{
			Name: "javascript", Version: "v4", Commands: []recipes.Command{{ID: "node-01", Args: []string{"node", "--test"}, Directory: ".", TimeoutSeconds: 60, ReportFormat: "tap"}},
			ProtectedPaths: []string{"value.test.js"}, MaxFiles: 1, MaxPatchBytes: 65536, MaxTurns: 2, TimeoutSeconds: 900,
		},
		ProtectedHashes: map[string]string{"value.test.js": hashBytes(files["value.test.js"])},
		Owner:           true,
	}
	plan.Digest = planDigest(plan)

	for _, tc := range []struct {
		name     string
		readErr  error
		wantText string
		regular  bool
	}{
		{name: "owner read failure", readErr: errors.New("private runtime detail"), wantText: "could not be read"},
		{name: "owner missing evidence", readErr: fs.ErrNotExist, wantText: "is missing"},
		{name: "owner changed evidence", wantText: "changed"},
		{name: "regular repair read failure", readErr: errors.New("private runtime detail"), wantText: "could not be read", regular: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan.Owner = !tc.regular
			plan.Digest = planDigest(plan)
			runtime := &diagnosticRuntime{files: files, patches: map[string][]sandbox.Patch{}, executed: map[string]bool{}, readErr: tc.readErr, baselineFail: tc.regular}
			patchCall := model.ToolCall{ID: "write", Name: "write_file", Arguments: []byte(`{"path":"value.js","content":"exports.add = (a,b) => a-b"}`)}
			if tc.regular {
				patchCall.Name = "apply_patch"
			}
			engine := Engine{
				Runtime: runtime,
				Model:   "fixture",
				Turn: func(context.Context, model.Turn) (model.TurnResult, error) {
					return model.TurnResult{ToolCalls: []model.ToolCall{
						patchCall,
						{ID: "checks", Name: "run_checks", Arguments: []byte(`{}`)},
					}}, nil
				},
			}
			var report Report
			var err error
			if tc.regular {
				report, err = engine.Run(context.Background(), plan, snapshotForEngine(t, base, files), snapshotForEngine(t, target, files))
			} else {
				report, err = engine.runCI(context.Background(), plan, Report{State: "handoff", Artifacts: []string{}}, files)
			}
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("expected validation sentinel, got report=%+v err=%v", report, err)
			}
			if tc.readErr != nil && !errors.Is(err, tc.readErr) {
				t.Fatalf("underlying read cause was not preserved: %v", err)
			}
			if !strings.Contains(report.Reason, "node-01") || !strings.Contains(report.Reason, "value.test.js") || !strings.Contains(report.Reason, tc.wantText) {
				t.Fatalf("safe validation detail missing: %q", report.Reason)
			}
			if strings.Contains(report.Reason, "private runtime detail") {
				t.Fatalf("underlying error text leaked into report: %q", report.Reason)
			}
		})
	}
}
