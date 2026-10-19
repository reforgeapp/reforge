package runnerclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/runner"
	"github.com/reforgeapp/reforge/internal/sandbox"
	"github.com/reforgeapp/reforge/internal/sandbox/guest"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type patchRuntime struct{ files map[string][]byte }

func (p patchRuntime) PreparePinnedWorkspace(context.Context, sandbox.WorkspaceRequest) (sandbox.Workspace, error) {
	return sandbox.Workspace{}, nil
}
func (p patchRuntime) ExecuteBoundedCommand(context.Context, sandbox.Workspace, sandbox.Command) (sandbox.CommandResult, error) {
	return sandbox.CommandResult{}, nil
}
func (p patchRuntime) ApplyPatch(context.Context, sandbox.Workspace, []sandbox.Patch) error {
	return nil
}
func (p patchRuntime) CollectArtifact(_ context.Context, _ sandbox.Workspace, name string) (sandbox.Artifact, error) {
	return sandbox.Artifact{Name: name, Data: p.files[name]}, nil
}
func (p patchRuntime) Destroy(context.Context, sandbox.Workspace) error { return nil }

func TestCustomProfilePatchExtraction(t *testing.T) {
	files := []guest.File{{Path: "value.js", Content: []byte("old")}, {Path: "keep.js", Content: []byte("same")}}
	digest, err := sandbox.SnapshotDigest(files)
	if err != nil {
		t.Fatal(err)
	}
	baseline := sandbox.Snapshot{CommitSHA: strings.Repeat("a", 40), Complete: true, ManifestSHA256: digest, Files: files}
	client := &Client{}
	patches, err := client.customProfilePatches(context.Background(), patchRuntime{files: map[string][]byte{"value.js": []byte("new"), "keep.js": []byte("same")}}, sandbox.Workspace{}, baseline)
	if err != nil || len(patches) != 1 || patches[0].Path != "value.js" || string(patches[0].Content) != "new" {
		t.Fatalf("patch extraction=%+v err=%v", patches, err)
	}
}

func TestCredentialRestartAndJobProtocol(t *testing.T) {
	var completed atomic.Bool
	supervisor := "sup/fixture-supervisor"
	job := Job{Token: "job/fixture-job", ExpiresAt: time.Now().Add(time.Minute), Lease: workflow.Lease{OrgID: "org", RepositoryID: "repo", TaskID: "task", ExpiresAt: time.Now().Add(time.Minute)}, Task: workflow.Task{ID: "task", RepositoryID: "repo"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			t.Error("worker request carried browser origin")
		}
		switch r.URL.Path {
		case "/runner/v1/enroll":
			if r.Header.Get("Authorization") != "Bearer enr/fixture" {
				t.Error("incorrect enrollment credential")
			}
			json.NewEncoder(w).Encode(credential{Token: supervisor, ExpiresAt: time.Now().Add(12 * time.Hour), Runner: runner.Runner{ID: "runner", OrgID: "org"}})
		case "/runner/v1/claim":
			if r.Header.Get("Authorization") != "Bearer "+supervisor {
				t.Error("incorrect supervisor credential")
			}
			json.NewEncoder(w).Encode(job)
		case "/runner/v1/progress", "/runner/v1/result":
			if r.Header.Get("Authorization") != "Bearer "+job.Token {
				t.Error("job used supervisor credential")
			}
			if strings.HasSuffix(r.URL.Path, "result") {
				completed.Store(true)
			}
			json.NewEncoder(w).Encode(job.Task)
		default:
			t.Errorf("unexpected worker method %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cfg := Config{Endpoint: server.URL, Development: true, Name: "Fixture", CredentialFile: filepath.Join(t.TempDir(), "private", "credential.json")}
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Enroll(context.Background(), "enr/fixture"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cfg.CredentialFile)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credential was not persisted privately")
	}
	restarted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Load(); err != nil {
		t.Fatal(err)
	}
	claimed, err := restarted.Claim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.runJob(context.Background(), claimed, func(ctx context.Context, c *Client, j Job) (workflow.Completion, error) {
		if err := c.Progress(ctx, j, domain.TaskPlanning); err != nil {
			return workflow.Completion{}, err
		}
		return workflow.Completion{Outcome: "failed"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !completed.Load() {
		t.Fatal("job result not reported")
	}
	if err = os.Chmod(cfg.CredentialFile, 0644); err != nil {
		t.Fatal(err)
	}
	if restarted.Load() == nil {
		t.Fatal("public credential file accepted")
	}
}

func TestWorkerRedirectDoesNotForwardCredentials(t *testing.T) {
	var received atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Enroll(context.Background(), "enr/private"); err == nil || received.Load() != 0 {
		t.Fatal("worker followed credential-bearing redirect")
	}
}
