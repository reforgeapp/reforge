package gitlab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/forge"
)

func TestGitLabDeliveryGuardAndPinnedDispatch(t *testing.T) {
	sha := strings.Repeat("a", 40)
	config := []byte("stages: [test]\n")
	digest := sha256.Sum256(config)
	configHash := hex.EncodeToString(digest[:])
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/17/pipeline":
			posts++
			var body struct {
				Ref       string              `json:"ref"`
				Variables []map[string]string `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Ref != "main" || len(body.Variables) != 5 {
				t.Error("invalid pinned variables")
			}
			seen := map[string]string{}
			for _, variable := range body.Variables {
				seen[variable["key"]] = variable["value"]
			}
			if seen["REFORGE_WORKFLOW_SHA"] != sha || seen["REFORGE_SOURCE_SHA"] != sha {
				t.Error("workflow or source pin missing")
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":46,"project_id":17,"sha":"` + sha + `"}`))
		case r.URL.Path == "/api/v4/projects/17":
			w.Write([]byte(`{"id":17,"path_with_namespace":"group/project","default_branch":"main","web_url":"https://gitlab.example/group/project","ci_config_path":null}`))
		case r.URL.Path == "/api/v4/projects/17/repository/branches/main":
			w.Write([]byte(`{"protected":true,"commit":{"id":"` + sha + `"}}`))
		case r.URL.Path == "/api/v4/projects/17/repository/files/.gitlab-ci.yml/raw":
			w.Write(config)
		case r.URL.Path == "/api/v4/projects/17/protected_environments":
			w.Write([]byte(`[{"name":"production","deploy_access_levels":[{"access_level":40}],"required_approval_count":1,"approval_rules":[{"required_approvals":1}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p, err := New(forge.Config{BaseURL: server.URL + "/api/v4", Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	in := forge.PipelineRequest{Repository: forge.RepoRef{NativeID: "17", FullName: "group/project"}, WorkflowID: "pipeline", WorkflowPath: ".gitlab-ci.yml", ConfigSHA256: configHash, Ref: "refs/heads/main", SourceSHA: sha, WorkflowSHA: sha, ArtifactDigest: "sha256:" + strings.Repeat("b", 64), Environment: "production", CorrelationID: "11111111-1111-1111-1111-111111111111", RulesHash: strings.Repeat("c", 64)}
	if _, err = p.TriggerOrObservePipeline(context.Background(), in); err == nil {
		t.Fatal("missing delivery guard accepted")
	}
	guarded := p.WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error { return nil })
	gates, err := guarded.ReadDeploymentGates(context.Background(), in.Repository, in.Environment)
	if err != nil {
		t.Fatal(err)
	}
	in.RulesHash = gates.RulesHash
	out, err := guarded.TriggerOrObservePipeline(context.Background(), in)
	if err != nil || out.ID != "46" || out.State != "queued" || posts != 1 {
		t.Fatalf("dispatch: %+v %v posts=%d", out, err, posts)
	}
}

func TestGitLabDeliveryObserveFailureDoesNotResubmit(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			t.Fatal("observe resubmitted pipeline")
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	p, err := New(forge.Config{BaseURL: server.URL + "/api/v4", Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	in := forge.PipelineRequest{Repository: forge.RepoRef{NativeID: "17", FullName: "group/project"}, WorkflowID: "pipeline", WorkflowPath: ".gitlab-ci.yml", ConfigSHA256: strings.Repeat("c", 64), Ref: "refs/heads/main", SourceSHA: strings.Repeat("a", 40), WorkflowSHA: strings.Repeat("a", 40), ArtifactDigest: "sha256:" + strings.Repeat("b", 64), Environment: "production", CorrelationID: "11111111-1111-1111-1111-111111111111", RulesHash: strings.Repeat("c", 64), RunID: "46", ObserveOnly: true}
	if _, err = p.TriggerOrObservePipeline(context.Background(), in); err == nil || posts != 0 {
		t.Fatalf("uncertain observe: err=%v posts=%d", err, posts)
	}
}
