package runnerclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/workflow"
)

func TestSourceMovedErrorRequiresConflictCode(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		body         string
		moved        bool
		unauthorized bool
	}{
		{name: "typed conflict", status: http.StatusConflict, body: `{"code":"source_moved","message":"refresh"}`, moved: true},
		{name: "unauthorized with same code", status: http.StatusUnauthorized, body: `{"code":"source_moved","message":"refresh"}`, unauthorized: true},
		{name: "malformed conflict", status: http.StatusConflict, body: `{"code":"source_moved","message":1}`},
		{name: "other conflict", status: http.StatusConflict, body: `{"code":"validation_rejected","message":"review"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client, err := New(Config{Endpoint: server.URL, Development: true, Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential")})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.call(context.Background(), http.MethodPost, "/runner/v1/repair/stage", "job-token", nil, nil)
			if errors.Is(err, ErrSourceMoved) != tc.moved || errors.Is(err, ErrUnauthorized) != tc.unauthorized {
				t.Fatalf("moved=%t unauthorized=%t err=%v", errors.Is(err, ErrSourceMoved), errors.Is(err, ErrUnauthorized), err)
			}
		})
	}
}

func TestRepairStageSourceMovedIsFailedButOtherStageErrorsStayUncertain(t *testing.T) {
	if got := repairStageCompletion(ErrSourceMoved); got.Outcome != "failed" {
		t.Fatalf("source moved completion=%+v", got)
	}
	if got := repairStageCompletion(errors.New("unknown stage outcome")); got.Outcome != "uncertain" {
		t.Fatalf("unknown completion=%+v", got)
	}
	if got := repairStageCompletion(workflow.ErrFence); got.Outcome != "uncertain" {
		t.Fatalf("fence completion=%+v", got)
	}
}

func TestCompletionReasonExplainsSourceMovement(t *testing.T) {
	err := errors.Join(ErrControlPlane, ErrSourceMoved)
	if got := completionReason(err); got != "Publication failed: source or target moved; run a fresh scan" {
		t.Fatalf("reason=%q", got)
	}
}

func TestProviderUnavailableCodeRemainsTransientAndExplainsProviderFailure(t *testing.T) {
	transport := fixedRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":"provider_unavailable","message":"Git provider temporarily unavailable; retry shortly"}`))}, nil
	})
	client, err := New(Config{Endpoint: "https://runner.example", Name: "fixture", CredentialFile: filepath.Join(t.TempDir(), "credential"), Client: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.call(context.Background(), http.MethodPost, "/runner/v1/repair/stage", "job-token", nil, nil)
	if !errors.Is(err, ErrTransientControlPlane) || !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("transient=%t provider=%t err=%v", errors.Is(err, ErrTransientControlPlane), errors.Is(err, ErrProviderUnavailable), err)
	}
	if got := completionReason(err); got != "Git provider temporarily unavailable" {
		t.Fatalf("reason=%q", got)
	}
}

type fixedRoundTripper func(*http.Request) (*http.Response, error)

func (f fixedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
