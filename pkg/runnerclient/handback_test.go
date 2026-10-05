package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestStoppingRunnerHandsJobBackAsPaused(t *testing.T) {
	completed := make(chan workflow.Completion, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runner/v1/result":
			var in workflow.Completion
			_ = json.NewDecoder(r.Body).Decode(&in)
			completed <- in
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Development: true, Name: "handback-test", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	err = client.runJob(ctx, Job{Token: "job-token"}, func(ctx context.Context, _ *Client, _ Job) (workflow.Completion, error) {
		stop()
		<-ctx.Done()
		return workflow.Completion{Outcome: "failed"}, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runJob error = %v", err)
	}
	select {
	case got := <-completed:
		if got.Outcome != "paused" || got.RetryAfterMS <= 0 {
			t.Fatalf("completion = %+v", got)
		}
	default:
		t.Fatal("stopped job was not handed back")
	}
}
