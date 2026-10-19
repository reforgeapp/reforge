package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/customcmd"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/maintenance/repair"
	"github.com/reforgeapp/reforge/internal/runnerclient"
	"github.com/reforgeapp/reforge/internal/sandbox"
	"github.com/reforgeapp/reforge/internal/sandbox/guest"
	"github.com/reforgeapp/reforge/internal/workflow"
)

func TestCustomProfileRealProcessorGVisor(t *testing.T) {
	if os.Getenv("REFORGE_CUSTOM_PROFILE_PROCESSOR_TEST") != "1" {
		t.Skip("set REFORGE_CUSTOM_PROFILE_PROCESSOR_TEST=1 for real gVisor custom-profile processor acceptance")
	}
	if runtime.GOOS != "linux" {
		t.Skip("gVisor fixture requires Linux")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	runsc, tool := "/tmp/reforge-gvisor/bin/runsc", filepath.Join(root, "bin/reforge-sandbox-tool")
	for _, path := range []string{runsc, tool} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("required local sandbox asset %s: %v", path, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	imageRoot := filepath.Join(t.TempDir(), "javascript")
	if output, err := exec.CommandContext(ctx, "python3", filepath.Join(root, "scripts/build-runner-images.py"), "--stack", "javascript", "--output", imageRoot).CombinedOutput(); err != nil {
		t.Fatalf("build local JavaScript image: %v: %s", err, output)
	}
	image, err := sandbox.ImageDigest(imageRoot)
	if err != nil {
		t.Fatal(err)
	}
	baseSHA, targetSHA, candidateSHA := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	baseFiles := map[string][]byte{
		"value.js":      []byte("exports.value = function() { return 1 }"),
		"value.test.js": []byte("const test = require('node:test'); const assert = require('node:assert/strict'); const { value } = require('./value.js'); test('value', () => assert.equal(value(), 2));"),
	}
	base := lifecycleSnapshot(t, baseSHA, baseFiles)
	target := lifecycleSnapshot(t, targetSHA, baseFiles)
	plan, err := repair.Freeze("javascript", image, baseSHA, targetSHA, baseFiles, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name        string
		patched     string
		wantState   string
		wantPublish bool
	}{
		{name: "valid candidate", patched: "exports.value = function() { return 2 }", wantState: "validated", wantPublish: true},
		{name: "failing candidate denied", patched: "exports.value = function() { return 3 }", wantState: "handoff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patched := lifecycleSnapshot(t, candidateSHA, map[string][]byte{"value.js": []byte(tc.patched), "value.test.js": baseFiles["value.test.js"]})
			stageCount, nativeCount, publishCount := 0, 0, 0
			published := false
			var recorded customcmd.ReportInput
			var repairReport repair.Report
			var publishInput repair.Publication
			var run repair.Run
			var task workflow.Task
			task = workflow.Task{ID: "task", RepositoryID: "repository", State: domain.TaskRepairing, PolicyHash: "policy", ModelConnectionID: "model", ModelRoute: "default"}
			profile := &customcmd.ProfileSpec{ID: "profile", Version: 1, ImageDigest: image, Executable: "/usr/local/bin/node", Argv: []string{"-e", "require('fs').writeFileSync('value.js', '" + tc.patched + "'), console.log(JSON.stringify({type:'result',data:{outcome:'success'}}))"}, ProtocolVersion: customcmd.ProtocolVersion, MaxWallSeconds: 20, MaxOutputBytes: 4096, MaxTurns: 2, Concurrency: 1}
			execution := repair.ExecutionContext{Plan: plan, Repository: forge.RepoRef{FullName: "fixture/repository"}, Model: "fixture", MaxOutputTokens: 128, TurnTimeoutMS: 1000, CustomProfile: profile, PolicyHash: "policy"}
			run = repair.Run{Task: task, Context: execution}
			snapshots := map[string]sandbox.Snapshot{baseSHA: base, targetSHA: target, candidateSHA: patched}
			mux := http.NewServeMux()
			mux.HandleFunc("/runner/v1/repair/run", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(run) })
			mux.HandleFunc("/runner/v1/repair/source/", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(snapshots[strings.TrimPrefix(r.URL.Path, "/runner/v1/repair/source/")])
			})
			mux.HandleFunc("/runner/v1/progress", func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					State domain.TaskState `json:"state"`
				}
				_ = json.NewDecoder(r.Body).Decode(&input)
				task.State = input.State
				_ = json.NewEncoder(w).Encode(task)
			})
			mux.HandleFunc("/runner/v1/repair/custom/authorize", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(customcmd.Authorized{RunID: "custom-run", Profile: *profile})
			})
			mux.HandleFunc("/runner/v1/repair/custom/report", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&recorded)
				_ = json.NewEncoder(w).Encode(customcmd.Authorized{})
			})
			mux.HandleFunc("/runner/v1/artifacts", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "fixture-artifact"})
			})
			mux.HandleFunc("/runner/v1/repair/report", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&repairReport)
				run.Report = &repairReport
				_ = json.NewEncoder(w).Encode(struct{}{})
			})
			mux.HandleFunc("/runner/v1/repair/stage", func(w http.ResponseWriter, _ *http.Request) {
				if run.Report == nil || run.Report.State != "validated" {
					http.Error(w, "candidate is not validated", http.StatusConflict)
					return
				}
				stageCount++
				run.CandidateSHA = candidateSHA
				_ = json.NewEncoder(w).Encode(run)
			})
			mux.HandleFunc("/runner/v1/repair/native-checks", func(w http.ResponseWriter, _ *http.Request) {
				if run.Report == nil || run.Report.State != "validated" || stageCount != 1 {
					http.Error(w, "candidate stage missing or unvalidated", http.StatusConflict)
					return
				}
				nativeCount++
				_ = json.NewEncoder(w).Encode(run)
			})
			mux.HandleFunc("/runner/v1/repair/publish", func(w http.ResponseWriter, r *http.Request) {
				if run.Report == nil || run.Report.State != "validated" || stageCount != 1 {
					http.Error(w, "validated report or candidate stage missing", http.StatusConflict)
					return
				}
				if json.NewDecoder(r.Body).Decode(&publishInput) != nil || publishInput.HeadSHA != candidateSHA || publishInput.PlanDigest != plan.Digest || len(publishInput.Checks) == 0 {
					http.Error(w, "native check evidence missing", http.StatusConflict)
					return
				}
				for _, check := range publishInput.Checks {
					if !check.Complete || check.ExitCode != 0 {
						http.Error(w, "native check evidence failed", http.StatusConflict)
						return
					}
				}
				publishCount++
				published = true
				_ = json.NewEncoder(w).Encode(run)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			client, err := runnerclient.New(runnerclient.Config{Endpoint: server.URL, Development: true, Name: "custom-profile-test", CredentialFile: filepath.Join(t.TempDir(), "credential")})
			if err != nil {
				t.Fatal(err)
			}
			runtimeRoot := filepath.Join(t.TempDir(), "runtime")
			cfg := sandbox.RuntimeConfig{Runsc: runsc, RunscSHA256: lifecycleFileDigest(t, runsc), Tool: tool, ToolSHA256: lifecycleFileDigest(t, tool), StateRoot: runtimeRoot, Images: map[string]string{image: imageRoot}, Development: true, Rootless: true, MemoryBytes: 1 << 30, DiskBytes: 256 << 20, CPUs: 1, MaxProcesses: 128, Fetch: func(_ context.Context, request sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
				out, ok := snapshots[request.CommitSHA]
				if !ok {
					return sandbox.Snapshot{}, sandbox.ErrBoundary
				}
				return out, nil
			}}
			job := runnerclient.Job{Token: "fixture-job-token", Lease: workflow.Lease{OrgID: "org", RepositoryID: "repository", JobID: "job", TaskID: task.ID, AttemptID: "attempt"}, Task: task}
			completion, err := runnerclient.RepairProcessor(cfg)(ctx, client, job)
			if tc.wantPublish && err != nil {
				t.Fatalf("processor: %v; stage=%d native=%d publish=%d; report=%+v", err, stageCount, nativeCount, publishCount, repairReport)
			}
			if !tc.wantPublish && err == nil {
				t.Fatal("failing candidate must stop processor")
			}
			if repairReport.State != tc.wantState {
				t.Fatalf("repair report state=%q want=%q reason=%q", repairReport.State, tc.wantState, repairReport.Reason)
			}
			var outcome struct {
				Outcome string `json:"outcome"`
			}
			if recorded.RunID != "custom-run" || recorded.State != "completed_unverified" || recorded.Usage.Known || len(recorded.Events) != 1 || recorded.Events[0].Type != "result" || json.Unmarshal(recorded.Events[0].Data, &outcome) != nil || outcome.Outcome != "success" {
				t.Fatalf("custom runtime report=%+v outcome=%+v", recorded, outcome)
			}
			if tc.wantPublish {
				if !published || stageCount != 1 || nativeCount != 0 || publishCount != 1 || completion.Outcome != "completed" || len(repairReport.Patches) != 1 || string(repairReport.Patches[0].Content) != tc.patched || len(repairReport.Baseline) == 0 || len(repairReport.Candidate) == 0 || len(repairReport.Target) == 0 || len(publishInput.Checks) == 0 {
					t.Fatalf("incomplete successful lifecycle: published=%t completion=%+v report=%+v", published, completion, repairReport)
				}
			} else if published || stageCount != 0 || nativeCount != 0 || publishCount != 0 {
				t.Fatalf("failed candidate reached publication path: stage=%d native=%d publish=%d", stageCount, nativeCount, publishCount)
			}
		})
	}
}

func lifecycleSnapshot(t *testing.T, sha string, contents map[string][]byte) sandbox.Snapshot {
	t.Helper()
	files := make([]guest.File, 0, len(contents))
	for name, body := range contents {
		files = append(files, guest.File{Path: name, Content: body})
	}
	digest, err := sandbox.SnapshotDigest(files)
	if err != nil {
		t.Fatal(err)
	}
	return sandbox.Snapshot{CommitSHA: sha, Complete: true, ManifestSHA256: digest, Files: files}
}

func lifecycleFileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
