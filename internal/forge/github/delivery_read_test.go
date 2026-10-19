package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

func TestListAllowedWorkflowsReturnsActiveIdentityAcrossPages(t *testing.T) {
	p, err := New(forge.Config{BaseURL: "https://api.github.test/api/v3", Token: "token", Client: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/api/v3/repos/acme/repo":
			return deliveryJSON(200, map[string]any{"id": 17, "full_name": "acme/repo", "default_branch": "main"})
		case req.URL.Path == "/api/v3/repos/acme/repo/actions/workflows" && req.URL.Query().Get("page") == "1":
			response, _ := deliveryJSON(200, map[string]any{"workflows": []any{map[string]any{"id": 11, "name": "Deploy", "path": ".github/workflows/deploy.yml", "state": "active", "html_url": "https://github.test/acme/repo/actions/workflows/11"}, map[string]any{"id": 12, "name": "Old", "path": ".github/workflows/old.yml", "state": "disabled_manually"}}})
			response.Header.Set("Link", `<https://api.github.test/api/v3/repos/acme/repo/actions/workflows?page=2&per_page=100>; rel="next"`)
			return response, nil
		case req.URL.Path == "/api/v3/repos/acme/repo/actions/workflows" && req.URL.Query().Get("page") == "2":
			response, _ := deliveryJSON(200, map[string]any{"workflows": []any{map[string]any{"id": 13, "name": "Checks", "path": ".github/workflows/checks.yml", "state": "active"}}})
			response.Header.Set("Link", `<https://api.github.test/api/v3/repos/acme/repo/actions/workflows?page=3&per_page=100>; rel="next"`)
			return response, nil
		case req.URL.Path == "/api/v3/repos/acme/repo/actions/workflows" && req.URL.Query().Get("page") == "3":
			return deliveryJSON(200, map[string]any{"workflows": []any{map[string]any{"id": 14, "name": "Release", "path": ".github/workflows/release.yml", "state": "active"}}})
		default:
			return deliveryJSON(404, nil)
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	workflows, err := p.ListAllowedWorkflows(context.Background(), forge.RepoRef{NativeID: "17", FullName: "acme/repo"})
	if err != nil || len(workflows) != 3 || workflows[0].ID != "11" || workflows[1].Path != ".github/workflows/checks.yml" || workflows[2].ID != "14" {
		t.Fatalf("workflows=%+v err=%v", workflows, err)
	}
}

func TestReadDeploymentStatusCorrelatesExactRunAndMapsState(t *testing.T) {
	correlation := domain.NewID()
	p, err := New(forge.Config{BaseURL: "https://api.github.test/api/v3", Token: "token", Client: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return deliveryJSON(200, map[string]any{"id": 71, "head_sha": strings.Repeat("a", 40), "head_branch": "main", "path": ".github/workflows/deploy.yml", "event": "workflow_dispatch", "status": "completed", "conclusion": "success", "workflow_id": 11, "display_title": "reforge:" + correlation, "run_attempt": 2, "html_url": "https://github.test/acme/repo/actions/runs/71", "created_at": "2026-09-21T00:00:00Z", "updated_at": "2026-09-21T00:01:00Z", "repository": map[string]any{"id": 17, "full_name": "acme/repo"}})
	})})
	if err != nil {
		t.Fatal(err)
	}
	status, err := p.ReadDeploymentStatus(context.Background(), forge.RepoRef{NativeID: "17", FullName: "acme/repo"}, "71")
	if err != nil || status.State != "success" || status.CorrelationID != correlation || status.WorkflowSHA == "" || status.Health != "unknown" || status.RunAttempt != 2 || status.CreatedAt.IsZero() {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestReadDeploymentStatusUnknownEvidenceIsConservative(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		status     string
		conclusion any
		wantState  string
		wantErr    bool
	}{
		{name: "unknown status", status: "mystery", conclusion: nil, wantState: "unknown"},
		{name: "missing conclusion", status: "completed", conclusion: nil, wantState: "unknown"},
		{name: "unknown conclusion", status: "completed", conclusion: "mystery", wantState: "unknown"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			p, err := New(forge.Config{BaseURL: "https://api.github.test/api/v3", Token: "token", Client: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return deliveryJSON(200, map[string]any{"id": 71, "head_sha": strings.Repeat("a", 40), "head_branch": "main", "path": ".github/workflows/deploy.yml", "event": "push", "status": testCase.status, "conclusion": testCase.conclusion, "workflow_id": 11, "display_title": "other", "run_attempt": 1, "html_url": "https://github.test/acme/repo/actions/runs/71", "created_at": "2026-09-21T00:00:00Z", "updated_at": "2026-09-21T00:01:00Z", "repository": map[string]any{"id": 17, "full_name": "acme/repo"}})
			})})
			if err != nil {
				t.Fatal(err)
			}
			status, err := p.ReadDeploymentStatus(context.Background(), forge.RepoRef{NativeID: "17", FullName: "acme/repo"}, "71")
			if (err != nil) != testCase.wantErr || err == nil && status.State != testCase.wantState || err != nil && status.State != "" {
				t.Fatalf("status=%+v err=%v", status, err)
			}
		})
	}
}

func deliveryJSON(status int, value any) (*http.Response, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
}
