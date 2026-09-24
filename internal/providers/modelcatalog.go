package providers

import (
	"context"

	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/model"
	"reforge/internal/model/anthropic"
	"reforge/internal/model/compatible"
	"reforge/internal/model/google"
	"reforge/internal/model/openai"
	"reforge/internal/network"
)

func (f Factory) Catalog(ctx context.Context, r connections.Resolved) ([]connections.CatalogItem, error) {
	c := r.Connection
	if c.Route != nil || r.Client == nil || c.Kind != "model" || c.Settings.BillingRoute != "direct_api" {
		return nil, network.ErrDestination
	}
	cfg := model.Config{Endpoint: c.Endpoint, APIKey: r.Secret, Client: r.Client, Profile: c.Settings.Profile}
	var lister model.ModelLister
	var err error
	switch c.Provider {
	case "openai":
		lister, err = openai.NewCatalog(cfg)
	case "anthropic":
		lister, err = anthropic.NewCatalog(cfg)
	case "google":
		lister, err = google.NewCatalog(cfg)
	case "compatible":
		lister, err = compatible.NewCatalog(cfg)
	default:
		return nil, &domain.ProviderError{Kind: "unsupported", Message: "Configured inference profile is unavailable"}
	}
	if err != nil {
		return nil, err
	}
	models, err := lister.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	gateway := c.Provider == "compatible" && compatible.IsOpenCode(c.Settings.Profile)
	items := make([]connections.CatalogItem, 0, len(models))
	for _, m := range models {
		item := connections.CatalogItem{ID: m.ID, Name: m.Name}
		if gateway && compatible.OpenCodeProtocol(m.ID) == "" {
			item.Disabled, item.Reason = true, "Model family has no documented gateway protocol"
		}
		items = append(items, item)
	}
	return items, nil
}
