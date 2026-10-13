package budget

import (
	"context"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

type PricingPreset struct {
	ID                       string `json:"id"`
	Provider                 string `json:"provider"`
	Model                    string `json:"model"`
	Source                   string `json:"source"`
	CheckedAt                string `json:"checked_at"`
	InputMicroUSDPerMillion  int64  `json:"input_micro_usd_per_million"`
	OutputMicroUSDPerMillion int64  `json:"output_micro_usd_per_million"`
}

var pricingPresets = []PricingPreset{
	{"openai-standard-ceiling-2026-10-13-astra", "openai", "gpt-6-astra", "https://developers.openai.com/api/docs/pricing", "2026-10-13", 25_000_000, 75_000_000},
	{"openai-standard-ceiling-2026-10-13-sol", "openai", "gpt-6.1-sol", "https://developers.openai.com/api/docs/pricing", "2026-10-13", 5_000_000, 15_000_000},
	{"openai-standard-ceiling-2026-10-13-luna", "openai", "gpt-6-luna", "https://developers.openai.com/api/docs/pricing", "2026-10-13", 250_000, 750_000},
}

type PricingOptions struct {
	Presets   []PricingPreset `json:"presets"`
	Manual    bool            `json:"manual"`
	Automatic bool            `json:"automatic"`
	Reason    string          `json:"reason"`
}

func pricePreset(id string) (PricingPreset, bool) {
	for _, p := range pricingPresets {
		if p.ID == id {
			return p, true
		}
	}
	return PricingPreset{}, false
}

func presetEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "api.openai.com" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/v1" || u.Path == "/v1/")
}

func (s *Service) Pricing(ctx context.Context, session auth.Session, org, connection string) (PricingOptions, error) {
	out := PricingOptions{Presets: []PricingPreset{}}
	if !auth.ValidID(connection) {
		return out, ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		var kind, provider, endpoint, model, state string
		if err := tx.QueryRow(ctx, `SELECT kind,provider,endpoint,coalesce(settings->>'model',''),state FROM connections WHERE org_id=$1 AND id=$2`, org, connection).Scan(&kind, &provider, &endpoint, &model, &state); err != nil {
			return auth.ErrForbidden
		}
		for _, p := range pricingPresets {
			if provider == p.Provider && model == p.Model && presetEndpoint(endpoint) {
				out.Presets = append(out.Presets, p)
			}
		}
		if kind == "agent" && provider != "custom_command" {
			out.Reason = "Runtime account, custody and quota qualification missing; no paid fallback"
			return nil
		}
		if state != "healthy" {
			out.Reason = "Reconnect and verify this model before maintenance"
			return nil
		}
		r, err := loadRoute(ctx, tx, org, connection, model, "default")
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !validRoute(r) {
			out.Reason = "Known pricing and bounded billing route required"
			return nil
		}
		if err != nil {
			return err
		}
		if r.Paused {
			out.Reason = "Resume this billing route"
			return nil
		}
		switch {
		case kind == "model" && AutomaticEligible(r):
			out.Manual, out.Automatic = true, true
			out.Reason = "Manual repairs and automatic maintenance; approved spending ceilings apply"
		case kind == "agent" && provider == "custom_command" && r.Mode == "quota" && r.Qualified && r.QualificationRef != "":
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM custom_profiles WHERE org_id=$1 AND approved_at IS NOT NULL AND revoked_at IS NULL)`, org).Scan(&out.Manual); err != nil {
				return err
			}
			out.Reason = "Manual repairs require an approved custom profile; automatic maintenance unavailable; no paid fallback"
		default:
			out.Reason = "Runtime and quota qualification required; no paid fallback"
		}
		return nil
	})
	return out, err
}

func (s *Service) ApplyPricingPreset(ctx context.Context, session auth.Session, org, connection, id string, expected int64, request string) (Route, error) {
	p, ok := pricePreset(id)
	if !ok {
		return Route{}, ErrInvalid
	}
	r := Route{ConnectionID: connection, Model: p.Model, Name: "default", Mode: "priced", PricingVersion: p.ID, PricingPresetID: p.ID, PricingSource: p.Source, PricingCheckedAt: p.CheckedAt, CostBasis: "published_ceiling", InputMicroUSDPerMillion: p.InputMicroUSDPerMillion, OutputMicroUSDPerMillion: p.OutputMicroUSDPerMillion, MaxInputTokens: model.MaxRequestBytes/2 + 4096, MaxOutputTokens: 4096, MaxMilliseconds: 600_000, MaxRequests: 1}
	return s.PutRoute(ctx, session, org, r, expected, request)
}

func pricingMetadataTx(ctx context.Context, tx pgx.Tx, org string, r *Route) error {
	if r.PricingPresetID == "" {
		r.PricingSource = ""
		r.PricingCheckedAt = ""
		r.CostBasis = "operator_override"
		return nil
	}
	p, ok := pricePreset(r.PricingPresetID)
	if !ok || r.Mode != "priced" || r.PricingVersion != p.ID || r.Model != p.Model || r.InputMicroUSDPerMillion != p.InputMicroUSDPerMillion || r.OutputMicroUSDPerMillion != p.OutputMicroUSDPerMillion || r.RequestMicroUSD != 0 || r.MaxInputTokens > model.MaxRequestBytes/2+4096 || r.MaxOutputTokens > 4096 {
		return ErrInvalid
	}
	var kind, provider, endpoint, model string
	if err := tx.QueryRow(ctx, `SELECT kind,provider,endpoint,coalesce(settings->>'model','') FROM connections WHERE org_id=$1 AND id=$2`, org, r.ConnectionID).Scan(&kind, &provider, &endpoint, &model); err != nil {
		return err
	}
	if kind != "model" || provider != p.Provider || model != p.Model || !presetEndpoint(endpoint) {
		return ErrInvalid
	}
	r.PricingSource, r.PricingCheckedAt, r.CostBasis = p.Source, p.CheckedAt, "published_ceiling"
	return nil
}

func AutomaticEligible(r Route) bool { return validRoute(r) && r.Mode == "priced" && !r.Paused }

func AutomaticRouteSQL(alias string) string {
	return alias + `.config @? '$ ? (@.mode == "priced" && @.pricing_version != "" && @.max_input_tokens > 0 && @.max_output_tokens > 0 && @.max_milliseconds > 0 && @.max_requests == 1 && @.input_micro_usd_per_million >= 0 && @.output_micro_usd_per_million >= 0 && @.request_micro_usd >= 0 && @.paused == false)' AND ` + alias + `.config->>'connection_id'=` + alias + `.connection_id::text AND ` + alias + `.config->>'model'=` + alias + `.model AND ` + alias + `.config->>'name'=` + alias + `.name`
}
