package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/model"
)

func retryTestTurn() model.Turn {
	return model.Turn{OperationID: "turn-operation", Model: "fixture", Messages: []model.Message{{Role: "user", Text: "continue"}}, MaxOutputTokens: 32, TimeoutMS: 5000, Continuation: json.RawMessage(`{"opaque":"state"}`)}
}

func TestModelTurnRetriesTransientWithIdenticalRequest(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var tokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		tokens = append(tokens, r.Header.Get("Authorization"))
		attempt := len(bodies)
		mu.Unlock()
		if attempt == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"ok"}`)
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.ModelTurn(context.Background(), Job{Token: "job-stable-token"}, retryTestTurn())
	if err != nil || got.Text != "ok" {
		t.Fatalf("turn=%+v err=%v", got, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) || tokens[0] != "Bearer job-stable-token" || tokens[1] != tokens[0] {
		t.Fatalf("retry changed request: attempts=%d tokens=%v", len(bodies), tokens)
	}
	var sent model.Turn
	if err := json.Unmarshal(bodies[0], &sent); err != nil || sent.OperationID != "turn-operation" || string(sent.Continuation) != `{"opaque":"state"}` {
		t.Fatalf("request identity changed: turn=%+v err=%v", sent, err)
	}
}

func TestModelTurnDoesNotRetryUnauthorizedOrUnknownState(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"message":"stop"}`)
			}))
			defer server.Close()
			client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.ModelTurn(context.Background(), Job{Token: "job-token"}, retryTestTurn()); err == nil || attempts.Load() != 1 {
				t.Fatalf("attempts=%d err=%v", attempts.Load(), err)
			}
		})
	}
}

func TestModelTurnCanceledDuringRetryBackoff(t *testing.T) {
	first := make(chan struct{})
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		if attempt == 1 {
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
		_, err := client.ModelTurn(ctx, Job{Token: "job-token"}, retryTestTurn())
		done <- err
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
