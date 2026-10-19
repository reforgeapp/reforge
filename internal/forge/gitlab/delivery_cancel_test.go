package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

func TestCancelPipelineCancelsExactPipelineOnceAndReobserves(t *testing.T) {
	f := newCancelGitLabHTTP(t)
	defer f.server.Close()
	p := f.provider(t).WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error {
		atomic.AddInt64(&f.guards, 1)
		return nil
	})
	status, err := p.CancelPipeline(context.Background(), f.request())
	if err != nil || status.State != "cancelled" || atomic.LoadInt64(&f.cancelPosts) != 1 || atomic.LoadInt64(&f.pipelineGets) < 2 || atomic.LoadInt64(&f.guards) != 1 {
		t.Fatalf("status=%+v err=%v cancel_posts=%d pipeline_gets=%d guards=%d", status, err, atomic.LoadInt64(&f.cancelPosts), atomic.LoadInt64(&f.pipelineGets), atomic.LoadInt64(&f.guards))
	}
}

func TestCancelPipelineTerminalAndInvalidEvidenceDoNotMutate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		state  string
		mutate func(*cancelGitLabHTTP, *forge.PipelineRequest)
	}{
		{name: "terminal", state: "success", mutate: func(_ *cancelGitLabHTTP, _ *forge.PipelineRequest) {}},
		{name: "wrong correlation", state: "running", mutate: func(_ *cancelGitLabHTTP, in *forge.PipelineRequest) {
			in.CorrelationID = "22222222-2222-4222-8222-222222222222"
		}},
		{name: "wrong sha", state: "running", mutate: func(_ *cancelGitLabHTTP, in *forge.PipelineRequest) { in.WorkflowSHA = strings.Repeat("b", 40) }},
		{name: "wrong attempt", state: "running", mutate: func(f *cancelGitLabHTTP, _ *forge.PipelineRequest) { f.retried = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCancelGitLabHTTP(t)
			defer f.server.Close()
			f.state = tc.state
			in := f.request()
			tc.mutate(f, &in)
			p := f.provider(t).WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error {
				t.Fatal("terminal or mismatched pipeline invoked guard")
				return nil
			})
			status, err := p.CancelPipeline(context.Background(), in)
			if tc.name == "terminal" {
				if err != nil || status.State != "success" || atomic.LoadInt64(&f.cancelPosts) != 0 {
					t.Fatalf("status=%+v err=%v posts=%d", status, err, atomic.LoadInt64(&f.cancelPosts))
				}
				return
			}
			if err == nil || atomic.LoadInt64(&f.cancelPosts) != 0 {
				t.Fatalf("status=%+v err=%v posts=%d", status, err, atomic.LoadInt64(&f.cancelPosts))
			}
		})
	}
}

func TestCancelPipelineRequiresGuardAndSurfacesLostResponse(t *testing.T) {
	f := newCancelGitLabHTTP(t)
	defer f.server.Close()
	if _, err := f.provider(t).CancelPipeline(context.Background(), f.request()); err == nil || atomic.LoadInt64(&f.cancelPosts) != 0 {
		t.Fatalf("missing guard err=%v posts=%d", err, atomic.LoadInt64(&f.cancelPosts))
	}
	f.loseCancel = true
	p := f.provider(t).WithDeliveryGuard(func(context.Context, forge.PipelineRequest, forge.DeploymentGates) error { return nil })
	_, err := p.CancelPipeline(context.Background(), f.request())
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain || atomic.LoadInt64(&f.cancelPosts) != 1 {
		t.Fatalf("lost response err=%v posts=%d", err, atomic.LoadInt64(&f.cancelPosts))
	}
}

type cancelGitLabHTTP struct {
	server                            *httptest.Server
	source, correlation, state        string
	retried, loseCancel               bool
	pipelineGets, cancelPosts, guards int64
}

func newCancelGitLabHTTP(t *testing.T) *cancelGitLabHTTP {
	f := &cancelGitLabHTTP{source: strings.Repeat("a", 40), correlation: "11111111-1111-4111-1111-111111111111", state: "running"}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}
func (f *cancelGitLabHTTP) request() forge.PipelineRequest {
	return forge.PipelineRequest{Repository: forge.RepoRef{NativeID: "17", FullName: "group/project"}, WorkflowID: "pipeline", WorkflowPath: ".gitlab-ci.yml", ConfigSHA256: strings.Repeat("c", 64), Ref: "refs/heads/main", WorkflowSHA: f.source, SourceSHA: f.source, ArtifactDigest: "sha256:" + strings.Repeat("d", 64), Environment: "production", CorrelationID: f.correlation, RulesHash: strings.Repeat("e", 64), RunID: "46"}
}
func (f *cancelGitLabHTTP) provider(t *testing.T) *Provider {
	t.Helper()
	p, err := New(forge.Config{BaseURL: f.server.URL + "/api/v4", Token: "token", Client: f.server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f *cancelGitLabHTTP) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/17" {
		_, _ = w.Write([]byte(`{"id":17,"path_with_namespace":"group/project","default_branch":"main","web_url":"https://gitlab.test/group/project","ci_config_path":".gitlab-ci.yml"}`))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/17/pipelines/46" {
		atomic.AddInt64(&f.pipelineGets, 1)
		_, _ = w.Write([]byte(`{"id":46,"project_id":17,"sha":"` + f.source + `","status":"` + f.state + `","ref":"main","source":"api","web_url":"https://gitlab.test/group/project/-/pipelines/46","created_at":"2026-09-21T00:00:00Z","updated_at":"2026-09-21T00:01:00Z"}`))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/17/pipelines/46/bridges" {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/17/pipelines/46/jobs" {
		retried := "false"
		if f.retried {
			retried = "true"
		}
		_, _ = w.Write([]byte(`[{"id":9,"name":"test","stage":"test","status":"success","retried":` + retried + `,"pipeline":{"id":46,"project_id":17,"sha":"` + f.source + `"},"commit":{"id":"` + f.source + `"}}]`))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/17/pipelines/46/variables" {
		_, _ = w.Write([]byte(`[{"key":"REFORGE_OPERATION","value":"` + f.correlation + `"}]`))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/17/protected_environments" {
		_, _ = w.Write([]byte(`[{"name":"production","deploy_access_levels":[{"access_level":40}],"required_approval_count":1,"approval_rules":[{"required_approvals":1}]}]`))
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/17/pipelines/46/cancel" {
		atomic.AddInt64(&f.cancelPosts, 1)
		if f.loseCancel {
			hijacker, ok := w.(http.Hijacker)
			if ok {
				connection, _, _ := hijacker.Hijack()
				_ = connection.Close()
			}
			return
		}
		f.state = "canceled"
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":46}`))
		return
	}
	http.NotFound(w, r)
}
