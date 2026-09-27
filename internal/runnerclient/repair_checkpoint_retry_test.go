package runnerclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"reforge/internal/maintenance/repair"
)

func TestRepairCheckpointRetriesTransientWithIdenticalRequest(t *testing.T) {
	var attempts atomic.Int32
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		if r.Header.Get("Authorization") != "Bearer stable-token" {
			t.Errorf("unexpected authorization header")
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	input := repair.Checkpoint{PlanDigest: "plan", Recent: "progress", Turns: 7}
	if err := client.RepairCheckpoint(context.Background(), Job{Token: "stable-token"}, input); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 || len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("attempts=%d request bodies differ", attempts.Load())
	}
}

func TestRepairCheckpointDoesNotRetryPermanentOrMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "conflict", status: http.StatusConflict, body: `{"code":"conflict","message":"stale"}`},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"message":"unauthorized"}`},
		{name: "malformed success", status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
			if err != nil {
				t.Fatal(err)
			}
			if err := client.RepairCheckpoint(context.Background(), Job{Token: "token"}, repair.Checkpoint{Turns: 1}); err == nil || attempts.Load() != 1 {
				t.Fatalf("attempts=%d err=%v", attempts.Load(), err)
			}
		})
	}
}

func TestRepairCheckpointCanceledDuringRetryBackoff(t *testing.T) {
	first := make(chan struct{})
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		if attempts.Add(1) == 1 {
			close(first)
		}
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.RepairCheckpoint(ctx, Job{Token: "token"}, repair.Checkpoint{Turns: 1})
	}()
	<-first
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled || attempts.Load() != 1 {
			t.Fatalf("attempts=%d err=%v", attempts.Load(), err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancellation did not interrupt retry backoff")
	}
}
