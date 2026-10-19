package providers

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/connections"
)

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func catalogResolved(provider, endpoint, profile string, fn catalogTransport) connections.Resolved {
	return connections.Resolved{
		Connection: connections.Connection{Kind: "model", Provider: provider, Endpoint: endpoint, Settings: connections.Settings{Profile: profile, AuthKind: "api_key", BillingRoute: "direct_api"}},
		Secret:     "catalog-secret",
		Client:     &http.Client{Transport: fn},
	}
}

func reply(status int, body string) (*http.Response, error) {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestCatalogOpenCodeMarksUndocumentedFamiliesDisabled(t *testing.T) {
	var paths []string
	r := catalogResolved("compatible", "https://opencode.ai/zen/v1", "opencode_zen", func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		return reply(200, `{"object":"list","data":[{"id":"gpt-5.1"},{"id":"claude-sonnet-4-5"},{"id":"gemini-3-pro"},{"id":"kimi-k2"},{"id":"big-pickle"}]}`)
	})
	items, err := Factory{}.Catalog(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/zen/v1/models" {
		t.Fatalf("gateway catalog path: %v", paths)
	}
	for _, item := range items {
		if item.Disabled != (item.ID == "big-pickle") {
			t.Fatalf("family gate wrong for %+v", item)
		}
	}
}

func TestCatalogFailuresAreBoundedAndSanitized(t *testing.T) {
	cases := map[string]catalogTransport{
		"unauthorized": func(*http.Request) (*http.Response, error) {
			return reply(401, `{"error":"bad key catalog-secret"}`)
		},
		"oversized": func(*http.Request) (*http.Response, error) {
			return reply(200, `{"data":[{"id":"`+strings.Repeat("x", 9<<20)+`"}]}`)
		},
		"timeout": func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
	}
	for name, fn := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		started := time.Now()
		_, err := Factory{}.Catalog(ctx, catalogResolved("openai", "https://api.openai.example", "", fn))
		cancel()
		if err == nil || strings.Contains(err.Error(), "catalog-secret") || strings.Contains(err.Error(), "bad key") || time.Since(started) > 2*time.Second {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := (Factory{}).Catalog(context.Background(), catalogResolved("compatible", "https://opencode.example/zen/v1", "opencode_zen", func(*http.Request) (*http.Response, error) {
		t.Fatal("unfixed gateway endpoint contacted")
		return nil, nil
	})); err == nil {
		t.Fatal("gateway endpoint override accepted")
	}
}

func TestCatalogEveryProfileWorksWithoutModel(t *testing.T) {
	cases := []struct{ provider, endpoint, profile, path string }{
		{"openai", "https://api.openai.example", "", "/v1/models"},
		{"anthropic", "https://api.anthropic.example", "", "/v1/models"},
		{"google", "https://gemini.example", "", "/v1beta/models"},
		{"google", "https://gemini.example", "gemini_api", "/v1beta/models"},
		{"google", "https://gemini.example", "gateway_v1", "/v1/models"},
		{"compatible", "https://llm.example/v1", "", "/v1/models"},
		{"compatible", "https://llm.example/v1", "ollama", "/v1/models"},
		{"compatible", "https://llm.example/v1", "vllm", "/v1/models"},
		{"compatible", "https://llm.example/v1", "responses", "/v1/models"},
		{"compatible", "https://opencode.ai/zen/v1", "opencode_zen", "/zen/v1/models"},
		{"compatible", "https://opencode.ai/zen/go/v1", "opencode_go", "/zen/go/v1/models"},
	}
	for _, tc := range cases {
		var paths []string
		r := catalogResolved(tc.provider, tc.endpoint, tc.profile, func(req *http.Request) (*http.Response, error) {
			paths = append(paths, req.URL.Path)
			return reply(200, `{"data":[{"id":"gpt-5.1","display_name":"GPT"}],"has_more":false,"models":[{"name":"models/gpt-5.1"}]}`)
		})
		items, err := Factory{}.Catalog(context.Background(), r)
		if err != nil || len(items) != 1 || items[0].ID != "gpt-5.1" || len(paths) != 1 || paths[0] != tc.path {
			t.Fatalf("%s/%s: items=%+v paths=%v err=%v", tc.provider, tc.profile, items, paths, err)
		}
	}
}
