package gitops

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/providers"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Service struct {
	gateMu        sync.RWMutex
	gateAuthority func(context.Context, pgx.Tx, string, string) error
	db            *store.Store
	auth          *auth.Service
	connections   *connections.Service
	policies      *policy.Service
	providers     providers.Client
	merges        *mergecontrol.Service
}

func New(db *store.Store, identity *auth.Service, connections *connections.Service, policies *policy.Service, providers providers.Client, merges *mergecontrol.Service) *Service {
	s := &Service{db: db, auth: identity, connections: connections, policies: policies, providers: providers, merges: merges}
	if merges != nil {
		merges.RegisterChangeAuthority(s.CheckMergeTx)
	}
	return s
}
func manage(a domain.Actor, repos ...string) bool {
	if a.Role != domain.Owner && a.Role != domain.Admin && a.Role != domain.Maintainer {
		return false
	}
	for _, r := range repos {
		if !auth.CanReadRepository(a, r) {
			return false
		}
	}
	return true
}
func lock(ctx context.Context, tx pgx.Tx, org string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id)
}
func repository(ctx context.Context, tx pgx.Tx, org, repo string) (forge.RepoRef, string, error) {
	var r forge.RepoRef
	var c string
	err := tx.QueryRow(ctx, `SELECT native_id,name,connection_id::text FROM repositories WHERE org_id=$1 AND id=$2 AND accessible AND NOT archived`, org, repo).Scan(&r.NativeID, &r.FullName, &c)
	if errors.Is(err, pgx.ErrNoRows) {
		err = auth.ErrForbidden
	}
	return r, c, err
}
func configTx(ctx context.Context, tx pgx.Tx, org, env string) (Configuration, error) {
	var out Configuration
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM gitops_configurations WHERE org_id=$1 AND environment=$2`, org, env).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}
func gateTx(ctx context.Context, tx pgx.Tx, org, id string) (Gate, error) {
	var out Gate
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM gitops_gates WHERE org_id=$1 AND id=$2`, org, id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}

var imageRepository = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]{1,5})?/[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)

func validKey(key string) bool {
	b, err := base64.StdEncoding.DecodeString(key)
	return err == nil && len(b) == ed25519.PublicKeySize
}
func validPath(value string) bool {
	return value != "" && len(value) <= 1024 && path.Clean(value) == value && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && value != "." && !strings.ContainsAny(value, "\x00\r\n\\")
}
func validBranch(value string) bool {
	return value != "" && len(value) <= 255 && !strings.HasPrefix(value, "-") && !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/") && !strings.HasSuffix(value, ".") && !strings.HasSuffix(value, ".lock") && !strings.Contains(value, "..") && !strings.Contains(value, "//") && !strings.Contains(value, "@{") && !strings.ContainsAny(value, " ~^:?*[\\\x00\r\n\t")
}
func (s *Service) Configurations(ctx context.Context, session auth.Session, org string) ([]Configuration, error) {
	out := []Configuration{}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT document FROM gitops_configurations WHERE org_id=$1 ORDER BY environment LIMIT 1000`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var c Configuration
			if err = rows.Scan(&raw); err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &c); err != nil {
				return err
			}
			if auth.CanReadRepository(a, c.SourceRepositoryID) && auth.CanReadRepository(a, c.DeliveryRepositoryID) {
				out = append(out, c)
			}
		}
		return rows.Err()
	})
	return out, err
}
func (s *Service) PutConfiguration(ctx context.Context, session auth.Session, org string, c Configuration, expected int64, request string) (Configuration, error) {
	if !auth.ValidID(c.SourceRepositoryID) || !auth.ValidID(c.DeliveryRepositoryID) || !forge.ValidEnvironment(c.Environment) || expected < 0 || !validBranch(c.TargetBranch) || !validPath(c.ManifestPath) || len(c.Pointer) > 1024 || len(c.ImageRepository) > 512 || !imageRepository.MatchString(c.ImageRepository) || !validKey(c.ProvenancePublicKey) || !validKey(c.HealthPublicKey) || len(c.HealthChecks) > 20 || c.ObservationSeconds < 0 || c.ObservationSeconds > 86400 || c.MaxEvidenceAgeSeconds < 10 || c.MaxEvidenceAgeSeconds > 3600 || c.DeadlineSeconds < 60 || c.DeadlineSeconds > 86400 || c.ObservationSeconds >= c.DeadlineSeconds {
		return c, auth.ErrInvalid
	}
	if _, err := pointerSegments(c.Pointer); err != nil {
		return c, auth.ErrInvalid
	}
	if c.PromoteFrom != "" && (!forge.ValidEnvironment(c.PromoteFrom) || c.PromoteFrom == c.Environment) {
		return c, auth.ErrInvalid
	}
	seen := map[string]bool{}
	for _, check := range c.HealthChecks {
		if check == "" || len(check) > 128 || strings.TrimSpace(check) != check || seen[check] {
			return c, auth.ErrInvalid
		}
		seen[check] = true
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if (a.Role != domain.Owner && a.Role != domain.Admin) || !manage(a, c.SourceRepositoryID, c.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		old, err := configTx(ctx, tx, org, c.Environment)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if old.Version != expected {
			return auth.ErrConflict
		}
		if old.Version > 0 && !manage(a, old.SourceRepositoryID, old.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		for _, repo := range []string{c.SourceRepositoryID, c.DeliveryRepositoryID} {
			if _, _, err = repository(ctx, tx, org, repo); err != nil {
				return err
			}
		}
		c.Version = expected + 1
		raw, _ := json.Marshal(c)
		_, err = tx.Exec(ctx, `INSERT INTO gitops_configurations(org_id,environment,source_repository_id,delivery_repository_id,version,document) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(org_id,environment) DO UPDATE SET source_repository_id=excluded.source_repository_id,delivery_repository_id=excluded.delivery_repository_id,version=excluded.version,document=excluded.document,updated_at=now()`, org, c.Environment, c.SourceRepositoryID, c.DeliveryRepositoryID, c.Version, raw)
		if err != nil {
			return err
		}
		return emit(ctx, tx, org, c.DeliveryRepositoryID, a.UserID, "gitops.configuration", c.Environment, c.Version, request, map[string]any{"enabled": c.Enabled, "source_repository_id": c.SourceRepositoryID})
	})
	return c, err
}
func proofValid(c Configuration, org string, r PreviewRequest, now time.Time) bool {
	return deployment.ValidProvenance(deployment.Configuration{RepositoryID: c.SourceRepositoryID, ProvenancePublicKey: c.ProvenancePublicKey}, org, deployment.PreviewRequest{SourceSHA: r.SourceSHA, ArtifactDigest: r.ArtifactDigest, Provenance: r.Provenance}, now)
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
	return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: repo, Type: action, AggregateType: "gitops", AggregateID: aggregate, AggregateVersion: version, RequestID: request, DataVersion: 1, Data: raw})
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
