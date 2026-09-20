package policy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/store"
)

type Service struct {
	db         *store.Store
	auth       *auth.Service
	deployment Policy
}

func New(db *store.Store, identity *auth.Service, deployment Policy) (*Service, error) {
	if db == nil || identity == nil {
		return nil, auth.ErrInvalid
	}
	if err := Validate(deployment); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(deployment)
	var copy Policy
	_ = json.Unmarshal(b, &copy)
	return &Service{db: db, auth: identity, deployment: canonical(copy)}, nil
}

func (s *Service) ResolveTx(ctx context.Context, tx pgx.Tx, orgID, repoID string) (Resolved, error) {
	if !auth.ValidID(orgID) || (repoID != "" && !auth.ValidID(repoID)) {
		return Resolved{}, auth.ErrInvalid
	}
	layers := []Layer{{Scope: Scope{Kind: "deployment", ID: "deployment"}, VersionID: hash(s.deployment), BindingVersion: 1, Policy: s.deployment}}
	var paused bool
	if err := tx.QueryRow(ctx, `SELECT paused FROM organisations WHERE id=$1`, orgID).Scan(&paused); err != nil {
		return Resolved{}, hidden(err)
	}
	teamIDs := []string{}
	primary := ""
	if repoID != "" {
		var repoPaused, accessible, archived bool
		if err := tx.QueryRow(ctx, `SELECT paused,accessible,archived FROM repositories WHERE org_id=$1 AND id=$2`, orgID, repoID).Scan(&repoPaused, &accessible, &archived); err != nil {
			return Resolved{}, hidden(err)
		}
		paused = paused || repoPaused || !accessible || archived
		if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(team_id::text ORDER BY team_id),'{}') FROM team_repositories WHERE org_id=$1 AND repository_id=$2`, orgID, repoID).Scan(&teamIDs); err != nil {
			return Resolved{}, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT b.scope_kind,b.scope_id::text,b.policy_version_id::text,b.version,coalesce(b.primary_team_id::text,''),v.document FROM policy_bindings b JOIN policy_versions v ON v.org_id=b.org_id AND v.id=b.policy_version_id WHERE b.org_id=$1 AND ((b.scope_kind='organisation' AND b.scope_id=$1) OR (b.scope_kind='repository' AND b.scope_id::text=$2) OR (b.scope_kind='team' AND b.scope_id::text=ANY($3::text[]))) ORDER BY b.scope_kind,b.scope_id`, orgID, repoID, teamIDs)
	if err != nil {
		return Resolved{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var l Layer
		var b []byte
		var p string
		if err = rows.Scan(&l.Scope.Kind, &l.Scope.ID, &l.VersionID, &l.BindingVersion, &p, &b); err != nil {
			return Resolved{}, err
		}
		if err = json.Unmarshal(b, &l.Policy); err != nil {
			return Resolved{}, err
		}
		if l.Scope.Kind == "repository" {
			primary = p
		}
		layers = append(layers, l)
	}
	if err = rows.Err(); err != nil {
		return Resolved{}, err
	}
	r := Resolve(layers, repoID, primary, paused)
	if primary != "" && !contains(teamIDs, primary) {
		r.Problems = append(r.Problems, "primary team no longer bound to repository")
		r.Hash = ""
		r.Hash = hash(r)
	}
	return r, nil
}

func (s *Service) Resolve(ctx context.Context, session auth.Session, orgID, repoID string) (Resolved, error) {
	var r Resolved
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if repoID == "" {
			if a.Role != domain.Owner {
				return auth.ErrForbidden
			}
		} else if !auth.CanReadRepository(a, repoID) {
			return auth.ErrForbidden
		}
		var err error
		r, err = s.ResolveTx(ctx, tx, orgID, repoID)
		return err
	})
	return r, err
}

func (s *Service) EvaluateTx(ctx context.Context, tx pgx.Tx, orgID, repoID string, in Input) (Result, error) {
	r, err := s.ResolveTx(ctx, tx, orgID, repoID)
	if err != nil {
		return Result{}, err
	}
	in.Now = time.Now().UTC()
	return Evaluate(r, in), nil
}

func (s *Service) CreateVersion(ctx context.Context, session auth.Session, orgID string, scope Scope, p Policy, reason, requestID string) (Version, error) {
	reason = strings.TrimSpace(reason)
	if err := Validate(p); err != nil {
		return Version{}, auth.ErrInvalid
	}
	if len(reason) == 0 || len(reason) > 1000 {
		return Version{}, auth.ErrInvalid
	}
	v := Version{ID: domain.NewID(), Scope: scope, Policy: canonical(p), ActorID: session.User.ID, Reason: reason}
	v.Hash = hash(v.Policy)
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if err := checkScope(ctx, tx, a, scope, true); err != nil {
			return err
		}
		b, _ := json.Marshal(v.Policy)
		if err := tx.QueryRow(ctx, `INSERT INTO policy_versions(org_id,id,scope_kind,scope_id,document,policy_hash,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at`, orgID, v.ID, scope.Kind, scope.ID, b, v.Hash, a.UserID, reason).Scan(&v.CreatedAt); err != nil {
			return err
		}
		return emit(ctx, tx, orgID, a.UserID, "policy.proposed", v.ID, requestID, map[string]any{"scope": scope, "hash": v.Hash, "reason": reason})
	})
	return v, err
}

func readVersion(ctx context.Context, tx pgx.Tx, orgID, id string) (Version, error) {
	var v Version
	var b []byte
	if !auth.ValidID(id) {
		return v, auth.ErrInvalid
	}
	err := tx.QueryRow(ctx, `SELECT id::text,scope_kind,scope_id::text,document,policy_hash,actor_id::text,reason,created_at FROM policy_versions WHERE org_id=$1 AND id=$2`, orgID, id).Scan(&v.ID, &v.Scope.Kind, &v.Scope.ID, &b, &v.Hash, &v.ActorID, &v.Reason, &v.CreatedAt)
	if err != nil {
		return v, hidden(err)
	}
	err = json.Unmarshal(b, &v.Policy)
	return v, err
}

func (s *Service) GetVersion(ctx context.Context, session auth.Session, orgID, id string) (Version, error) {
	var v Version
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		v, err = readVersion(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		return checkScope(ctx, tx, a, v.Scope, false)
	})
	return v, err
}

func (s *Service) preview(ctx context.Context, tx pgx.Tx, orgID, repoID, primary string, v Version) (Resolved, string, error) {
	if primary != "" && (v.Scope.Kind != "repository" || !auth.ValidID(primary)) {
		return Resolved{}, "", auth.ErrInvalid
	}
	if v.Scope.Kind == "repository" && repoID != v.Scope.ID {
		return Resolved{}, "", auth.ErrForbidden
	}
	if v.Scope.Kind == "team" {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_repositories WHERE org_id=$1 AND team_id=$2 AND repository_id::text=$3)`, orgID, v.Scope.ID, repoID).Scan(&ok); err != nil {
			return Resolved{}, "", err
		}
		if !ok {
			return Resolved{}, "", auth.ErrForbidden
		}
	}
	if primary != "" {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_repositories WHERE org_id=$1 AND repository_id::text=$2 AND team_id=$3)`, orgID, repoID, primary).Scan(&ok); err != nil {
			return Resolved{}, "", err
		}
		if !ok {
			return Resolved{}, "", auth.ErrForbidden
		}
	}
	r, err := s.ResolveTx(ctx, tx, orgID, repoID)
	if err != nil {
		return r, "", err
	}
	currentHash := r.Hash
	var bindingVersion int64
	for i, l := range r.Layers {
		if l.Scope == v.Scope {
			bindingVersion = l.BindingVersion
			r.Layers = append(r.Layers[:i], r.Layers[i+1:]...)
			break
		}
	}
	r.Layers = append(r.Layers, Layer{Scope: v.Scope, VersionID: v.ID, BindingVersion: bindingVersion + 1, Policy: v.Policy})
	if v.Scope.Kind != "repository" {
		primary = r.PrimaryTeamID
	}
	resolved := Resolve(r.Layers, repoID, primary, r.ScopePaused)
	return resolved, hash(struct {
		Current  string
		Proposed string
		Scope    Scope
		Version  string
	}{currentHash, resolved.Hash, v.Scope, v.ID}), nil
}

func (s *Service) Simulate(ctx context.Context, session auth.Session, orgID, versionID, repoID, primaryTeamID string, in Input) (Simulation, error) {
	var out Simulation
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		v, err := readVersion(ctx, tx, orgID, versionID)
		if err != nil {
			return err
		}
		if err = checkScope(ctx, tx, a, v.Scope, true); err != nil {
			return err
		}
		if repoID != "" && !auth.CanReadRepository(a, repoID) {
			return auth.ErrForbidden
		}
		out.Resolved, out.Hash, err = s.preview(ctx, tx, orgID, repoID, primaryTeamID, v)
		if err != nil {
			return err
		}
		in.Now = time.Now().UTC()
		out.Decision = Evaluate(out.Resolved, in)
		return nil
	})
	return out, err
}

func (s *Service) Activate(ctx context.Context, session auth.Session, orgID, versionID, repoID, primaryTeamID string, expected int64, simulationHash, reason, requestID string) (int64, error) {
	reason = strings.TrimSpace(reason)
	if expected < 0 || simulationHash == "" || reason == "" || len(reason) > 1000 {
		return 0, auth.ErrInvalid
	}
	var version int64
	err := s.auth.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		v, err := readVersion(ctx, tx, orgID, versionID)
		if err != nil {
			return err
		}
		if err = checkScope(ctx, tx, a, v.Scope, true); err != nil {
			return err
		}
		if repoID != "" && !auth.CanReadRepository(a, repoID) {
			return auth.ErrForbidden
		}
		preview, proof, err := s.preview(ctx, tx, orgID, repoID, primaryTeamID, v)
		if err != nil {
			return err
		}
		if proof != simulationHash {
			return auth.ErrConflict
		}
		if len(preview.Problems) > 0 {
			return auth.ErrInvalid
		}
		err = tx.QueryRow(ctx, `SELECT version FROM policy_bindings WHERE org_id=$1 AND scope_kind=$2 AND scope_id=$3`, orgID, v.Scope.Kind, v.Scope.ID).Scan(&version)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if version != expected {
			return auth.ErrConflict
		}
		version++
		_, err = tx.Exec(ctx, `INSERT INTO policy_bindings(org_id,scope_kind,scope_id,policy_version_id,version,primary_team_id,simulation_hash) VALUES($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7) ON CONFLICT(org_id,scope_kind,scope_id) DO UPDATE SET policy_version_id=EXCLUDED.policy_version_id,version=EXCLUDED.version,primary_team_id=EXCLUDED.primary_team_id,simulation_hash=EXCLUDED.simulation_hash`, orgID, v.Scope.Kind, v.Scope.ID, v.ID, version, primaryTeamID, proof)
		if err != nil {
			return err
		}
		return emit(ctx, tx, orgID, a.UserID, "policy.changed", v.ID, requestID, map[string]any{"scope": v.Scope, "binding_version": version, "hash": v.Hash, "reason": reason, "simulation_hash": proof})
	})
	return version, err
}

func checkScope(ctx context.Context, tx pgx.Tx, a domain.Actor, scope Scope, write bool) error {
	if !auth.ValidID(scope.ID) {
		return auth.ErrInvalid
	}
	if write && a.Role != domain.Owner && a.Role != domain.Admin {
		return auth.ErrForbidden
	}
	var ok bool
	switch scope.Kind {
	case "organisation":
		if scope.ID != a.OrgID || (write && a.Role != domain.Owner) {
			return auth.ErrForbidden
		}
		return nil
	case "team":
		if !a.AllRepositories && !contains(a.TeamIDs, scope.ID) {
			return auth.ErrForbidden
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE org_id=$1 AND id=$2)`, a.OrgID, scope.ID).Scan(&ok); err != nil {
			return err
		}
	case "repository":
		if !auth.CanReadRepository(a, scope.ID) {
			return auth.ErrForbidden
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE org_id=$1 AND id=$2)`, a.OrgID, scope.ID).Scan(&ok); err != nil {
			return err
		}
	default:
		return auth.ErrInvalid
	}
	if !ok {
		return auth.ErrForbidden
	}
	return nil
}

func (s *Service) ListVersions(ctx context.Context, session auth.Session, orgID string, scope Scope, limit int, cursor string) (domain.Page[Version], error) {
	page := domain.Page[Version]{Items: []Version{}}
	if limit < 1 || limit > 200 || (cursor != "" && !auth.ValidID(cursor)) {
		return page, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if err := checkScope(ctx, tx, a, scope, false); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text,document,policy_hash,actor_id::text,reason,created_at FROM policy_versions WHERE org_id=$1 AND scope_kind=$2 AND scope_id=$3 AND id::text>$4 ORDER BY id LIMIT $5`, orgID, scope.Kind, scope.ID, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v := Version{Scope: scope}
			var raw []byte
			if err = rows.Scan(&v.ID, &raw, &v.Hash, &v.ActorID, &v.Reason, &v.CreatedAt); err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &v.Policy); err != nil {
				return err
			}
			page.Items = append(page.Items, v)
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
func hidden(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrForbidden
	}
	return err
}
func emit(ctx context.Context, tx pgx.Tx, org, actor, action, object, request string, data any) error {
	b, _ := json.Marshal(data)
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), org, actor, action, object, request, b)
	return err
}
