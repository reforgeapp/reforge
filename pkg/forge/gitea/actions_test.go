package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/forge"
)

func TestNativeActionsChecksBindJobIdentityAndExposeRedactedLogs(t *testing.T) {
	head := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, location, jobHead string
		runID                   int
		valid                   bool
	}{
		{"native", "/org/project/actions/runs/4/jobs/9", head, 4, true},
		{"subpath", "/gitea/org/project/actions/runs/4/jobs/9", head, 4, true},
		{"wrong head", "/org/project/actions/runs/4/jobs/9", strings.Repeat("b", 40), 4, false},
		{"wrong run", "/org/project/actions/runs/4/jobs/9", head, 5, false},
		{"other repository", "/org/other/actions/runs/4/jobs/9", head, 4, false},
		{"external location", "https://elsewhere.example/org/project/actions/runs/4/jobs/9", head, 4, false},
		{"unattributed status", "", head, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mount := ""
			if tc.name == "subpath" {
				mount = "/gitea"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch strings.TrimPrefix(r.URL.Path, mount) {
				case "/api/v1/repos/org/project":
					fmt.Fprint(w, `{"id":31,"full_name":"org/project"}`)
				case "/api/v1/repos/org/project/statuses/" + head:
					_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 2, "context": "CI", "status": "success", "target_url": tc.location}})
				case "/api/v1/repos/org/project/actions/jobs/9":
					_ = json.NewEncoder(w).Encode(map[string]any{"id": 9, "run_id": tc.runID, "head_sha": tc.jobHead, "status": "completed", "conclusion": "failure"})
				case "/api/v1/repos/org/project/actions/jobs/9/logs":
					fmt.Fprint(w, strings.Repeat("x", forge.MaxLogBytes)+"\nUnable to find LTS release\npassword=fixture-secret")
				default:
					t.Errorf("unexpected route %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			p, err := New(forge.Config{BaseURL: server.URL + mount, Client: server.Client(), Token: "token"})
			if err != nil {
				t.Fatal(err)
			}
			ref := forge.RepoRef{NativeID: "31", FullName: "org/project"}
			checks, err := p.ListChecks(context.Background(), ref, head)
			if tc.name == "wrong head" {
				if err != nil || len(checks) != 0 {
					t.Fatalf("different revision attributed to requested head: %+v %v", checks, err)
				}
				return
			}
			if !tc.valid {
				if err == nil {
					t.Fatal("unbound native job accepted")
				}
				return
			}
			if err != nil || len(checks) != 1 || checks[0].ID != "actions:9" || checks[0].PublisherID != "gitea-actions" || checks[0].Conclusion != "failure" || checks[0].URL != server.URL+tc.location {
				t.Fatalf("native checks=%+v error=%v", checks, err)
			}
			logs, err := p.ReadCheckLog(context.Background(), ref, checks[0].ID)
			if err != nil || !strings.Contains(logs, "Unable to find LTS release") || strings.Contains(logs, "fixture-secret") || len(logs) > forge.MaxLogBytes {
				t.Fatalf("bounded redacted log unavailable: %v", err)
			}
		})
	}
}

func TestStatusOrderingKeepsNewestPerPublisher(t *testing.T) {
	head := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/project") {
			fmt.Fprint(w, `{"id":31,"full_name":"org/project"}`)
			return
		}
		fmt.Fprint(w, `[{"id":1,"context":"CI","creator":{"id":7},"status":"pending"},{"id":3,"context":"CI","creator":{"id":7},"status":"failure"},{"id":2,"context":"CI","creator":{"id":7},"status":"success"},{"id":4,"context":"CI","creator":{"id":8},"status":"success"}]`)
	}))
	defer server.Close()
	p, _ := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "token"})
	checks, err := p.ListChecks(context.Background(), forge.RepoRef{NativeID: "31", FullName: "org/project"}, head)
	if err != nil || len(checks) != 2 || checks[0].ID != "3" || checks[0].Conclusion != "failure" || checks[0].PublisherID != "7" || checks[1].PublisherID != "8" {
		t.Fatalf("newest publisher checks lost: %+v %v", checks, err)
	}
}
