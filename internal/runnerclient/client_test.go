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

	"reforge/internal/domain"
	"reforge/internal/runner"
	"reforge/internal/workflow"
)

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
