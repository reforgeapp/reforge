package agent

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/store"
)

type ConnectionReader interface {
	Get(context.Context, auth.Session, string, string) (connections.Connection, error)
}

type AuthService struct {
	db             *store.Store
	auth           *auth.Service
	connections    ConnectionReader
	qualifications *QualificationService
	factory        *Factory
}

func NewAuthService(db *store.Store, identity *auth.Service, reader ConnectionReader, qualifications *QualificationService, factory *Factory) *AuthService {
	return &AuthService{db: db, auth: identity, connections: reader, qualifications: qualifications, factory: factory}
}

func BindingFor(connection connections.Connection) Binding {
	runner := ""
	if connection.Route != nil {
		runner = connection.Route.RunnerID
	}
	return Binding{
		OrgID:             connection.OrgID,
		ConnectionID:      connection.ID,
		RunnerID:          runner,
		ConnectionVersion: connection.Version,
		CredentialVersion: connection.CredentialVersion,
		AccountID:         connection.Settings.Namespace,
		Model:             connection.Settings.Model,
		RuntimeDigest:     connection.Settings.RuntimeVersion,
		Deployment:        connection.Settings.Profile,
	}
}

func (s *AuthService) resolve(ctx context.Context, session auth.Session, org, connectionID string) (connections.Connection, domain.Actor, error) {
	var actor domain.Actor
	connection, err := s.connections.Get(ctx, session, org, connectionID)
	if err != nil {
		return connection, actor, err
	}
	if err = s.auth.WithActor(ctx, session, org, func(_ pgx.Tx, a domain.Actor) error { actor = a; return nil }); err != nil {
		return connection, actor, err
	}
	return connection, actor, nil
}

func (s *AuthService) bridge(ctx context.Context, session auth.Session, org string, connection connections.Connection) (*Codex, error) {
	if connection.Kind != "agent" || connection.Provider != "codex" {
		return nil, ErrDisabled
	}
	if s.factory == nil || !s.factory.Configured() {
		return nil, ErrDisabled
	}
	qualification, found, err := s.qualifications.Get(ctx, session, org, connection.ID)
	if err != nil {
		return nil, err
	}
	binding := BindingFor(connection)
	if !found || !qualified(qualification, binding, true) {
		return nil, ErrDisabled
	}
	qualify := func(ctx context.Context, want Binding) (Qualification, error) {
		current, found, err := s.qualifications.Get(ctx, session, org, connection.ID)
		if err != nil {
			return Qualification{}, err
		}
		if !found || !qualified(current, want, true) {
			return Qualification{}, ErrDisabled
		}
		return current, nil
	}
	authorize := func(context.Context, Effect, func() error) error { return ErrDenied }
	authorizeUser := func(ctx context.Context, want Binding, action UserAction, perform func() error) error {
		current, err := s.connections.Get(ctx, session, org, connection.ID)
		if err != nil {
			return err
		}
		if current.Version != want.ConnectionVersion || current.CredentialVersion != want.CredentialVersion {
			return ErrDenied
		}
		claim, err := json.Marshal(map[string]any{"action": action.Kind, "connection_version": current.Version})
		if err != nil {
			return err
		}
		if err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$5,$6)`, domain.NewID(), org, action.UserID, "agent."+action.Kind, connection.ID, claim)
			return e
		}); err != nil {
			return err
		}
		return perform()
	}
	return s.factory.Bridge(binding, qualify, authorize, authorizeUser)
}

func (s *AuthService) Login(ctx context.Context, session auth.Session, org, connectionID string) (Login, error) {
	if session.AutomationID() != "" {
		return Login{}, auth.ErrForbidden
	}
	connection, actor, err := s.resolve(ctx, session, org, connectionID)
	if err != nil {
		return Login{}, err
	}
	if actor.Role != domain.Owner && actor.Role != domain.Admin {
		return Login{}, auth.ErrForbidden
	}
	bridge, err := s.bridge(ctx, session, org, connection)
	if err != nil {
		return Login{}, err
	}
	defer bridge.Close()
	return bridge.ManagedLogin(ctx, UserAction{ID: domain.NewID(), UserID: session.User.ID, Kind: "login"})
}

func (s *AuthService) Logout(ctx context.Context, session auth.Session, org, connectionID string) error {
	if session.AutomationID() != "" {
		return auth.ErrForbidden
	}
	connection, actor, err := s.resolve(ctx, session, org, connectionID)
	if err != nil {
		return err
	}
	if actor.Role != domain.Owner && actor.Role != domain.Admin {
		return auth.ErrForbidden
	}
	bridge, err := s.bridge(ctx, session, org, connection)
	if err != nil {
		return err
	}
	defer bridge.Close()
	return bridge.ManagedLogout(ctx, UserAction{ID: domain.NewID(), UserID: session.User.ID, Kind: "logout"})
}
