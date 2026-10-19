package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

func TestCancelPipelineCancelsExactRunOnceAndReobserves(t *testing.T) {
	f := newCancelGitHubHTTP()
	p := f.provider(t).WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error { f.guards++; return nil })
	status, err := p.CancelPipeline(context.Background(), f.request())
	if err != nil || status.State != "cancelled" || f.cancelPosts != 1 || f.runGets < 2 || f.guards != 1 {
		t.Fatalf("status=%+v err=%v cancel_posts=%d run_gets=%d guards=%d", status, err, f.cancelPosts, f.runGets, f.guards)
	}
}

func TestCancelPipelineTerminalRunDoesNotMutate(t *testing.T) {
	f := newCancelGitHubHTTP()
	f.state = "success"
	f.conclusion = "success"
	p := f.provider(t).WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error {
		t.Fatal("terminal run invoked guard")
		return nil
	})
	status, err := p.CancelPipeline(context.Background(), f.request())
	if err != nil || status.State != "success" || f.cancelPosts != 0 {
		t.Fatalf("status=%+v err=%v cancel_posts=%d", status, err, f.cancelPosts)
	}
}

func TestCancelPipelineRejectsIdentityGuardAndUnknownResponse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*cancelGitHubHTTP, *forge.PipelineRequest)
		guard  bool
		lost   bool
	}{
		{name: "wrong correlation", guard: true, mutate: func(_ *cancelGitHubHTTP, in *forge.PipelineRequest) {
			in.CorrelationID = "22222222-2222-4222-8222-222222222222"
		}},
		{name: "wrong sha", guard: true, mutate: func(_ *cancelGitHubHTTP, in *forge.PipelineRequest) { in.WorkflowSHA = strings.Repeat("b", 40) }},
		{name: "wrong attempt", guard: true, mutate: func(f *cancelGitHubHTTP, _ *forge.PipelineRequest) { f.attempt = 2 }},
		{name: "guard failure", guard: true, mutate: func(_ *cancelGitHubHTTP, _ *forge.PipelineRequest) {}},
		{name: "missing guard", mutate: func(_ *cancelGitHubHTTP, _ *forge.PipelineRequest) {}},
		{name: "lost response", guard: true, lost: true, mutate: func(_ *cancelGitHubHTTP, _ *forge.PipelineRequest) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCancelGitHubHTTP()
			in := f.request()
			tc.mutate(f, &in)
			var p *Provider
			if tc.guard {
				p = f.provider(t).WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error {
					if tc.name == "guard failure" {
						return errors.New("native guard denied")
					}
					return nil
				})
			} else {
				p = f.provider(t)
			}
			f.loseCancel = tc.lost
			_, err := p.CancelPipeline(context.Background(), in)
			if err == nil || tc.lost && f.cancelPosts != 1 {
				t.Fatalf("err=%v cancel_posts=%d", err, f.cancelPosts)
			}
			if tc.lost {
				var providerErr *domain.ProviderError
				if !errors.As(err, &providerErr) || !providerErr.Uncertain {
					t.Fatalf("lost response err=%v", err)
				}
			} else if f.cancelPosts != 0 {
				t.Fatalf("rejected request mutated native run: %d", f.cancelPosts)
			}
		})
	}
}

type cancelGitHubHTTP struct {
	source, correlation, state, conclusion string
	attempt, runGets, cancelPosts, guards  int
	loseCancel                             bool
}

func newCancelGitHubHTTP() *cancelGitHubHTTP {
	return &cancelGitHubHTTP{source: strings.Repeat("a", 40), correlation: "11111111-1111-4111-8111-111111111111", state: "running", attempt: 1}
}
func (f *cancelGitHubHTTP) request() forge.PipelineRequest {
	return forge.PipelineRequest{Repository: forge.RepoRef{NativeID: "17", FullName: "acme/repo"}, WorkflowID: "11", WorkflowPath: ".github/workflows/deploy.yml", ConfigSHA256: strings.Repeat("c", 64), Ref: "refs/heads/main", WorkflowSHA: f.source, SourceSHA: f.source, ArtifactDigest: "sha256:" + strings.Repeat("d", 64), Environment: "production", CorrelationID: f.correlation, RulesHash: strings.Repeat("e", 64), RunID: "71"}
}
func (f *cancelGitHubHTTP) provider(t *testing.T) *Provider {
	t.Helper()
	p, err := New(forge.Config{BaseURL: "https://api.github.test/api/v3", Token: "token", Client: roundTripFunc(f.do)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f *cancelGitHubHTTP) do(req *http.Request) (*http.Response, error) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v3")
	if path == "/repos/acme/repo/actions/runs/71" && req.Method == http.MethodGet {
		f.runGets++
		conclusion := any(nil)
		status := "in_progress"
		if f.state == "success" {
			status, conclusion = "completed", f.conclusion
		}
		if f.state == "cancelled" {
			status, conclusion = "completed", "cancelled"
		}
		return deliveryJSON(200, map[string]any{"id": 71, "head_sha": f.source, "head_branch": "main", "path": ".github/workflows/deploy.yml", "event": "workflow_dispatch", "status": status, "conclusion": conclusion, "workflow_id": 11, "display_title": "reforge:" + f.correlation, "run_attempt": f.attempt, "html_url": "https://github.test/acme/repo/actions/runs/71", "created_at": "2026-09-21T00:00:00Z", "updated_at": "2026-09-21T00:01:00Z", "repository": map[string]any{"id": 17, "full_name": "acme/repo"}})
	}
	if path == "/repos/acme/repo/actions/runs/71/cancel" && req.Method == http.MethodPost {
		f.cancelPosts++
		if f.loseCancel {
			return nil, errors.New("connection reset after cancellation")
		}
		f.state = "cancelled"
		return deliveryJSON(http.StatusAccepted, nil)
	}
	return deliveryJSON(http.StatusNotFound, nil)
}
