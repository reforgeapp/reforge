package connections

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/network"
)

var ErrCatalogUnavailable = errors.New("model catalog unavailable")

const (
	catalogTimeout  = 10 * time.Second
	maxCatalogItems = 2000
	maxCatalogText  = 512
)

func (s *Service) RegisterCatalog(catalog Cataloger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.catalog = catalog
}

func (s *Service) ModelCatalog(ctx context.Context, session auth.Session, orgID string, r CatalogRequest) ([]CatalogItem, error) {
	err := s.auth.WithActor(ctx, session, orgID, func(_ pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	setup := CreateRequest{Kind: "model", Provider: r.Provider, Endpoint: r.Endpoint, Secret: r.Secret, Settings: Settings{Model: r.Settings.Model, Profile: r.Settings.Profile, AuthKind: r.Settings.AuthKind, BillingRoute: r.Settings.BillingRoute, CAPEM: r.Settings.CAPEM}}
	if !validBounds(setup) || !validModel(setup, false) {
		return nil, auth.ErrInvalid
	}
	c := Connection{OrgID: orgID, Kind: "model", Provider: r.Provider, Endpoint: r.Endpoint, Settings: setup.Settings}
	if _, err := network.ValidateEndpoint(c.Endpoint, s.options(c, "")); err != nil {
		return nil, network.ErrDestination
	}
	s.mu.RLock()
	catalog := s.catalog
	s.mu.RUnlock()
	if catalog == nil {
		return nil, ErrCatalogUnavailable
	}
	client, err := network.NewClient(c.Endpoint, s.options(c, ""))
	if err != nil {
		return nil, network.ErrDestination
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	items, err := catalog(ctx, Resolved{Connection: c, Secret: r.Secret, Client: client})
	if err != nil || len(items) > maxCatalogItems {
		return nil, ErrCatalogUnavailable
	}
	out := make([]CatalogItem, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if !catalogText(item.ID) || item.ID == "" || seen[item.ID] || (r.Secret != "" && strings.Contains(item.ID, r.Secret)) {
			continue
		}
		seen[item.ID] = true
		if !catalogText(item.Name) || item.Name == "" || (r.Secret != "" && strings.Contains(item.Name, r.Secret)) {
			item.Name = item.ID
		}
		if !catalogText(item.Reason) || (r.Secret != "" && strings.Contains(item.Reason, r.Secret)) {
			item.Reason = ""
		}
		out = append(out, item)
	}
	return out, nil
}

func catalogText(value string) bool {
	if len(value) > maxCatalogText {
		return false
	}
	for _, r := range value {
		if r == unicode.ReplacementChar || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
