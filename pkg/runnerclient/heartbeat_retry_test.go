package runnerclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestHeartbeatRetriesTransientFailureWithinLease(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(runner.Heartbeat{Lease: workflow.Lease{ExpiresAt: time.Now().Add(time.Minute)}})
	}))
	defer server.Close()
	client := testHeartbeatClient(t, server.URL)
	job := heartbeatTestJob(time.Now().Add(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		client.heartbeatLoop(ctx, job, cancel, time.Millisecond, time.Second, time.Millisecond)
		close(stopped)
	}()
	deadline := time.After(time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("heartbeat did not recover")
		case <-time.After(time.Millisecond):
		}
	}
	if ctx.Err() != nil {
		t.Fatal("transient heartbeat failure canceled work")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("heartbeat loop did not stop")
	}
}

func TestHeartbeatDefinitiveRejectionCancelsImmediately(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := testHeartbeatClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		client.heartbeatLoop(ctx, heartbeatTestJob(time.Now().Add(time.Minute)), cancel, time.Millisecond, time.Second, time.Millisecond)
		close(stopped)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("definitive heartbeat rejection did not cancel work")
	}
	<-stopped
	if calls.Load() != 1 {
		t.Fatalf("heartbeat calls=%d, want 1", calls.Load())
	}
}

func TestHeartbeatLeaseSafetyMarginCancelsWork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := testHeartbeatClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		client.heartbeatLoop(ctx, heartbeatTestJob(time.Now().Add(5100*time.Millisecond)), cancel, time.Second, time.Second, time.Millisecond)
		close(stopped)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("work continued without a safely valid lease")
	}
	<-stopped
	if calls.Load() != 0 {
		t.Fatalf("heartbeat calls=%d after lease safety deadline, want 0", calls.Load())
	}
}

func testHeartbeatClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	client, err := New(Config{Endpoint: endpoint, Development: true, Name: "heartbeat-test", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func heartbeatTestJob(expires time.Time) Job {
	return Job{Token: "job/heartbeat", Lease: workflow.Lease{OrgID: "org", RepositoryID: "repo", TaskID: "task", ExpiresAt: expires}}
}
