package deployment

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/providers"
	"github.com/reforgeapp/reforge/pkg/source"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

type Service struct {
	gateMu        sync.RWMutex
	gateAuthority func(context.Context, pgx.Tx, string, string) error
	db            *store.Store
	auth          *auth.Service
	connections   *connections.Service
	policies      *policy.Service
	providers     providers.Client
}

func New(db *store.Store, identity *auth.Service, connections *connections.Service, policies *policy.Service, providers providers.Client) *Service {
	return &Service{db: db, auth: identity, connections: connections, policies: policies, providers: providers}
}
func manage(a domain.Actor, repo string) bool {
	return auth.CanReadRepository(a, repo) && (a.Role == domain.Owner || a.Role == domain.Admin || a.Role == domain.Maintainer)
}
func lock(ctx context.Context, tx pgx.Tx, org string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id)
}
func repository(ctx context.Context, tx pgx.Tx, org, repo string) (forge.RepoRef, string, error) {
	var ref forge.RepoRef
	var connection string
	err := tx.QueryRow(ctx, `SELECT native_id,name,connection_id::text FROM repositories WHERE org_id=$1 AND id=$2 AND accessible AND NOT archived`, org, repo).Scan(&ref.NativeID, &ref.FullName, &connection)
	if errors.Is(err, pgx.ErrNoRows) {
		err = auth.ErrForbidden
	}
	return ref, connection, err
}
func configTx(ctx context.Context, tx pgx.Tx, org, env string) (Configuration, error) {
	var out Configuration
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM deployment_configurations WHERE org_id=$1 AND environment=$2`, org, env).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	if err == nil && out.NativeEnvironment == "" {
		out.NativeEnvironment = out.Environment
	}
	return out, err
}
func gateTx(ctx context.Context, tx pgx.Tx, org, id string) (Gate, error) {
	var out Gate
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM deployment_gates WHERE org_id=$1 AND id=$2`, org, id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}
func publicKey(value string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, auth.ErrInvalid
	}
	return ed25519.PublicKey(raw), nil
}
func VerifySignature(key string, document any, signature string) bool {
	pub, err := publicKey(key)
	if err != nil {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	raw, err := json.Marshal(document)
	return err == nil && ed25519.Verify(pub, raw, sig)
}
func validWorkflow(w Workflow) bool {
	in := forge.PipelineRequest{Repository: forge.RepoRef{NativeID: "1"}, WorkflowID: w.ID, WorkflowPath: w.Path, WorkflowSHA: w.SHA, ConfigSHA256: w.ConfigSHA256, Ref: w.Ref, SourceSHA: strings.Repeat("a", 40), ArtifactDigest: "sha256:" + strings.Repeat("a", 64), Environment: "check", CorrelationID: domain.NewID(), Inputs: w.Inputs, RulesHash: strings.Repeat("a", 64)}
	return forge.ValidPipelineRequest(in)
}
func ValidProvenance(cfg Configuration, org string, in PreviewRequest, now time.Time) bool {
	p := in.Provenance.Document
	return p.OrgID == org && p.RepositoryID == cfg.RepositoryID && p.SourceSHA == in.SourceSHA && p.ArtifactDigest == in.ArtifactDigest && p.BuildID != "" && len(p.BuildID) <= 512 && !p.IssuedAt.IsZero() && !p.IssuedAt.After(now) && p.ExpiresAt.After(now) && p.ExpiresAt.Sub(p.IssuedAt) <= 30*24*time.Hour && VerifySignature(cfg.ProvenancePublicKey, p, in.Provenance.Signature)
}
func qualified(cfg Configuration, c connections.Connection, now time.Time) bool {
	q := cfg.Qualification
	return cfg.Enabled && cfg.Mode == "pipeline" && c.State == "healthy" && (c.Provider == "github" || c.Provider == "gitlab") && q.Provider == c.Provider && q.ServerVersion == c.ServerVersion && q.ConnectionVersion == c.Version && q.ExpiresAt.After(now) && !q.VerifiedAt.After(now) && q.PinnedInputs && q.NativeEnforcement && q.EnvironmentSerialization && q.NoBypass
}

func (s *Service) Configurations(ctx context.Context, session auth.Session, org string) ([]Configuration, error) {
	out := []Configuration{}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT repository_id::text,document FROM deployment_configurations WHERE org_id=$1 ORDER BY environment LIMIT 1000`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var repo string
			var raw []byte
			if err = rows.Scan(&repo, &raw); err != nil {
				return err
			}
			if !auth.CanReadRepository(a, repo) {
				continue
			}
			var c Configuration
			if err = json.Unmarshal(raw, &c); err != nil {
				return err
			}
			if c.NativeEnvironment == "" {
				c.NativeEnvironment = c.Environment
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) PutConfiguration(ctx context.Context, session auth.Session, org string, in Configuration, expected int64, request string) (Configuration, error) {
	if in.Workflow.Inputs == nil {
		in.Workflow.Inputs = map[string]string{}
	}
	if in.RecoveryWorkflow != nil && in.RecoveryWorkflow.Inputs == nil {
		in.RecoveryWorkflow.Inputs = map[string]string{}
	}
	if in.HealthChecks == nil {
		in.HealthChecks = []string{}
	}
	if in.NativeEnvironment == "" {
		in.NativeEnvironment = in.Environment
	}
	if !forge.ValidEnvironment(in.NativeEnvironment) {
		return Configuration{}, auth.ErrInvalid
	}
	if !auth.ValidID(in.RepositoryID) || !forge.ValidEnvironment(in.Environment) || expected < 0 || (in.Mode != "pipeline" && in.Mode != "observe") || !validWorkflow(in.Workflow) || in.RecoveryWorkflow != nil && !validWorkflow(*in.RecoveryWorkflow) || len(in.HealthChecks) > 20 || in.ObservationSeconds < 0 || in.ObservationSeconds > 86400 || in.MaxEvidenceAgeSeconds < 10 || in.MaxEvidenceAgeSeconds > 3600 || in.DeadlineSeconds < 60 || in.DeadlineSeconds > 86400 || in.ObservationSeconds >= in.DeadlineSeconds {
		return Configuration{}, auth.ErrInvalid
	}
	if _, err := publicKey(in.ProvenancePublicKey); err != nil {
		return Configuration{}, err
	}
	if _, err := publicKey(in.HealthPublicKey); err != nil {
		return Configuration{}, err
	}
	seen := map[string]bool{}
	for _, check := range in.HealthChecks {
		if strings.TrimSpace(check) != check || check == "" || len(check) > 128 || seen[check] {
			return Configuration{}, auth.ErrInvalid
		}
		seen[check] = true
	}
	q := in.Qualification
	if in.Enabled && in.Mode == "pipeline" && (!source.ValidSHA(q.EvidenceSHA256, "sha256") || q.EvidenceReference == "" || len(q.EvidenceReference) > 2048 || q.VerifiedAt.IsZero() || q.ExpiresAt.Sub(q.VerifiedAt) > 30*24*time.Hour) {
		return Configuration{}, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if (a.Role != domain.Owner && a.Role != domain.Admin) || !auth.CanReadRepository(a, in.RepositoryID) {
			return auth.ErrForbidden
		}
		old, err := configTx(ctx, tx, org, in.Environment)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return err
		}
		if old.Version != expected {
			return auth.ErrConflict
		}
		if old.RepositoryID != "" && !auth.CanReadRepository(a, old.RepositoryID) {
			return auth.ErrForbidden
		}
		_, id, err := repository(ctx, tx, org, in.RepositoryID)
		if err != nil {
			return err
		}
		c, err := s.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if in.Enabled && in.Mode == "pipeline" && !qualified(in, c, time.Now()) {
			return &domain.ProviderError{Kind: "unsupported", Message: "A current native workflow, pinned inputs, environment serialization and non-bypass qualification are required"}
		}
		in.Version = expected + 1
		raw, _ := json.Marshal(in)
		if _, err = tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,environment,repository_id,version,document) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,environment) DO UPDATE SET repository_id=excluded.repository_id,version=excluded.version,document=excluded.document,updated_at=now()`, org, in.Environment, in.RepositoryID, in.Version, raw); err != nil {
			return err
		}
		return emit(ctx, tx, org, in.RepositoryID, a.UserID, "deployment.configuration", in.Environment, in.Version, request, map[string]any{"version": in.Version, "enabled": in.Enabled})
	})
	return in, err
}

func emit(ctx context.Context, tx pgx.Tx, org, repo, actor, action, id string, version int64, request string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data,repository_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, domain.NewID(), org, actor, action, id, request, raw, repo)
	if err != nil {
		return err
	}
	aggregate := id
	if !auth.ValidID(aggregate) {
		aggregate = repo
	}
	return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: repo, Type: action, AggregateType: "deployment", AggregateID: aggregate, AggregateVersion: version, RequestID: request, DataVersion: 1, Data: raw})
}
func permitted(resolved policy.Resolved, cfg Configuration, w Workflow, action policy.Action, binding policy.Binding, now time.Time) policy.Result {
	ids := []string{"execution_authority", "native_enforcement", "artifact_provenance", "workflow_authority"}
	if action == policy.Recover {
		ids = []string{"execution_authority", "native_enforcement", "artifact_provenance", "recovery_authority", "known_good_artifact"}
	}
	evidence := []policy.Evidence{}
	for _, id := range ids {
		evidence = append(evidence, policy.Evidence{ID: id, State: "satisfied", Binding: binding, ObservedAt: now, Reference: cfg.Qualification.EvidenceReference + "#" + id})
	}
	input := policy.Input{Stage: "deployment_admission", Action: action, Environment: cfg.Environment, Workflow: w.ID, Current: binding, Now: now, Evidence: evidence}
	result := policy.Evaluate(resolved, input)
	if !slices.Contains(resolved.Policy.Allow.Environments, cfg.Environment) || !slices.Contains(resolved.Policy.Allow.Workflows, w.ID) {
		result.Outcome = "deny"
		result.Blockers = append(result.Blockers, "Environment and workflow require explicit policy allowlists")
	}
	return result
}

func (s *Service) Workflows(ctx context.Context, session auth.Session, org, repo string) ([]forge.Workflow, error) {
	if !auth.ValidID(repo) {
		return nil, auth.ErrInvalid
	}
	var ref forge.RepoRef
	var connection string
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		var err error
		ref, connection, err = repository(ctx, tx, org, repo)
		return err
	})
	if err != nil {
		return nil, err
	}
	result, err := s.providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeDeliveryWorkflows, Delivery: &privateconnector.DeliveryArgs{Repository: ref}}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		current, id, err := repository(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if id != c.ID || c.ID != connection || current != ref {
			return auth.ErrConflict
		}
		return nil
	})
	return result.Workflows, err
}

func (s *Service) RegisterGateAuthority(check func(context.Context, pgx.Tx, string, string) error) {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	s.gateAuthority = check
}
func (s *Service) checkGateAuthority(ctx context.Context, tx pgx.Tx, org, gate string) error {
	s.gateMu.RLock()
	check := s.gateAuthority
	s.gateMu.RUnlock()
	if check != nil {
		return check(ctx, tx, org, gate)
	}
	return nil
}
