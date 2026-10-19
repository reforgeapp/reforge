package compatible

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/model"
)

type gatewayRecorder struct {
	mu       sync.Mutex
	paths    []string
	body     string
	sessions []string
}

func (g *gatewayRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	g.mu.Lock()
	g.paths = append(g.paths, r.URL.Host+r.URL.Path)
	g.sessions = append(g.sessions, r.Header.Get("x-opencode-session"))
	g.mu.Unlock()
	status := http.StatusInternalServerError
	if g.body != "" {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(g.body)), Request: r}, nil
}

func gatewayConfig(profile, id string, recorder *gatewayRecorder) model.Config {
	return model.Config{Endpoint: OpenCodeEndpoint(profile), APIKey: "gateway-key", Model: id, Profile: profile, Client: &http.Client{Transport: recorder}}
}

func TestOpenCodeGatewayRoutesByDocumentedFamily(t *testing.T) {
	cases := map[string]string{
		"gpt-5.1":           "opencode.ai/zen/v1/responses",
		"grok-code":         "opencode.ai/zen/v1/responses",
		"claude-sonnet-4-5": "opencode.ai/zen/v1/messages",
		"qwen3.6-plus":      "opencode.ai/zen/v1/messages",
		"gemini-3-pro":      "opencode.ai/zen/v1/models/gemini-3-pro:streamGenerateContent",
		"glm-4.6":           "opencode.ai/zen/v1/chat/completions",
		"hy3":               "opencode.ai/zen/v1/chat/completions",
	}
	for id, want := range cases {
		recorder := &gatewayRecorder{}
		p, err := New(gatewayConfig(OpenCodeZen, id, recorder))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		_ = p.StreamTurn(context.Background(), model.TurnRequest{Model: id, MaxOutputTokens: 16, Messages: []model.Message{{Role: "user", Text: "hi"}}}, func(model.Event) error { return nil })
		if len(recorder.paths) != 1 || recorder.paths[0] != want {
			t.Fatalf("%s routed to %v", id, recorder.paths)
		}
	}
	recorder := &gatewayRecorder{}
	p, err := New(gatewayConfig(OpenCodeGo, "claude-haiku-4-5", recorder))
	if err != nil {
		t.Fatal(err)
	}
	_ = p.StreamTurn(context.Background(), model.TurnRequest{MaxOutputTokens: 16, Messages: []model.Message{{Role: "user", Text: "hi"}}}, func(model.Event) error { return nil })
	if len(recorder.paths) != 1 || recorder.paths[0] != "opencode.ai/zen/go/v1/messages" {
		t.Fatalf("go gateway routed to %v", recorder.paths)
	}
}

func TestOpenCodeGatewayFailsClosed(t *testing.T) {
	for _, id := range []string{"big-pickle", "qwen3-coder", "gpt", "hy30", ""} {
		recorder := &gatewayRecorder{}
		_, err := New(gatewayConfig(OpenCodeZen, id, recorder))
		var pe *domain.ProviderError
		if !errors.As(err, &pe) || len(recorder.paths) != 0 {
			t.Fatalf("%q: %v", id, err)
		}
	}
	for _, config := range []model.Config{
		{Endpoint: "https://opencode.ai/zen/v1/", Model: "gpt-5", Profile: OpenCodeZen, Client: http.DefaultClient},
		{Endpoint: "https://opencode.ai/zen/go/v1", APIKey: "k", Model: "gpt-5", Profile: OpenCodeZen, Client: http.DefaultClient},
		{Endpoint: "https://evil.example/zen/v1", APIKey: "k", Model: "gpt-5", Profile: OpenCodeZen, Client: http.DefaultClient},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("gateway misconfiguration accepted: %s", config.Endpoint)
		}
	}
	p, err := New(gatewayConfig(OpenCodeZen, "gpt-5", &gatewayRecorder{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.StreamTurn(context.Background(), model.TurnRequest{Model: "big-pickle", MaxOutputTokens: 16, Messages: []model.Message{{Role: "user", Text: "hi"}}}, func(model.Event) error { return nil }); err == nil {
		t.Fatal("turn switched to an unmapped model")
	}
}

func TestOpenCodeProbeUsesCatalogMetadataOnly(t *testing.T) {
	recorder := &gatewayRecorder{body: `{"object":"list","data":[{"id":"claude-sonnet-4-5"},{"id":"gemini-3-pro"}]}`}
	for _, id := range []string{"claude-sonnet-4-5", "gemini-3-pro"} {
		p, err := New(gatewayConfig(OpenCodeZen, id, recorder))
		if err != nil {
			t.Fatal(err)
		}
		caps, err := p.Probe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for name, c := range caps.Features {
			if c.State != domain.Unknown {
				t.Fatalf("%s: metadata verified %s", id, name)
			}
		}
	}
	for _, path := range recorder.paths {
		if path != "opencode.ai/zen/v1/models" {
			t.Fatalf("probe used native metadata endpoint %s", path)
		}
	}
	if _, err := New(gatewayConfig(OpenCodeZen, "", recorder)); err == nil {
		t.Fatal("gateway turn provider built without a model")
	}
	lister, err := NewCatalog(gatewayConfig(OpenCodeZen, "", recorder))
	if err != nil {
		t.Fatal(err)
	}
	if models, err := lister.ListModels(context.Background()); err != nil || len(models) != 2 {
		t.Fatalf("catalog-only listing: %v %v", models, err)
	}
}

func TestOpenCodeGoSendsStableSession(t *testing.T) {
	recorder := &gatewayRecorder{}
	p, err := New(gatewayConfig(OpenCodeGo, "deepseek-v4.1-flash", recorder))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_ = p.StreamTurn(context.Background(), model.TurnRequest{Model: "deepseek-v4.1-flash", MaxOutputTokens: 16, Session: "attempt-1", Messages: []model.Message{{Role: "user", Text: "hi"}}}, func(model.Event) error { return nil })
	}
	if len(recorder.sessions) != 2 || recorder.sessions[0] != "attempt-1" || recorder.sessions[1] != "attempt-1" {
		t.Fatalf("sessions %v", recorder.sessions)
	}
}
