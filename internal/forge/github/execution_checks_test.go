package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

func TestExecutionChecksCreateUpdateReadAndIdentity(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	repository := forge.RepoRef{NativeID: "42", FullName: "acme/repo"}
	operation := domain.NewID()
	requests := []string{}
	checkGets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/repos/acme/repo" {
			_, _ = io.WriteString(w, `{"id":42,"full_name":"acme/repo"}`)
			return
		}
		if r.URL.Path == "/repos/acme/repo/check-runs" && r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["head_sha"] != sha || body["external_id"] != operation || body["status"] != "in_progress" {
				t.Fatalf("create body: %#v err=%v", body, err)
			}
			_, _ = io.WriteString(w, `{"id":7,"name":"reforge","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"in_progress","external_id":"`+operation+`","app":{"id":42}}`)
			return
		}
		if r.URL.Path == "/repos/acme/repo/check-runs/7" {
			if r.Method == http.MethodGet {
				checkGets++
				if checkGets == 1 || checkGets == 2 {
					_, _ = io.WriteString(w, `{"id":7,"name":"reforge","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"in_progress","external_id":"`+operation+`","app":{"id":42}}`)
				} else if checkGets == 3 {
					_, _ = io.WriteString(w, `{"id":7,"name":"reforge","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"failure","external_id":"`+operation+`","app":{"id":42}}`)
				} else {
					_, _ = io.WriteString(w, `{"id":7,"name":"reforge","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success","external_id":"`+operation+`","app":{"id":42}}`)
				}
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["status"] != "completed" || body["conclusion"] != "failure" {
				t.Fatalf("update body: %#v err=%v", body, err)
			}
			_, _ = io.WriteString(w, `{"id":7,"name":"reforge","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"failure","external_id":"`+operation+`","app":{"id":42}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "installation-token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider.app = &installationAuth{appID: "42", installationID: "17", token: "installation-token", actorID: "2", slug: "reforge", expires: time.Now().Add(time.Hour)}
	created, err := provider.WriteExecutionCheck(context.Background(), forge.ExecutionCheckRequest{Repository: repository, SHA: sha, Name: "reforge", State: "pending", OperationID: operation})
	if err != nil || created.ID != "7" || created.State != "pending" {
		t.Fatalf("create: %#v %v", created, err)
	}
	updated, err := provider.WriteExecutionCheck(context.Background(), forge.ExecutionCheckRequest{Repository: repository, SHA: sha, Name: "reforge", State: "failure", OperationID: operation, CheckID: "7"})
	if err != nil || updated.State != "failure" {
		t.Fatalf("update: %#v %v", updated, err)
	}
	read, err := provider.ReadExecutionCheck(context.Background(), repository, "7")
	if err != nil || read.State != "success" || read.OperationID != operation {
		t.Fatalf("read: %#v %v", read, err)
	}
	if len(requests) != 9 {
		t.Fatalf("request sequence: %#v", requests)
	}
}

func TestExecutionChecksRejectMismatchedExistingIdentityAndUncertainCreate(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/repo" {
			_, _ = io.WriteString(w, `{"id":42,"full_name":"acme/repo"}`)
			return
		}
		if r.URL.Path == "/repos/acme/repo/check-runs/7" {
			_, _ = io.WriteString(w, `{"id":7,"name":"other","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"in_progress","external_id":"00000000-0000-0000-0000-000000000001","app":{"id":42}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider.app = &installationAuth{appID: "42", installationID: "17", token: "token", actorID: "2", slug: "reforge", expires: time.Now().Add(time.Hour)}
	_, err = provider.WriteExecutionCheck(context.Background(), forge.ExecutionCheckRequest{Repository: forge.RepoRef{NativeID: "42", FullName: "acme/repo"}, SHA: sha, Name: "reforge", State: "success", OperationID: domain.NewID(), CheckID: "7"})
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != "identity" {
		t.Fatalf("mismatch error: %#v", err)
	}
}

func TestExecutionChecksCreateFailureIsUncertainAndNotRetried(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/repo" {
			_, _ = io.WriteString(w, `{"id":42,"full_name":"acme/repo"}`)
			return
		}
		if r.URL.Path == "/repos/acme/repo/check-runs" {
			requests++
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider.app = &installationAuth{appID: "42", installationID: "17", token: "token", actorID: "2", slug: "reforge", expires: time.Now().Add(time.Hour)}
	_, err = provider.WriteExecutionCheck(context.Background(), forge.ExecutionCheckRequest{Repository: forge.RepoRef{NativeID: "42", FullName: "acme/repo"}, SHA: sha, Name: "reforge", State: "success", OperationID: domain.NewID()})
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain || requests != 1 {
		t.Fatalf("uncertain create: err=%#v requests=%d", err, requests)
	}
}

func TestExecutionChecksMutationResponseFailuresAreUncertain(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	for _, mode := range []string{"wrong-id", "wrong-state", "malformed", "lost-read"} {
		t.Run(mode, func(t *testing.T) {
			operation := domain.NewID()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/repos/acme/repo" {
					_, _ = io.WriteString(w, `{"id":42,"full_name":"acme/repo"}`)
					return
				}
				if r.URL.Path == "/repos/acme/repo/check-runs" {
					switch mode {
					case "wrong-id":
						_, _ = io.WriteString(w, `{"id":8,"name":"reforge","head_sha":"`+sha+`","status":"in_progress","external_id":"`+operation+`","app":{"id":42}}`)
					case "wrong-state":
						_, _ = io.WriteString(w, `{"id":7,"name":"reforge","head_sha":"`+sha+`","status":"completed","conclusion":"success","external_id":"`+operation+`","app":{"id":42}}`)
					case "malformed":
						_, _ = io.WriteString(w, `{`)
					default:
						_, _ = io.WriteString(w, `{"id":7,"name":"reforge","head_sha":"`+sha+`","status":"in_progress","external_id":"`+operation+`","app":{"id":42}}`)
					}
					return
				}
				if r.URL.Path == "/repos/acme/repo/check-runs/7" {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			provider.app = &installationAuth{appID: "42", installationID: "17", token: "token", actorID: "2", slug: "reforge", expires: time.Now().Add(time.Hour)}
			_, err = provider.WriteExecutionCheck(context.Background(), forge.ExecutionCheckRequest{Repository: forge.RepoRef{NativeID: "42", FullName: "acme/repo"}, SHA: sha, Name: "reforge", State: "pending", OperationID: operation})
			var providerErr *domain.ProviderError
			if !errors.As(err, &providerErr) || !providerErr.Uncertain {
				t.Fatalf("mode=%s err=%#v", mode, err)
			}
		})
	}
}
