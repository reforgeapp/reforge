package mergecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/providers"
	"github.com/reforgeapp/reforge/internal/source"
	"github.com/reforgeapp/reforge/internal/store"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type Service struct {
	changeAuthority func(context.Context, pgx.Tx, string, string, forge.MergeEvidence, domain.Actor) (string, error)
	db              *store.Store
	auth            *auth.Service
	connections     *connections.Service
	policies        *policy.Service
	providers       providers.Client
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

func configTx(ctx context.Context, tx pgx.Tx, org, repo string) (Configuration, error) {
	out := Configuration{RepositoryID: repo, CheckPublishers: map[string]string{}}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM merge_configurations WHERE org_id=$1 AND repository_id=$2`, org, repo).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}

func (s *Service) Config(ctx context.Context, session auth.Session, org, repo string) (Configuration, error) {
	var out Configuration
	if !auth.ValidID(repo) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		if _, _, err := repository(ctx, tx, org, repo); err != nil {
			return err
		}
		var err error
		out, err = configTx(ctx, tx, org, repo)
		return err
	})
	return out, err
}

func (s *Service) PutConfig(ctx context.Context, session auth.Session, org, repo string, in Configuration, expected int64, request string) (Configuration, error) {
	if !auth.ValidID(repo) || expected < 0 || len(in.CheckPublishers) > 100 || len(in.CooperationReference) > 2048 || strings.TrimSpace(in.CooperationReference) != in.CooperationReference || in.InspectorConnectionID != "" && !auth.ValidID(in.InspectorConnectionID) {
		return Configuration{}, auth.ErrInvalid
	}
	for name, publisher := range in.CheckPublishers {
		if name == "" || len(name) > 256 || publisher == "" || len(publisher) > 128 {
			return Configuration{}, auth.ErrInvalid
		}
	}
	q := in.Qualification
	if in.Enabled && (in.CooperationReference == "" || !q.ExactHead || !q.StrictTarget && !q.QueueExecutionGate || !source.ValidSHA(q.EvidenceSHA256, "sha256") || q.EvidenceReference == "" || len(q.EvidenceReference) > 2048 || q.VerifiedAt.IsZero() || q.VerifiedAt.After(time.Now()) || !q.ExpiresAt.After(time.Now()) || q.ExpiresAt.Sub(q.VerifiedAt) > 30*24*time.Hour) {
		return Configuration{}, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) || a.Role != domain.Owner && a.Role != domain.Admin {
			return auth.ErrForbidden
		}
		_, id, err := repository(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		c, err := s.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		old, err := configTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if old.Version != expected {
			return auth.ErrConflict
		}
		if in.Enabled && (c.State != "healthy" || q.Provider != c.Provider || q.ServerVersion != c.ServerVersion || q.ConnectionVersion != c.Version) {
			return auth.ErrConflict
		}
		if in.Enabled && q.QueueExecutionGate {
			supported := c.Provider == "github" && c.Settings.GitHubApp() && c.Settings.AppID != "" && in.CheckPublishers[forge.QueueExecutionCheckName] == c.Settings.AppID
			supported = supported || c.Provider == "gitlab" && source.ValidSHA(q.CIConfigSHA256, "sha256") && in.CheckPublishers[forge.QueueExecutionCheckName] != ""
			if !supported {
				return &domain.ProviderError{Kind: "unsupported", Message: "Queue execution requires a qualified GitHub App or protected GitLab train job with a pinned CI configuration and operational publisher"}
			}
		}
		if in.InspectorConnectionID != "" {
			inspector, err := s.connections.MetadataTx(ctx, tx, org, in.InspectorConnectionID)
			if err != nil {
				return err
			}
			if inspector.ID == c.ID || inspector.Provider != "gitea" || c.Provider != "gitea" || inspector.Endpoint != c.Endpoint || inspector.Kind != "forge" {
				return auth.ErrInvalid
			}
			if in.Enabled && (inspector.State != "healthy" || inspector.Version != q.InspectorVersion) {
				return auth.ErrConflict
			}
		} else if in.Enabled && c.Provider == "gitea" {
			return auth.ErrInvalid
		}
		in.RepositoryID, in.Version = repo, expected+1
		if in.CheckPublishers == nil {
			in.CheckPublishers = map[string]string{}
		}
		raw, _ := json.Marshal(in)
		if _, err = tx.Exec(ctx, `INSERT INTO merge_configurations(org_id,repository_id,version,document) VALUES($1,$2,$3,$4) ON CONFLICT(org_id,repository_id) DO UPDATE SET version=excluded.version,document=excluded.document,updated_at=now()`, org, repo, in.Version, raw); err != nil {
			return err
		}
		return emit(ctx, tx, org, repo, a.UserID, "merge.configuration", repo, in.Version, request, map[string]any{"version": in.Version, "enabled": in.Enabled})
	})
	return in, err
}

func emit(ctx context.Context, tx pgx.Tx, org, repo, actor, action, id string, version int64, request string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data,repository_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, domain.NewID(), org, actor, action, id, request, raw, repo); err != nil {
		return err
	}
	return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: repo, Type: action, AggregateType: "change", AggregateID: id, AggregateVersion: version, RequestID: request, DataVersion: 1, Data: raw})
}

func (s *Service) RegisterChangeAuthority(check func(context.Context, pgx.Tx, string, string, forge.MergeEvidence, domain.Actor) (string, error)) {
	s.changeAuthority = check
}
