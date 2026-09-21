package connections

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/network"
	"reforge/internal/secrets"
	"reforge/internal/store"
)

var ErrRunnerRequired = errors.New("enrol a runner in this organisation before approving a private route")
var ErrRevoked = errors.New("connection revoked")

type RunnerCheck func(context.Context, pgx.Tx, string, string) error
type Service struct {
	db           *store.Store
	auth         *auth.Service
	vault        *secrets.Vault
	development  bool
	mu           sync.RWMutex
	probers      map[string]Prober
	runnerCheck  RunnerCheck
	privateProbe PrivateProber
}

func New(db *store.Store, identity *auth.Service, vault *secrets.Vault, development bool) *Service {
	return &Service{db: db, auth: identity, vault: vault, development: development, probers: map[string]Prober{}}
}
func (s *Service) Register(kind, provider string, probe Prober) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probers[kind+":"+provider] = probe
}
func (s *Service) RegisterPrivateProbe(probe PrivateProber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.privateProbe = probe
}
func (s *Service) RegisterRunnerCheck(check RunnerCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runnerCheck = check
}
func (s *Service) checkRunner(ctx context.Context, tx pgx.Tx, orgID, runnerID string) error {
	s.mu.RLock()
	check := s.runnerCheck
	s.mu.RUnlock()
	if check == nil {
		return ErrRunnerRequired
	}
	return check(ctx, tx, orgID, runnerID)
}

func validSetup(r CreateRequest) bool {
	if r.Settings.CAPEM != "" && !x509.NewCertPool().AppendCertsFromPEM([]byte(r.Settings.CAPEM)) {
		return false
	}
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 160 || len(r.Endpoint) > 2048 || len(r.Secret) > 65536 || len(r.Settings.CAPEM) > 65536 {
		return false
	}
	if r.Settings.BillingRoute == "" {
		return false
	}
	switch r.Kind {
	case "forge", "delivery":
		if r.Provider != "github" && r.Provider != "gitlab" && r.Provider != "gitea" {
			return false
		}
		if r.Settings.BillingRoute != "forge" {
			return false
		}
		if r.Settings.AuthKind == "github_app" {
			return r.Provider == "github" && r.Secret != "" && r.Settings.AppID != "" && r.Settings.InstallationID != ""
		}
		return r.Settings.AuthKind == "token" && r.Secret != ""
	case "model":
		if r.Provider != "openai" && r.Provider != "anthropic" && r.Provider != "google" && r.Provider != "compatible" {
			return false
		}
		if r.Settings.BillingRoute != "direct_api" || r.Settings.Model == "" || r.Settings.AuthKind != "api_key" {
			return false
		}
		if strings.HasPrefix(r.Secret, "sk-ant-oat") || strings.HasPrefix(r.Secret, "eyJ") {
			return false
		}
		return r.Secret != "" || r.Provider == "compatible"
	case "agent":
		if r.Provider == "custom_command" {
			return r.Settings.AuthKind == "official_runtime" && r.Settings.BillingRoute == "subscription" && r.Secret == "" && r.Settings.Model != ""
		}
		return (r.Provider == "codex" || r.Provider == "claude_code" || r.Provider == "agy" || r.Provider == "gemini_cli") && r.Settings.AuthKind == "official_runtime" && r.Settings.BillingRoute == "subscription" && r.Secret == ""
	default:
		return false
	}
}

func (s *Service) options(c Connection, runnerID string) network.Options {
	o := network.Options{RunnerID: runnerID, CAPEM: []byte(c.Settings.CAPEM), Development: s.development}
	if c.Route != nil && c.Route.RevokedAt == nil {
		o.PrivateRoute = &network.PrivateRoute{OrgID: c.OrgID, ConnectionID: c.ID, RunnerID: c.Route.RunnerID, Host: c.Route.Host, CIDRs: c.Route.CIDRs}
	}
	return o
}
func (s *Service) Create(ctx context.Context, session auth.Session, orgID string, r CreateRequest, requestID string) (Connection, error) {
	c := Connection{ID: domain.NewID(), OrgID: orgID, Kind: r.Kind, Provider: r.Provider, Name: strings.TrimSpace(r.Name), Endpoint: r.Endpoint, Settings: r.Settings, State: "unverified", Reason: "Run a capability test before use", Capabilities: map[string]domain.Capability{}, CredentialVersion: 1, Version: 1, Route: r.PrivateRoute}
	if !validSetup(r) {
		return Connection{}, auth.ErrInvalid
	}
	runnerID := ""
	if c.Route != nil {
		c.Route = &Route{RunnerID: r.PrivateRoute.RunnerID, Host: r.PrivateRoute.Host, CIDRs: append([]string(nil), r.PrivateRoute.CIDRs...)}
		runnerID = c.Route.RunnerID
		if !auth.ValidID(runnerID) {
			return Connection{}, auth.ErrInvalid
		}
	}
	if _, err := network.ValidateEndpoint(c.Endpoint, s.options(c, runnerID)); err != nil {
		return Connection{}, auth.ErrInvalid
	}
	if c.Kind == "agent" {
		if c.Provider == "custom_command" {
			c.State = "healthy"
			c.Reason = "Approved custom command profiles are gated per run; exit 0 is not a validated repair"
		} else {
			c.State = "disabled"
			c.Reason = "Connect a documented official runtime and verify account entitlement, topology and budget controls"
		}
	}
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if c.Route != nil {
			if err := s.checkRunner(ctx, tx, orgID, runnerID); err != nil {
				return err
			}
		}
		settings, _ := json.Marshal(c.Settings)
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, orgID, c.ID, c.Kind, c.Provider, c.Name, c.Endpoint, settings, c.State, c.Reason); err != nil {
			return err
		}
		if r.Secret != "" {
			if err := s.putSecret(ctx, tx, &c, r.Secret); err != nil {
				return err
			}
		}
		if c.Route != nil {
			if err := s.putRoute(ctx, tx, c, a.UserID); err != nil {
				return err
			}
		}
		return record(ctx, tx, a, "connection.created", c.ID, requestID, c.Version)
	})
	return c, err
}
func (s *Service) putSecret(ctx context.Context, tx pgx.Tx, c *Connection, value string) error {
	e, err := s.vault.SealContext(ctx, secrets.Binding{OrgID: c.OrgID, ConnectionID: c.ID, Version: c.CredentialVersion}, []byte(value))
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if c.SecretID == nil {
		id := domain.NewID()
		c.SecretID = &id
	}
	_, err = tx.Exec(ctx, `INSERT INTO connection_secrets(org_id,id,connection_id,version,envelope) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,connection_id) DO UPDATE SET version=EXCLUDED.version,envelope=EXCLUDED.envelope,rotated_at=now()`, c.OrgID, c.SecretID, c.ID, c.CredentialVersion, encoded)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE connections SET secret_id=$3 WHERE org_id=$1 AND id=$2`, c.OrgID, c.ID, c.SecretID)
	return err
}
func (s *Service) putRoute(ctx context.Context, tx pgx.Tx, c Connection, userID string) error {
	return tx.QueryRow(ctx, `INSERT INTO connection_routes(org_id,connection_id,runner_id,hostname,cidrs,approved_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(org_id,connection_id) DO UPDATE SET runner_id=EXCLUDED.runner_id,hostname=EXCLUDED.hostname,cidrs=EXCLUDED.cidrs,approved_by=EXCLUDED.approved_by,approved_at=now(),revoked_at=NULL RETURNING approved_at`, c.OrgID, c.ID, c.Route.RunnerID, c.Route.Host, c.Route.CIDRs, userID).Scan(&c.Route.ApprovedAt)
}

const columns = `c.id::text,c.org_id::text,c.kind,c.provider,c.name,c.endpoint,c.settings,c.state,c.reason,c.capabilities,c.server_version,c.verified_at,c.credential_version,c.version,c.secret_id::text,(SELECT jsonb_build_object('runner_id',r.runner_id,'host',r.hostname,'cidrs',r.cidrs,'approved_at',r.approved_at,'revoked_at',r.revoked_at) FROM connection_routes r WHERE r.org_id=c.org_id AND r.connection_id=c.id)`

func scan(row pgx.Row) (Connection, error) {
	var c Connection
	var settings, caps, route []byte
	err := row.Scan(&c.ID, &c.OrgID, &c.Kind, &c.Provider, &c.Name, &c.Endpoint, &settings, &c.State, &c.Reason, &caps, &c.ServerVersion, &c.VerifiedAt, &c.CredentialVersion, &c.Version, &c.SecretID, &route)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, auth.ErrForbidden
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(settings, &c.Settings); err != nil {
		return c, err
	}
	if err = json.Unmarshal(caps, &c.Capabilities); err != nil {
		return c, err
	}
	if len(route) > 0 {
		err = json.Unmarshal(route, &c.Route)
	}
	return c, err
}
func load(ctx context.Context, tx pgx.Tx, orgID, id string) (Connection, error) {
	return scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM connections c WHERE c.org_id=$1 AND c.id=$2`, orgID, id))
}
func (s *Service) Get(ctx context.Context, session auth.Session, orgID, id string) (Connection, error) {
	var c Connection
	if !auth.ValidID(id) {
		return c, auth.ErrForbidden
	}
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		var err error
		c, err = load(ctx, tx, orgID, id)
		return err
	})
	return c, err
}
func (s *Service) List(ctx context.Context, session auth.Session, orgID, kind string, limit int, cursor string) (domain.Page[Connection], error) {
	page := domain.Page[Connection]{Items: []Connection{}}
	if limit < 1 || limit > 200 || (cursor != "" && !auth.ValidID(cursor)) {
		return page, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM connections c WHERE c.org_id=$1 AND ($2='' OR c.kind=$2) AND c.id::text>$3 ORDER BY c.id LIMIT $4`, orgID, kind, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scan(rows)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, c)
		}
		return rows.Err()
	})
	page.Complete = len(page.Items) <= limit
	if !page.Complete {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[limit-1].ID
	}
	return page, err
}
func (s *Service) change(ctx context.Context, session auth.Session, orgID, id string, expected int64, requestID, action string, fn func(pgx.Tx, *Connection) error) (Connection, error) {
	var c Connection
	if !auth.ValidID(id) || expected < 1 {
		return c, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		var err error
		c, err = load(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if c.Version != expected {
			return auth.ErrConflict
		}
		if c.State == "revoked" {
			return ErrRevoked
		}
		if err = fn(tx, &c); err != nil {
			return err
		}
		c.Version++
		caps, _ := json.Marshal(c.Capabilities)
		if _, err = tx.Exec(ctx, `UPDATE connections SET state=$3,reason=$4,capabilities=$5,server_version=$6,verified_at=$7,credential_version=$8,version=$9,revoked_at=CASE WHEN $3='revoked' THEN now() ELSE NULL END WHERE org_id=$1 AND id=$2`, orgID, id, c.State, c.Reason, caps, c.ServerVersion, c.VerifiedAt, c.CredentialVersion, c.Version); err != nil {
			return err
		}
		return record(ctx, tx, a, action, id, requestID, c.Version)
	})
	return c, err
}
func invalidate(c *Connection) {
	c.State = "unverified"
	c.Reason = "Connection changed; run a new capability test"
	c.Capabilities = map[string]domain.Capability{}
	c.VerifiedAt = nil
	c.ServerVersion = ""
}
func (s *Service) Rotate(ctx context.Context, session auth.Session, orgID, id string, expected int64, value, requestID string) (Connection, error) {
	return s.change(ctx, session, orgID, id, expected, requestID, "connection.credential_rotated", func(tx pgx.Tx, c *Connection) error {
		if value == "" || !validSetup(CreateRequest{Kind: c.Kind, Provider: c.Provider, Name: c.Name, Endpoint: c.Endpoint, Settings: c.Settings, Secret: value}) || c.Kind == "agent" {
			return auth.ErrInvalid
		}
		c.CredentialVersion++
		invalidate(c)
		return s.putSecret(ctx, tx, c, value)
	})
}
func (s *Service) Revoke(ctx context.Context, session auth.Session, orgID, id string, expected int64, requestID string) (Connection, error) {
	return s.change(ctx, session, orgID, id, expected, requestID, "connection.revoked", func(tx pgx.Tx, c *Connection) error {
		invalidate(c)
		c.State = "revoked"
		c.Reason = "Connection revoked; create a new connection to restore access"
		c.SecretID = nil
		c.CredentialVersion++
		if _, err := tx.Exec(ctx, `UPDATE connections SET secret_id=NULL WHERE org_id=$1 AND id=$2`, orgID, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM connection_secrets WHERE org_id=$1 AND connection_id=$2`, orgID, id); err != nil {
			return err
		}
		if c.Route != nil {
			return tx.QueryRow(ctx, `UPDATE connection_routes SET revoked_at=now() WHERE org_id=$1 AND connection_id=$2 RETURNING revoked_at`, orgID, id).Scan(&c.Route.RevokedAt)
		}
		return nil
	})
}
func (s *Service) Rewrap(ctx context.Context, session auth.Session, orgID, id string, expected int64, requestID string) (Connection, error) {
	return s.change(ctx, session, orgID, id, expected, requestID, "connection.wrapping_key_rotated", func(tx pgx.Tx, c *Connection) error {
		if c.SecretID == nil {
			return auth.ErrInvalid
		}
		var raw []byte
		var envelope secrets.Envelope
		if err := tx.QueryRow(ctx, `SELECT envelope FROM connection_secrets WHERE org_id=$1 AND id=$2 AND connection_id=$3 AND version=$4`, orgID, c.SecretID, id, c.CredentialVersion).Scan(&raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		envelope, err := s.vault.RewrapContext(ctx, secrets.Binding{OrgID: orgID, ConnectionID: id, Version: c.CredentialVersion}, envelope)
		if err != nil {
			return err
		}
		raw, err = json.Marshal(envelope)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE connection_secrets SET envelope=$3,rotated_at=now() WHERE org_id=$1 AND id=$2`, orgID, c.SecretID, raw)
		return err
	})
}
func (s *Service) SetRoute(ctx context.Context, session auth.Session, orgID, id string, expected int64, route *Route, requestID string) (Connection, error) {
	return s.change(ctx, session, orgID, id, expected, requestID, "connection.route_changed", func(tx pgx.Tx, c *Connection) error {
		if route == nil {
			if _, err := tx.Exec(ctx, `DELETE FROM connection_routes WHERE org_id=$1 AND connection_id=$2`, orgID, id); err != nil {
				return err
			}
			c.Route = nil
		} else {
			if !auth.ValidID(route.RunnerID) {
				return auth.ErrInvalid
			}
			if err := s.checkRunner(ctx, tx, orgID, route.RunnerID); err != nil {
				return err
			}
			c.Route = &Route{RunnerID: route.RunnerID, Host: route.Host, CIDRs: append([]string(nil), route.CIDRs...)}
			if _, err := network.ValidateEndpoint(c.Endpoint, s.options(*c, route.RunnerID)); err != nil {
				return auth.ErrInvalid
			}
			if err := s.putRoute(ctx, tx, *c, session.User.ID); err != nil {
				return err
			}
		}
		invalidate(c)
		return nil
	})
}
func (s *Service) MetadataTx(ctx context.Context, tx pgx.Tx, orgID, id string) (Connection, error) {
	if !auth.ValidID(orgID) || !auth.ValidID(id) {
		return Connection{}, auth.ErrForbidden
	}
	return load(ctx, tx, orgID, id)
}

func (s *Service) ResolveTx(ctx context.Context, tx pgx.Tx, orgID, id, runnerID string) (Resolved, error) {
	var r Resolved
	if !auth.ValidID(orgID) || !auth.ValidID(id) {
		return r, auth.ErrForbidden
	}
	var err error
	r.Connection, err = load(ctx, tx, orgID, id)
	if err != nil {
		return r, err
	}
	if r.Connection.State == "revoked" {
		return r, ErrRevoked
	}
	if r.Connection.Route != nil {
		if r.Connection.Route.RevokedAt != nil || r.Connection.Route.RunnerID != runnerID {
			return r, ErrRunnerRequired
		}
		if err = s.checkRunner(ctx, tx, orgID, runnerID); err != nil {
			return r, err
		}
		if _, err = network.ValidateEndpoint(r.Connection.Endpoint, s.options(r.Connection, runnerID)); err != nil {
			return r, err
		}
	} else {
		r.Client, err = network.NewClient(r.Connection.Endpoint, s.options(r.Connection, ""))
		if err != nil {
			return r, err
		}
	}
	if r.Connection.SecretID != nil {
		var e secrets.Envelope
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT envelope FROM connection_secrets WHERE org_id=$1 AND id=$2 AND connection_id=$3 AND version=$4`, orgID, r.Connection.SecretID, id, r.Connection.CredentialVersion).Scan(&raw); err != nil {
			return r, err
		}
		if err = json.Unmarshal(raw, &e); err != nil {
			return r, err
		}
		plain, err := s.vault.OpenContext(ctx, secrets.Binding{OrgID: orgID, ConnectionID: id, Version: r.Connection.CredentialVersion}, e)
		if err != nil {
			return r, err
		}
		r.Secret = string(plain)
		clear(plain)
	}
	return r, nil
}
func (s *Service) Test(ctx context.Context, session auth.Session, orgID, id string, expected int64, requestID string) (Connection, error) {
	observed, err := s.Get(ctx, session, orgID, id)
	if err != nil {
		return Connection{}, err
	}
	if observed.State == "revoked" {
		return Connection{}, ErrRevoked
	}
	if observed.Version != expected {
		return Connection{}, auth.ErrConflict
	}
	s.mu.RLock()
	privateProbe := s.privateProbe
	s.mu.RUnlock()
	if observed.Route != nil && privateProbe != nil {
		var updated Connection
		err = privateProbe(ctx, observed, func(probeCtx context.Context, call ProbeCall) error {
			var updateErr error
			updated, updateErr = s.change(probeCtx, session, orgID, id, expected, requestID, "connection.tested", func(tx pgx.Tx, c *Connection) error {
				if c.Route == nil || c.Version != observed.Version || c.Route.RunnerID != observed.Route.RunnerID {
					return auth.ErrConflict
				}
				r, err := s.ResolveTx(probeCtx, tx, orgID, id, c.Route.RunnerID)
				if err != nil {
					return err
				}
				result, err := call(probeCtx, tx, r)
				result = redactProbe(result, r.Secret)
				r.Secret = ""
				if err != nil {
					return err
				}
				return applyProbe(c, result)
			})
			return updateErr
		})
		return updated, err
	}
	return s.change(ctx, session, orgID, id, expected, requestID, "connection.tested", func(tx pgx.Tx, c *Connection) error {
		s.mu.RLock()
		probe := s.probers[c.Kind+":"+c.Provider]
		s.mu.RUnlock()
		result := ProbeResult{State: "disabled", Reason: "Provider capability verification is unavailable; configure a supported adapter before use", Capabilities: map[string]domain.Capability{}}
		if c.Route != nil {
			result.Reason = "Run capability verification on the enrolled private runner"
		} else if probe != nil && c.Kind != "agent" {
			probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			r, err := s.ResolveTx(probeCtx, tx, orgID, id, "")
			if err != nil {
				return err
			}
			result, err = probe(probeCtx, r)
			if r.Client != nil {
				r.Client.CloseIdleConnections()
			}
			cancel()
			result = redactProbe(result, r.Secret)
			r.Secret = ""
			if err != nil {
				result = ProbeResult{State: "degraded", Reason: "Capability test failed; check endpoint, credentials and provider access, then retry", Capabilities: map[string]domain.Capability{}}
			}
		}
		return applyProbe(c, result)
	})
}
func applyProbe(c *Connection, result ProbeResult) error {
	if result.State != "healthy" && result.State != "disabled" && result.State != "degraded" {
		return auth.ErrInvalid
	}
	if result.Capabilities == nil {
		result.Capabilities = map[string]domain.Capability{}
	}
	now := time.Now().UTC()
	c.State, c.Reason, c.Capabilities, c.ServerVersion, c.VerifiedAt = result.State, result.Reason, result.Capabilities, result.ServerVersion, &now
	return nil
}
func redactProbe(result ProbeResult, secret string) ProbeResult {
	if secret == "" {
		return result
	}
	redact := func(value string) string { return strings.ReplaceAll(value, secret, "[redacted]") }
	result.Reason = redact(result.Reason)
	result.ServerVersion = redact(result.ServerVersion)
	clean := make(map[string]domain.Capability, len(result.Capabilities))
	for name, c := range result.Capabilities {
		c.Reason = redact(c.Reason)
		c.Source = redact(c.Source)
		c.Version = redact(c.Version)
		c.Scope = redact(c.Scope)
		clean[redact(name)] = c
	}
	result.Capabilities = clean
	return result
}
func record(ctx context.Context, tx pgx.Tx, a domain.Actor, action, id, requestID string, version int64) error {
	data, _ := json.Marshal(map[string]int64{"version": version})
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), a.OrgID, a.UserID, action, id, requestID, data)
	return err
}
