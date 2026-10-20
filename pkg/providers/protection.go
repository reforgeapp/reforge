package providers

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

func (s Service) ForProtection(id string, publishers map[string]string) Client {
	s.protectionID = id
	s.publishers = make(map[string]string, len(publishers))
	for name, publisher := range publishers {
		s.publishers[name] = publisher
	}
	return &s
}

func sameProtectionRoute(a, b connections.Connection) bool {
	if a.OrgID != b.OrgID || a.ID == b.ID || a.Endpoint != b.Endpoint || a.Provider != "gitea" || b.Provider != "gitea" || b.Kind != "forge" || b.State != "healthy" || a.Settings.CAPEM != b.Settings.CAPEM {
		return false
	}
	if a.Route == nil || b.Route == nil {
		return a.Route == nil && b.Route == nil
	}
	return a.Route.RevokedAt == nil && b.Route.RevokedAt == nil && a.Route.RunnerID == b.Route.RunnerID && a.Route.Host == b.Route.Host && slices.Equal(a.Route.CIDRs, b.Route.CIDRs)
}

func (s *Service) resolveProtectionTx(ctx context.Context, tx pgx.Tx, resolved *connections.Resolved, runnerID string) error {
	resolved.CheckPublishers = s.publishers
	if s.protectionID == "" {
		return nil
	}
	if !auth.ValidID(s.protectionID) {
		return auth.ErrInvalid
	}
	meta, err := s.connections.MetadataTx(ctx, tx, resolved.Connection.OrgID, s.protectionID)
	if err != nil {
		return err
	}
	if !sameProtectionRoute(resolved.Connection, meta) {
		return auth.ErrForbidden
	}
	reader, err := s.connections.ResolveTx(ctx, tx, meta.OrgID, meta.ID, runnerID)
	if err != nil {
		return err
	}
	resolved.Protection = &reader
	return nil
}

func (s *Service) checkProtectionTx(ctx context.Context, tx pgx.Tx, resolved connections.Resolved) error {
	if resolved.Protection == nil {
		return nil
	}
	expected := resolved.Protection.Connection
	current, err := s.connections.MetadataTx(ctx, tx, expected.OrgID, expected.ID)
	if err != nil {
		return err
	}
	if !sameProtectionRoute(resolved.Connection, current) || current.Version != expected.Version || current.CredentialVersion != expected.CredentialVersion {
		return auth.ErrConflict
	}
	return nil
}

func closeProtection(r *connections.Resolved) {
	if r.Protection != nil {
		r.Protection.Secret = ""
		if r.Protection.Client != nil {
			r.Protection.Client.CloseIdleConnections()
		}
	}
}

func exposesCredential(raw []byte, r connections.Resolved) bool {
	return privateconnector.ContainsSecret(raw, r.Secret) || r.Protection != nil && privateconnector.ContainsSecret(raw, r.Protection.Secret)
}
