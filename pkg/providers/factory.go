package providers

import (
	"context"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/forge/gitea"
	"github.com/reforgeapp/reforge/pkg/forge/github"
	"github.com/reforgeapp/reforge/pkg/forge/gitlab"
	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/model/anthropic"
	"github.com/reforgeapp/reforge/pkg/model/compatible"
	"github.com/reforgeapp/reforge/pkg/model/google"
	"github.com/reforgeapp/reforge/pkg/model/openai"
	"github.com/reforgeapp/reforge/pkg/network"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"net/url"
)

type Factory struct{ Development bool }

func (f Factory) Forge(ctx context.Context, r connections.Resolved, checks ...func(context.Context) error) (forge.Provider, error) {
	c := r.Connection
	if c.Route != nil || r.Client == nil || (c.Kind != "forge" && c.Kind != "delivery") {
		return nil, network.ErrDestination
	}
	var client forge.HTTPClient = r.Client
	if len(checks) == 1 {
		client = privateconnector.GuardHTTP(client, checks[0])
	}
	cfg := forge.Config{OrgID: c.OrgID, ConnectionID: c.ID, BaseURL: c.Endpoint, Token: r.Secret, Client: client, ServerVersion: c.ServerVersion}
	switch c.Provider {
	case "github":
		var p *github.Provider
		var err error
		if c.Settings.GitHubApp() {
			p, err = github.NewApp(ctx, cfg, github.AppConfig{AppID: c.Settings.AppID, InstallationID: c.Settings.InstallationID, PrivateKeyPEM: []byte(r.Secret)})
		} else {
			p, err = github.New(cfg)
		}
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(c.Endpoint)
		if err != nil {
			return nil, network.ErrDestination
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/graphql"
		} else if u.Path == "/api/v3" || u.Path == "/api/v3/" {
			u.Path = "/api/graphql"
		} else {
			return nil, &domain.ProviderError{Kind: "configuration", Message: "GitHub endpoint must be the cloud REST origin or GHES /api/v3"}
		}
		client, err := network.NewClient(u.String(), network.Options{CAPEM: []byte(c.Settings.CAPEM), Development: f.Development})
		if err != nil {
			return nil, err
		}
		var graph forge.HTTPClient = client
		if len(checks) == 1 {
			graph = privateconnector.GuardHTTP(graph, checks[0])
		}
		return p.WithGraphQL(u.String(), graph)
	case "gitlab":
		return gitlab.New(cfg)
	case "gitea":
		p, err := gitea.New(cfg)
		if err != nil {
			return nil, err
		}
		p = p.WithCheckPublishers(r.CheckPublishers)
		if r.Protection != nil {
			readerConfig := cfg
			readerConfig.ConnectionID = r.Protection.Connection.ID
			readerConfig.Token = r.Protection.Secret
			readerConfig.Client = privateconnector.ProtectionHTTP(r.Client)
			reader, err := gitea.New(readerConfig)
			if err != nil {
				return nil, err
			}
			return p.WithProtectionReader(reader)
		}
		return p, nil
	default:
		return nil, &domain.ProviderError{Kind: "unsupported", Message: "Forge provider is not supported"}
	}
}
func (f Factory) Model(r connections.Resolved) (model.ModelProvider, error) {
	c := r.Connection
	if c.Route != nil || r.Client == nil || c.Kind != "model" || c.Settings.BillingRoute != "direct_api" {
		return nil, network.ErrDestination
	}
	cfg := model.Config{Endpoint: c.Endpoint, APIKey: r.Secret, Model: c.Settings.Model, Client: r.Client, Profile: c.Settings.Profile}
	switch c.Provider {
	case "openai":
		return openai.New(cfg)
	case "anthropic":
		return anthropic.New(cfg)
	case "google":
		return google.New(cfg)
	case "compatible":
		return compatible.New(cfg)
	default:
		return nil, &domain.ProviderError{Kind: "unsupported", Message: "Configured inference profile is unavailable"}
	}
}
func (f Factory) Probe(ctx context.Context, r connections.Resolved) (connections.ProbeResult, error) {
	out := connections.ProbeResult{State: "healthy", Reason: "Authenticated metadata verified; each write and model turn remains subject to policy and qualification"}
	if r.Connection.Kind == "forge" || r.Connection.Kind == "delivery" {
		p, err := f.Forge(ctx, r)
		if err != nil {
			return out, err
		}
		if closer, ok := p.(interface{ CloseIdleConnections() }); ok {
			defer closer.CloseIdleConnections()
		}
		caps, err := p.ProbeCapabilities(ctx)
		out.Capabilities, out.ServerVersion = caps.Features, caps.ServerVersion
		return out, err
	}
	p, err := f.Model(r)
	if err != nil {
		return out, err
	}
	caps, err := p.Probe(ctx)
	out.Reason = "Endpoint metadata returned; inference capabilities and usage limits need a qualified model turn"
	if r.Connection.Provider == "compatible" && compatible.IsOpenCode(r.Connection.Settings.Profile) {
		out.Reason = "Public gateway catalog listed the model; API key and inference remain unverified until a qualified model turn"
	}
	out.Capabilities = caps.Features
	return out, err
}
func (f Factory) Register(service *connections.Service) {
	for _, kind := range []string{"forge", "delivery"} {
		for _, provider := range []string{"github", "gitlab", "gitea"} {
			service.Register(kind, provider, f.Probe)
		}
	}
	for _, provider := range []string{"openai", "anthropic", "google", "compatible"} {
		service.Register("model", provider, f.Probe)
	}
	service.RegisterCatalog(f.Catalog)
}
