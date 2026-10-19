package agentcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/agent"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/store"
)

type Service struct {
	db          *store.Store
	auth        *auth.Service
	connections *connections.Service
	runners     *runner.Service
	private     *privateconnector.Connector
	enabled     bool
	policies    *policy.Service
}

func New(db *store.Store, identity *auth.Service, connections *connections.Service, runners *runner.Service, private *privateconnector.Connector, policies *policy.Service, enabled bool) *Service {
	return &Service{db: db, auth: identity, connections: connections, runners: runners, private: private, policies: policies, enabled: enabled}
}

func (s *Service) UseDefault(ctx context.Context, session auth.Session, org, id string) error {
	c, err := s.connection(ctx, session, org, id)
	if err != nil {
		return err
	}
	if c.State != "healthy" || c.Settings.Model == "" {
		return agent.ErrDisabled
	}
	return s.policies.UseModelDefault(ctx, session, org, c.ID, c.Settings.Model, "default")
}

type Options struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func nativeRunner(ctx context.Context, tx pgx.Tx, org string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT r.id::text FROM runners r JOIN runner_pools p ON p.org_id=r.org_id AND p.id=r.pool_id WHERE r.org_id=$1 AND p.builtin AND p.state='active' AND r.state='active' AND r.credential_expires_at>clock_timestamp() AND r.last_seen_at>clock_timestamp()-interval '5 minutes' ORDER BY r.last_seen_at DESC LIMIT 1`, org).Scan(&id)
	return id, err
}

func (s *Service) Options(ctx context.Context, session auth.Session, org string) (Options, error) {
	result := Options{Reason: "Agent runtime unavailable; runner update required"}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner || session.AutomationID() != "" {
			return auth.ErrForbidden
		}
		if !s.enabled {
			return nil
		}
		_, err := nativeRunner(ctx, tx, org)
		if errors.Is(err, pgx.ErrNoRows) {
			result.Reason = "Waiting for built-in runner"
			return nil
		}
		result.Available, result.Reason = err == nil, ""
		return err
	})
	return result, err
}

func (s *Service) Create(ctx context.Context, session auth.Session, org, provider, name string) (connections.Connection, error) {
	if !s.enabled || (provider != "codex" && provider != "claude_code") || session.AutomationID() != "" {
		return connections.Connection{}, agent.ErrDisabled
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = map[string]string{"codex": "Codex", "claude_code": "Claude Code"}[provider]
	}
	if len(name) > 160 {
		return connections.Connection{}, auth.ErrInvalid
	}
	c := connections.Connection{OrgID: org, ID: domain.NewID(), Kind: "agent", Provider: provider, Name: name, State: "disabled", Reason: "Sign in to your subscription", Version: 1, CredentialVersion: 1, Capabilities: map[string]domain.Capability{}}
	c.Endpoint = map[string]string{"codex": "https://chatgpt.com", "claude_code": "https://claude.ai"}[provider]
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner || !a.AllRepositories {
			return auth.ErrForbidden
		}
		runnerID, err := nativeRunner(ctx, tx, org)
		if err != nil {
			return agent.ErrDisabled
		}
		c.Settings = connections.Settings{AuthKind: "official_runtime", BillingRoute: "subscription", Managed: "native_runtime", RuntimeUserID: a.UserID, RuntimeRunnerID: runnerID}
		raw, _ := json.Marshal(c.Settings)
		if _, err = tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, org, c.ID, c.Kind, c.Provider, c.Name, c.Endpoint, raw, c.State, c.Reason); err != nil {
			return err
		}
		return audit(ctx, tx, a, "agent.created", c.ID, domain.NewID())
	})
	return c, err
}

func (s *Service) connection(ctx context.Context, session auth.Session, org, id string) (connections.Connection, error) {
	var c connections.Connection
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if session.AutomationID() != "" || (a.Role != domain.Owner && a.Role != domain.Admin) {
			return auth.ErrForbidden
		}
		var err error
		c, err = s.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !c.NativeAgent() || c.Settings.RuntimeUserID != a.UserID || c.State == "revoked" {
			return auth.ErrForbidden
		}
		return nil
	})
	return c, err
}

func Scope(c connections.Connection) agent.NativeScope {
	return agent.NativeScope{OrgID: c.OrgID, UserID: c.Settings.RuntimeUserID, ConnectionID: c.ID, RunnerID: c.Settings.RuntimeRunnerID, Provider: c.Provider, AccountID: c.Settings.Namespace}
}

func audit(ctx context.Context, tx pgx.Tx, actor domain.Actor, action, id, operation string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,'{}'::jsonb)`, domain.NewID(), actor.OrgID, actor.UserID, action, id, operation)
	return err
}

func (s *Service) Action(ctx context.Context, session auth.Session, org, id string, action agent.NativeAction) (agent.NativeState, error) {
	if !s.enabled || !action.Valid() {
		return agent.NativeState{}, agent.ErrDisabled
	}
	c, err := s.connection(ctx, session, org, id)
	if err != nil {
		return agent.NativeState{}, err
	}
	operation := domain.NewID()
	if action.Kind == "login" || action.Kind == "logout" {
		err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			if a.UserID != c.Settings.RuntimeUserID || (a.Role != domain.Owner && a.Role != domain.Admin) {
				return auth.ErrForbidden
			}
			var running bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_tasks t JOIN workflow_jobs j ON j.org_id=t.org_id AND j.task_id=t.id WHERE t.org_id=$1 AND t.model_connection_id=$2 AND j.state IN ('running','reconciling')) OR EXISTS(SELECT 1 FROM budget_reservations WHERE org_id=$1 AND record->>'connection_id'=$2::text AND state IN ('reserved','dispatched','unknown'))`, org, id).Scan(&running); err != nil {
				return err
			}
			if running {
				return connections.ErrInUse
			}
			settings := c.Settings
			settings.RuntimeDisconnected = false
			if action.Kind == "login" {
				settings.Namespace = ""
			}
			raw, _ := json.Marshal(settings)
			changed, err := tx.Exec(ctx, `UPDATE connections SET state='disabled',reason=$4,settings=$5,version=version+1 WHERE org_id=$1 AND id=$2 AND version=$3`, org, id, c.Version, "Sign in to your subscription", raw)
			if err != nil {
				return err
			}
			if changed.RowsAffected() != 1 {
				return auth.ErrConflict
			}
			return audit(ctx, tx, a, "agent."+action.Kind, id, operation)
		})
		if err != nil {
			return agent.NativeState{}, err
		}
		c, err = s.connection(ctx, session, org, id)
		if err != nil {
			return agent.NativeState{}, err
		}
	}
	if action.Kind == "verify" || action.Kind == "code" {
		if err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error { return audit(ctx, tx, a, "agent."+action.Kind, id, operation) }); err != nil {
			return agent.NativeState{}, err
		}
	}
	op := privateconnector.Operation{ID: operation, Kind: privateconnector.AgentAction, Agent: &action}
	var state agent.NativeState
	_, err = s.private.Dispatch(ctx, privateconnector.Target{OrgID: org, RunnerID: c.Settings.RuntimeRunnerID}, op, func(ctx context.Context, ready privateconnector.Ready, deliver privateconnector.Deliver) error {
		check := func(ctx context.Context) error {
			return s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
				current, err := s.connections.MetadataTx(ctx, tx, org, id)
				if err != nil {
					return err
				}
				if current.Version != c.Version || current.CredentialVersion != c.CredentialVersion || current.State == "revoked" || current.Settings.RuntimeUserID != a.UserID || (a.Role != domain.Owner && a.Role != domain.Admin) || ready.ID != c.Settings.RuntimeRunnerID || ready.OrgID != org {
					return auth.ErrForbidden
				}
				return s.runners.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash)
			})
		}
		if err := check(ctx); err != nil {
			return err
		}
		scope := Scope(c)
		received, err := deliver(privateconnector.GrantSpec{Check: check, OperationID: operation, AuthorityID: operation, RunnerVersion: ready.Version, CredentialHash: ready.CredentialHash, Connection: privateconnector.Connection{Native: &scope, OrgID: org, ID: id, Kind: "agent", Provider: c.Provider, AuthKind: "official_runtime", Model: c.Settings.Model, Version: c.Version, CredentialVersion: c.CredentialVersion}})
		if err != nil {
			return err
		}
		if received.Native == nil {
			return agent.ErrProtocol
		}
		state = *received.Native
		return nil
	})
	if err != nil {
		return agent.NativeState{}, err
	}
	if action.Kind == "logout" && state.State == "disconnected" {
		if err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			changed, err := tx.Exec(ctx, `UPDATE connections SET settings=jsonb_set(settings-'namespace','{runtime_disconnected}','true'::jsonb),version=version+1 WHERE org_id=$1 AND id=$2 AND version=$3`, org, id, c.Version)
			if err == nil && changed.RowsAffected() != 1 {
				return auth.ErrConflict
			}
			return err
		}); err != nil {
			return agent.NativeState{}, err
		}
	}
	if state.State == "disconnected" && c.State == "healthy" {
		if err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			if a.UserID != c.Settings.RuntimeUserID || (a.Role != domain.Owner && a.Role != domain.Admin) {
				return auth.ErrForbidden
			}
			_, err := tx.Exec(ctx, `UPDATE connections SET state='degraded',reason='Sign in to your subscription again',version=version+1 WHERE org_id=$1 AND id=$2 AND version=$3`, org, id, c.Version)
			return err
		}); err != nil {
			return agent.NativeState{}, err
		}
	}
	if state.Account != nil {
		if state.Account.ID == "" || len(state.Account.ID) > 512 || len(state.Account.Email) > 320 || len(state.Account.Plan) > 100 {
			return agent.NativeState{}, agent.ErrProtocol
		}
		if c.Settings.Namespace != "" && c.Settings.Namespace != state.Account.ID {
			return agent.NativeState{}, agent.ErrDenied
		}
		if state.State == "ready" && action.Kind != "verify" || state.State == "ready" && state.Model != action.Model {
			return agent.NativeState{}, agent.ErrProtocol
		}
		if err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			if a.UserID != c.Settings.RuntimeUserID || (a.Role != domain.Owner && a.Role != domain.Admin) {
				return auth.ErrForbidden
			}
			if state.State == "ready" {
				c.Settings.Model, c.Settings.RuntimeVersion = state.Model, state.Version
				c.Settings.Namespace = state.Account.ID
				if err := budget.ConfigureNativeTx(ctx, tx, org, c.ID, state.Model, operation); err != nil {
					return err
				}
			} else if c.Settings.Namespace == "" {
				c.Settings.Namespace = state.Account.ID
			} else {
				return nil
			}
			raw, _ := json.Marshal(c.Settings)
			status, reason := "disabled", "Choose a model and verify your subscription"
			if state.State == "ready" {
				status, reason = "healthy", "Subscription verified; ready for Autopilot"
			}
			changed, err := tx.Exec(ctx, `UPDATE connections SET settings=$4,state=$5,reason=$6,server_version=$7,verified_at=CASE WHEN $5='healthy' THEN clock_timestamp() ELSE verified_at END,version=version+1 WHERE org_id=$1 AND id=$2 AND version=$3`, org, id, c.Version, raw, status, reason, state.Version)
			if err == nil && changed.RowsAffected() != 1 {
				return auth.ErrConflict
			}
			return err
		}); err != nil {
			return agent.NativeState{}, err
		}
		if c.State == "healthy" && state.State == "connected" {
			state.State, state.Model = "ready", c.Settings.Model
		}
	}
	return state, nil
}
