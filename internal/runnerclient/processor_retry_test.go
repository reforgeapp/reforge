package runnerclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/reforgeapp/reforge/internal/sandbox"
)

func TestRepairProcessorResumesOnlyTransientControlFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		retry  bool
	}{
		{"server unavailable", http.StatusServiceUnavailable, `{"message":"temporarily unavailable"}`, true},
		{"database failure", http.StatusInternalServerError, `{"message":"internal error"}`, true},
		{"revoked credential", http.StatusUnauthorized, `{"message":"unauthorized"}`, false},
		{"changed authority", http.StatusConflict, `{"message":"conflict"}`, false},
		{"invalid request", http.StatusBadRequest, `{"message":"invalid"}`, false},
		{"malformed success", http.StatusOK, `{`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/runner/v1/repair/run" {
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := New(Config{Endpoint: server.URL, Development: true, Name: "retry-test", CredentialFile: filepath.Join(t.TempDir(), "credential")})
			if err != nil {
				t.Fatal(err)
			}
			process, closeRuntime := RepairProcessorWithCloser(sandbox.RuntimeConfig{})
			defer closeRuntime()
			completion, err := process(context.Background(), client, Job{Token: "test-token"})
			if err == nil || completion.Outcome != "failed" || completion.Retryable != tc.retry || errors.Is(err, ErrTransientControlPlane) != tc.retry {
				t.Fatalf("completion=%+v err=%v", completion, err)
			}
		})
	}
}

func TestRepairProcessorCancellationDoesNotRequestRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("canceled job sent a request") }))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Development: true, Name: "retry-test", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process, closeRuntime := RepairProcessorWithCloser(sandbox.RuntimeConfig{})
	defer closeRuntime()
	completion, err := process(ctx, client, Job{Token: "test-token"})
	if !errors.Is(err, context.Canceled) || completion.Retryable {
		t.Fatalf("completion=%+v err=%v", completion, err)
	}
}
