package runnerclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/reforgeapp/reforge/internal/workflow"
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
