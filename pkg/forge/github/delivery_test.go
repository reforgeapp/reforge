package github

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

const deliveryConfig = "name: deploy\n"

func TestDeliveryDispatchRequiresGuardAndExactSourceConfiguration(t *testing.T) {
	fixture := newDeliveryHTTP()
	p := fixture.provider(t)
	in := fixture.request()
	if _, err := p.TriggerOrObservePipeline(context.Background(), in); err == nil {
		t.Fatal("dispatch without delivery guard succeeded")
	}
	if fixture.posts != 0 {
		t.Fatal("unguarded dispatch reached GitHub")
	}
	p = p.WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error { return nil })
	in.SourceSHA = strings.Repeat("b", 40)
	in.WorkflowSHA = in.SourceSHA
	if _, err := p.TriggerOrObservePipeline(context.Background(), in); err == nil {
		t.Fatal("moved source accepted")
	}
	if fixture.posts != 0 {
		t.Fatal("source mismatch reached GitHub")
	}
}

func TestDeliveryNativeGateDriftBlocksDispatch(t *testing.T) {
	fixture := newDeliveryHTTP()
	p := fixture.provider(t)
	gates, err := p.ReadDeploymentGates(context.Background(), fixture.repository, fixture.environment)
	if err != nil {
		t.Fatal(err)
	}
	in := fixture.request()
	in.RulesHash = gates.RulesHash
	fixture.rulesDrift = true
	p = p.WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error { return nil })
	if _, err = p.TriggerOrObservePipeline(context.Background(), in); err == nil {
		t.Fatal("native gate drift accepted")
	}
	if fixture.posts != 0 {
		t.Fatal("gate drift reached GitHub")
	}
}

func TestDeliveryDispatchReservesInputsAndDoesNotRetryUnknownResponse(t *testing.T) {
	fixture := newDeliveryHTTP()
	p := fixture.provider(t).WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error { return nil })
	in := fixture.request()
	gates, err := p.ReadDeploymentGates(context.Background(), fixture.repository, fixture.environment)
	if err != nil {
		t.Fatal(err)
	}
	in.RulesHash = gates.RulesHash
	in.Inputs = map[string]string{"safe": "value", "reforge_source_sha": "attacker"}
	if _, err = p.TriggerOrObservePipeline(context.Background(), in); err == nil || fixture.posts != 0 {
		t.Fatalf("reserved caller input accepted err=%v posts=%d", err, fixture.posts)
	}
	in.Inputs = map[string]string{"safe": "value"}
	fixture.dispatchStatus = http.StatusInternalServerError
	_, err = p.TriggerOrObservePipeline(context.Background(), in)
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain || fixture.posts != 1 {
		t.Fatalf("dispatch err=%v posts=%d", err, fixture.posts)
	}
	if fixture.bodyInputs["reforge_source_sha"] != in.SourceSHA || fixture.bodyInputs["reforge_operation"] != in.CorrelationID || fixture.bodyInputs["safe"] != "value" {
		t.Fatalf("reserved inputs were overwritten: %#v", fixture.bodyInputs)
	}
}

func TestDeliveryObserveRejectsMultipleRunsAndReruns(t *testing.T) {
	for _, mode := range []string{"multiple", "rerun"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newDeliveryHTTP()
			fixture.runMode = mode
			p := fixture.provider(t)
			in := fixture.request()
			in.ObserveOnly = true
			if mode == "rerun" {
				in.RunID = "71"
			}
			status, err := p.TriggerOrObservePipeline(context.Background(), in)
			if err == nil || status.State == "success" {
				t.Fatalf("unsafe observation status=%+v err=%v", status, err)
			}
		})
	}
}

type deliveryHTTP struct {
	repository     forge.RepoRef
	environment    string
	source         string
	workflowID     string
	workflowPath   string
	correlation    string
	configHash     string
	rulesDrift     bool
	dispatchStatus int
	posts          int
	bodyInputs     map[string]string
	runMode        string
}

func newDeliveryHTTP() *deliveryHTTP {
	sum := sha256.Sum256([]byte(deliveryConfig))
	return &deliveryHTTP{repository: forge.RepoRef{NativeID: "17", FullName: "acme/repo"}, environment: "production", source: strings.Repeat("a", 40), workflowID: "11", workflowPath: ".github/workflows/deploy.yml", correlation: "11111111-1111-4111-8111-111111111111", configHash: hex.EncodeToString(sum[:]), dispatchStatus: http.StatusNoContent, bodyInputs: map[string]string{}}
}

func (f *deliveryHTTP) provider(t *testing.T) *Provider {
	t.Helper()
	p, err := New(forge.Config{BaseURL: "https://api.github.test/api/v3", Token: "token", Client: roundTripFunc(f.Do)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *deliveryHTTP) request() forge.PipelineRequest {
	return forge.PipelineRequest{Repository: f.repository, WorkflowID: f.workflowID, WorkflowPath: f.workflowPath, ConfigSHA256: f.configHash, Ref: "refs/heads/main", WorkflowSHA: f.source, SourceSHA: f.source, ArtifactDigest: "sha256:" + strings.Repeat("c", 64), Environment: f.environment, CorrelationID: f.correlation, RulesHash: strings.Repeat("d", 64)}
}

func (f *deliveryHTTP) Do(req *http.Request) (*http.Response, error) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v3")
	switch {
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/actions/workflows/11/dispatches"):
		f.posts++
		if req.Body != nil {
			var body struct {
				Inputs map[string]string `json:"inputs"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			f.bodyInputs = body.Inputs
		}
		return deliveryJSON(f.dispatchStatus, nil)
	case path == "/repos/acme/repo":
		return deliveryJSON(200, map[string]any{"id": 17, "full_name": "acme/repo", "default_branch": "main"})
	case path == "/repos/acme/repo/git/ref/heads/main":
		return deliveryJSON(200, map[string]any{"object": map[string]string{"sha": f.source}})
	case path == "/repos/acme/repo/contents/.github/workflows/deploy.yml":
		return deliveryJSON(200, map[string]any{"type": "file", "encoding": "base64", "path": f.workflowPath, "sha": f.source, "content": base64.StdEncoding.EncodeToString([]byte(deliveryConfig))})
	case path == "/repos/acme/repo/actions/workflows/11":
		return deliveryJSON(200, map[string]any{"id": 11, "path": f.workflowPath, "state": "active"})
	case path == "/repos/acme/repo/environments/production":
		wait := 0
		if f.rulesDrift {
			wait = 10
		}
		return deliveryJSON(200, map[string]any{"id": 21, "name": f.environment, "html_url": "https://github.test/acme/repo/environments/production", "protection_rules": []any{map[string]any{"type": "required_reviewers", "prevent_self_review": true, "reviewers": []any{map[string]any{"type": "User", "reviewer": map[string]any{"id": 81}}}}, map[string]any{"type": "wait_timer", "wait_timer": wait}}, "deployment_branch_policy": map[string]any{"protected_branches": true, "custom_branch_policies": false}})
	case path == "/repos/acme/repo/actions/workflows/11/runs":
		if f.runMode == "multiple" {
			return deliveryJSON(200, map[string]any{"total_count": 2, "workflow_runs": []any{deliveryRun(71, f.source, f.correlation, 1), deliveryRun(72, f.source, f.correlation, 1)}})
		}
		return deliveryJSON(200, map[string]any{"total_count": 0, "workflow_runs": []any{}})
	case path == "/repos/acme/repo/actions/runs/71":
		return deliveryJSON(200, deliveryRun(71, f.source, f.correlation, 2))
	default:
		return deliveryJSON(404, nil)
	}
}

func deliveryRun(id int64, source, correlation string, attempt int64) map[string]any {
	return map[string]any{"id": id, "head_sha": source, "head_branch": "main", "path": ".github/workflows/deploy.yml", "event": "workflow_dispatch", "status": "completed", "conclusion": "success", "workflow_id": 11, "display_title": "reforge:" + correlation, "run_attempt": attempt, "html_url": "https://github.test/acme/repo/actions/runs/" + strconv.FormatInt(id, 10), "created_at": "2026-09-21T00:00:00Z", "updated_at": "2026-09-21T00:01:00Z", "repository": map[string]any{"id": 17, "full_name": "acme/repo"}}
}
