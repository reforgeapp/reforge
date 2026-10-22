package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func TestLargeOwnerRepairTransfersAndOrdinaryRequestLimit(t *testing.T) {
	const token = "job/fixture"
	content := strings.Repeat("x", 1800<<10)
	patch := sandbox.Patch{Path: "dist/THIRD_PARTY_LICENSES/license.txt", Content: []byte(content)}
	checkpoint := repair.Checkpoint{PlanDigest: "fixture-plan", Patches: []sandbox.Patch{patch}, Turns: 1}
	responseBody, err := json.Marshal(repair.Run{State: "repairing", Checkpoint: &checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unexpected repair credential")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		seen[r.URL.Path] = body
		switch r.URL.Path {
		case "/runner/v1/repair/checkpoint":
			w.WriteHeader(http.StatusNoContent)
		case "/runner/v1/repair/report":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"state":"stored"}`)
		case "/runner/v1/repair/run":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(responseBody)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Token: token}
	assertReceivedPatch := func(t *testing.T, path string) {
		t.Helper()
		body := seen[path]
		var received struct {
			Patches []sandbox.Patch `json:"patches"`
		}
		if len(body) <= 1<<20 || json.Unmarshal(body, &received) != nil || len(received.Patches) != 1 || string(received.Patches[0].Content) != content {
			t.Fatalf("repair request body size/content = %d", len(body))
		}
	}

	t.Run("checkpoint request", func(t *testing.T) {
		if err := client.RepairCheckpoint(context.Background(), job, checkpoint); err != nil {
			t.Fatalf("large owner checkpoint rejected: %v", err)
		}
		assertReceivedPatch(t, "/runner/v1/repair/checkpoint")
	})

	t.Run("report request", func(t *testing.T) {
		report := repair.Report{PlanDigest: "fixture-plan", State: "unverified", Mode: "owner", Patches: []sandbox.Patch{patch}}
		if _, err := client.RepairReport(context.Background(), job, report); err != nil {
			t.Fatalf("large owner report rejected: %v", err)
		}
		assertReceivedPatch(t, "/runner/v1/repair/report")
	})

	t.Run("run response", func(t *testing.T) {
		run, err := client.RepairRun(context.Background(), job)
		if err != nil {
			t.Fatalf("large repair run response rejected: %v", err)
		}
		if run.Checkpoint == nil || len(run.Checkpoint.Patches) != 1 || string(run.Checkpoint.Patches[0].Content) != content {
			patches := 0
			if run.Checkpoint != nil {
				patches = len(run.Checkpoint.Patches)
			}
			t.Fatalf("large repair run response round trip: patches=%d", patches)
		}
	})

	t.Run("ordinary request remains capped", func(t *testing.T) {
		ordinary := strings.Repeat("y", 1200<<10)
		_, err := client.call(context.Background(), http.MethodPost, "/runner/v1/ordinary", token, map[string]string{"payload": ordinary}, nil)
		if !errors.Is(err, ErrControlPlane) {
			t.Fatalf("oversized ordinary request returned %v", err)
		}
		if _, sent := seen["/runner/v1/ordinary"]; sent {
			t.Fatal("oversized ordinary request reached server")
		}
	})
}
