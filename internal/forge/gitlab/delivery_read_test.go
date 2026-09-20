package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reforge/internal/forge"
)

func TestDeliveryReadWorkflowAndPipelineIdentity(t *testing.T) {
	sha := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/17":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":17,"path_with_namespace":"group/project","default_branch":"main","web_url":"https://gitlab.example/group/project","ci_config_path":""}`))
		case "/api/v4/projects/17/pipelines/46":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":46,"project_id":17,"sha":"` + sha + `","ref":"main","source":"api","status":"success","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","web_url":"https://gitlab.example/group/project/-/pipelines/46"}`))
		case "/api/v4/projects/17/pipelines/46/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":9,"name":"test","stage":"test","status":"success","pipeline":{"id":46,"project_id":17,"sha":"` + sha + `"},"commit":{"id":"` + sha + `"}}]`))
		case "/api/v4/projects/17/pipelines/46/bridges":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		case "/api/v4/projects/17/pipelines/46/variables":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"key":"OTHER","value":"secret"},{"key":"REFORGE_OPERATION","value":"11111111-1111-1111-1111-111111111111"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p, err := New(forge.Config{BaseURL: server.URL + "/api/v4", Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	repo := forge.RepoRef{NativeID: "17", FullName: "group/project"}
	workflows, err := p.ListAllowedWorkflows(context.Background(), repo)
	if err != nil || len(workflows) != 1 || workflows[0].ID != "pipeline" || workflows[0].Path != ".gitlab-ci.yml" || workflows[0].Ref != "main" {
		t.Fatalf("workflow: %+v %v", workflows, err)
	}
	status, err := p.ReadDeploymentStatus(context.Background(), repo, "46")
	if err != nil || status.ID != "46" || status.State != "success" || status.WorkflowSHA != sha || status.CorrelationID != "11111111-1111-1111-1111-111111111111" || status.RunAttempt != 1 {
		t.Fatalf("status: %+v %v", status, err)
	}
}

func TestDeliveryReadRejectsIdentityAndCorrelationDrift(t *testing.T) {
	sha := strings.Repeat("b", 40)
	cases := []struct{ name, project, pipeline, variables string }{
		{"project", `{"id":18,"path_with_namespace":"group/project","default_branch":"main","web_url":"https://gitlab.example/group/project","ci_config_path":""}`, ``, ``},
		{"pipeline", `{"id":17,"path_with_namespace":"group/project","default_branch":"main","web_url":"https://gitlab.example/group/project","ci_config_path":""}`, `{"id":46,"project_id":18,"sha":"` + sha + `","ref":"main","source":"api","status":"success","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","web_url":"https://gitlab.example/p/46"}`, ``},
		{"duplicate correlation", `{"id":17,"path_with_namespace":"group/project","default_branch":"main","web_url":"https://gitlab.example/group/project","ci_config_path":""}`, `{"id":46,"project_id":17,"sha":"` + sha + `","ref":"main","source":"api","status":"success","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","web_url":"https://gitlab.example/p/46"}`, `[{"key":"REFORGE_OPERATION","value":"11111111-1111-1111-1111-111111111111"},{"key":"REFORGE_OPERATION","value":"22222222-2222-2222-2222-222222222222"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/api/v4/projects/17":
					w.Write([]byte(tc.project))
				case r.URL.Path == "/api/v4/projects/17/pipelines/46":
					w.Write([]byte(tc.pipeline))
				case strings.HasSuffix(r.URL.Path, "/jobs"):
					w.Write([]byte(`[{"id":9,"name":"test","stage":"test","status":"success","pipeline":{"id":46,"project_id":17,"sha":"` + sha + `"},"commit":{"id":"` + sha + `"}}]`))
				case strings.HasSuffix(r.URL.Path, "/bridges"):
					w.Write([]byte(`[]`))
				case strings.HasSuffix(r.URL.Path, "/jobs"):
					w.Write([]byte(`[{"id":9,"name":"test","stage":"test","status":"success","pipeline":{"id":46,"project_id":17,"sha":"` + sha + `"},"commit":{"id":"` + sha + `"}}]`))
				case strings.HasSuffix(r.URL.Path, "/variables"):
					w.Write([]byte(tc.variables))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			p, err := New(forge.Config{BaseURL: server.URL + "/api/v4", Token: "token", Client: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "project" {
				if _, err = p.ListAllowedWorkflows(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/project"}); err == nil {
					t.Fatal("project identity accepted")
				}
				return
			}
			if _, err = p.ReadDeploymentStatus(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/project"}, "46"); err == nil {
				t.Fatal("pipeline drift accepted")
			}
		})
	}
}

func TestMapPipelineStateConservative(t *testing.T) {
	for input, expected := range map[string]string{"pending": "queued", "running": "running", "manual": "awaiting_approval", "success": "success", "failed": "failed", "canceled": "cancelled", "weird": "unknown"} {
		if got := mapPipelineState(input); got != expected {
			t.Fatalf("%s: got %s want %s", input, got, expected)
		}
	}
}
