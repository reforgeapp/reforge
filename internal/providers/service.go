package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/forge/gitea"
	"reforge/internal/forge/github"
	"reforge/internal/forge/gitlab"
	"reforge/internal/privateconnector"
	"reforge/internal/runner"
	"reforge/internal/store"
)

type Service struct {
	db          *store.Store
	connections *connections.Service
	private     *privateconnector.Connector
	runners     *runner.Service
	factory     Factory
}

func New(db *store.Store, connections *connections.Service, private *privateconnector.Connector, runners *runner.Service, development bool) *Service {
	return &Service{db: db, connections: connections, private: private, runners: runners, factory: Factory{Development: development}}
}

func (s *Service) Read(ctx context.Context, orgID, connectionID string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	var result privateconnector.Result
	if !auth.ValidID(orgID) || !auth.ValidID(connectionID) || authorize == nil || op.Validate() != nil {
		return result, auth.ErrInvalid
	}
	var initial connections.Connection
	err := s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
		var err error
		initial, err = s.connections.MetadataTx(ctx, tx, orgID, connectionID)
		return err
	})
	if err != nil {
		return result, err
	}
	read := func(ctx context.Context, ready *privateconnector.Ready, deliver privateconnector.Deliver) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
			var locked string
			if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR SHARE`, orgID).Scan(&locked); err != nil {
				return err
			}
			current, err := s.connections.MetadataTx(ctx, tx, orgID, connectionID)
			if err != nil {
				return err
			}
			if current.Version != initial.Version || current.State != "healthy" || current.Kind != "forge" && current.Kind != "delivery" {
				return auth.ErrConflict
			}
			runnerID := ""
			if ready != nil {
				if current.Route == nil || current.Route.RevokedAt != nil || current.Route.RunnerID != ready.ID || ready.OrgID != orgID {
					return auth.ErrForbidden
				}
				if err = s.runners.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash); err != nil {
					return err
				}
				runnerID = ready.ID
			} else if current.Route != nil {
				return auth.ErrConflict
			}
			if err = authorize(ctx, tx, current); err != nil {
				return err
			}
			resolved, err := s.connections.ResolveTx(ctx, tx, orgID, connectionID, runnerID)
			if err != nil {
				return err
			}
			if resolved.Client != nil {
				defer resolved.Client.CloseIdleConnections()
			}
			if ready != nil {
				result, err = deliver(privateconnector.GrantSpec{OperationID: op.ID, AuthorityID: op.ID, RunnerVersion: ready.Version, CredentialHash: ready.CredentialHash, Connection: privateConnection(resolved)})
			} else {
				var provider forge.Provider
				provider, err = s.factory.Forge(ctx, resolved)
				if err == nil {
					result, err = privateconnector.ReadForge(ctx, provider, op)
				}
			}
			if err != nil {
				return err
			}
			raw, err := json.Marshal(result)
			if err != nil || len(raw) > privateconnector.MaxResponse || privateconnector.ContainsSecret(raw, resolved.Secret) {
				return privateconnector.ErrInvalid
			}
			return nil
		})
	}
	if initial.Route == nil {
		err = read(ctx, nil, nil)
	} else if s.private == nil || s.runners == nil {
		err = privateconnector.ErrUnavailable
	} else {
		_, err = s.private.Dispatch(ctx, privateconnector.Target{OrgID: orgID, RunnerID: initial.Route.RunnerID}, op, func(ctx context.Context, ready privateconnector.Ready, deliver privateconnector.Deliver) error {
			return read(ctx, &ready, deliver)
		})
	}
	if err != nil {
		return privateconnector.Result{}, err
	}
	return result, nil
}

type denyHTTP struct{}

func (denyHTTP) Do(*http.Request) (*http.Response, error) { return nil, auth.ErrForbidden }

func DecodeWebhook(provider, secret string, headers http.Header, payload []byte) (forge.Event, error) {
	cfg := forge.Config{BaseURL: "https://webhook.invalid", Token: "decode-only", WebhookSecret: secret, Client: denyHTTP{}}
	var decoder forge.ForgeEvents
	var err error
	switch provider {
	case "github":
		decoder, err = github.New(cfg)
	case "gitlab":
		decoder, err = gitlab.New(cfg)
	case "gitea":
		decoder, err = gitea.New(cfg)
	default:
		return forge.Event{}, &domain.ProviderError{Kind: "unsupported", Message: "Webhook provider is unsupported"}
	}
	if err != nil {
		return forge.Event{}, err
	}
	return decoder.DecodeEvent(headers, payload)
}
