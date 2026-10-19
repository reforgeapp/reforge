package compatible

import (
	"context"
	"strings"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/model"
	anthropicmodel "github.com/reforgeapp/reforge/internal/model/anthropic"
	googlemodel "github.com/reforgeapp/reforge/internal/model/google"
	openaimodel "github.com/reforgeapp/reforge/internal/model/openai"
)

const (
	OpenCodeZen = "opencode_zen"
	OpenCodeGo  = "opencode_go"
)

var openCodeEndpoints = map[string]string{
	OpenCodeZen: "https://opencode.ai/zen/v1",
	OpenCodeGo:  "https://opencode.ai/zen/go/v1",
}

var openCodeFamilies = []struct{ prefix, protocol string }{
	{"gpt-", "responses"}, {"grok-", "responses"}, {"muse-spark-", "responses"},
	{"claude-", "messages"}, {"qwen3.", "messages"}, {"minimax-", "messages"},
	{"gemini-", "gemini"},
	{"deepseek-", defaultProfile}, {"glm-", defaultProfile}, {"kimi-", defaultProfile}, {"longcat-", defaultProfile}, {"mimo-", defaultProfile},
	{"hy3-", defaultProfile}, {"hy3.", defaultProfile}, {"hy4-", defaultProfile}, {"hy4.", defaultProfile},
}

func IsOpenCode(profile string) bool {
	_, ok := openCodeEndpoints[profile]
	return ok
}

func OpenCodeEndpoint(profile string) string { return openCodeEndpoints[profile] }

func OpenCodeProtocol(id string) string {
	if id == "hy3" || id == "hy4" {
		return defaultProfile
	}
	for _, family := range openCodeFamilies {
		if strings.HasPrefix(id, family.prefix) && len(id) > len(family.prefix) {
			return family.protocol
		}
	}
	return ""
}

func openCode(config model.Config, p *Provider, requireModel bool) (*Provider, error) {
	if strings.TrimRight(config.Endpoint, "/") != openCodeEndpoints[config.Profile] || strings.TrimSpace(config.APIKey) == "" {
		return nil, &domain.ProviderError{Kind: "configuration", Message: "OpenCode gateway requires its fixed endpoint and an API key"}
	}
	if !requireModel {
		return p, nil
	}
	delegate := config
	var err error
	switch OpenCodeProtocol(config.Model) {
	case "responses":
		delegate.Profile = "responses"
		p.delegate, err = openaimodel.New(delegate)
	case "messages":
		delegate.Profile = "messages"
		p.delegate, err = anthropicmodel.New(delegate)
	case "gemini":
		delegate.Profile = googlemodel.GatewayV1Profile
		p.delegate, err = googlemodel.New(delegate)
	case defaultProfile:
	default:
		return nil, &domain.ProviderError{Kind: "model_unsupported", Message: "model family has no documented gateway protocol"}
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) gatewayProbe(ctx context.Context) (model.Capabilities, error) {
	models, err := p.ListModels(ctx)
	if err != nil {
		return model.Capabilities{}, err
	}
	protocol := OpenCodeProtocol(p.config.Model)
	found := false
	for _, entry := range models {
		found = found || entry.ID == p.config.Model
	}
	if !found || protocol == "" {
		return model.Capabilities{}, &domain.ProviderError{Kind: "model_unsupported", Message: "configured model was not returned by the gateway catalog"}
	}
	features := map[string]domain.Capability{}
	for _, name := range []string{protocol, "streaming", "tool_calling", "usage"} {
		features[name] = capability(domain.Unknown, "public gateway catalog does not verify credentials or inference", "gateway catalog")
	}
	return model.Capabilities{Provider: "compatible", Model: p.config.Model, BillingRoute: "customer_endpoint", Features: features}, nil
}
