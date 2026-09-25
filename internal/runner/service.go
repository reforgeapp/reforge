package runner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/artifact"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

type Service struct {
	db              *store.Store
	auth            *auth.Service
	workflow        *workflow.Service
	artifacts       *artifact.Local
	mu              sync.RWMutex
	operations      map[string]FixedOperation
	CompletionCheck func(context.Context, pgx.Tx, workflow.Lease, workflow.Task, workflow.Completion) error
}

func New(db *store.Store, identity *auth.Service, jobs *workflow.Service, artifacts *artifact.Local) *Service {
	return &Service{db: db, auth: identity, workflow: jobs, artifacts: artifacts, operations: map[string]FixedOperation{}}
}
func token(kind, org, id string) string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return kind + "/" + org + "/" + id + "/" + base64.RawURLEncoding.EncodeToString(raw)
}
func hash(raw string) string { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }
func matches(raw, digest string) bool {
	return subtle.ConstantTimeCompare([]byte(hash(raw)), []byte(digest)) == 1
}
func parse(raw, kind string) (string, string, error) {
	parts := strings.Split(raw, "/")
	if len(parts) != 4 || parts[0] != kind || !auth.ValidID(parts[1]) || !auth.ValidID(parts[2]) {
		return "", "", auth.ErrUnauthenticated
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(secret) != 32 {
		return "", "", auth.ErrUnauthenticated
	}
	return parts[1], parts[2], nil
}
func lockOrg(ctx context.Context, tx pgx.Tx, org string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrUnauthenticated
	}
	return err
}
func audit(ctx context.Context, tx pgx.Tx, org, actor, action, id, request string, details ...any) error {
	data := []byte(`{}`)
	if len(details) > 0 {
		data, _ = json.Marshal(details[0])
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), org, actor, action, id, request, data)
	return err
}
func (s *Service) PutPool(ctx context.Context, session auth.Session, org, id string, in PoolInput, expected int64, request string) (Pool, error) {
	var p Pool
	if id == "" {
		id = domain.NewID()
	} else if !auth.ValidID(id) {
		return p, auth.ErrInvalid
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 160 || len(in.RepositoryIDs) > 1000 {
		return p, auth.ErrInvalid
	}
	if in.State == "" {
		in.State = "active"
	}
	if in.State != "active" && in.State != "draining" && in.State != "revoked" {
		return p, auth.ErrInvalid
	}
	seen := map[string]bool{}
	for _, repo := range in.RepositoryIDs {
		if !auth.ValidID(repo) || seen[repo] {
			return p, auth.ErrInvalid
		}
		seen[repo] = true
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		for _, repo := range in.RepositoryIDs {
			if !auth.CanReadRepository(a, repo) {
				return auth.ErrForbidden
			}
		}
		var found int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM repositories WHERE org_id=$1 AND id=ANY($2::uuid[])`, org, in.RepositoryIDs).Scan(&found); err != nil {
			return err
		}
		if found != len(in.RepositoryIDs) {
			return auth.ErrForbidden
		}
		var version int64
		var state, name string
		var builtin bool
		err := tx.QueryRow(ctx, `SELECT version,state,name,builtin FROM runner_pools WHERE org_id=$1 AND id=$2`, org, id).Scan(&version, &state, &name, &builtin)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if version > 0 {
			if err = checkPoolAccess(ctx, tx, a, id); err != nil {
				return err
			}
		}
		if expected != version || state == "revoked" {
			return auth.ErrConflict
		}
		if builtin {
			if in.State == "revoked" {
				return auth.ErrInvalid
			}
			in.Name = name
			if err = tx.QueryRow(ctx, `SELECT coalesce(array_agg(repository_id::text),'{}') FROM runner_pool_repositories WHERE org_id=$1 AND pool_id=$2`, org, id).Scan(&in.RepositoryIDs); err != nil {
				return err
			}
		}
		p = Pool{ID: id, OrgID: org, Name: strings.TrimSpace(in.Name), State: in.State, RepositoryIDs: append([]string{}, in.RepositoryIDs...), Version: version + 1, Builtin: builtin}
		if _, err = tx.Exec(ctx, `INSERT INTO runner_pools(org_id,id,name,state,version) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,id) DO UPDATE SET name=EXCLUDED.name,state=EXCLUDED.state,version=EXCLUDED.version`, org, id, p.Name, p.State, p.Version); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM runner_pool_repositories WHERE org_id=$1 AND pool_id=$2`, org, id); err != nil {
			return err
		}
		for _, repo := range in.RepositoryIDs {
			if _, err = tx.Exec(ctx, `INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) VALUES($1,$2,$3)`, org, id, repo); err != nil {
				return err
			}
		}
		if version > 0 {
			if _, err = tx.Exec(ctx, `UPDATE runner_enrollments SET consumed_at=clock_timestamp() WHERE org_id=$1 AND pool_id=$2 AND consumed_at IS NULL`, org, id); err != nil {
				return err
			}
			if err = s.invalidatePoolTx(ctx, tx, org, id, in.State == "revoked"); err != nil {
				return err
			}
		}
		return audit(ctx, tx, org, a.UserID, "runner.pool_changed", id, request, p)
	})
	return p, err
}
func (s *Service) invalidatePoolTx(ctx context.Context, tx pgx.Tx, org, pool string, revoke bool) error {
	rows, err := tx.Query(ctx, `SELECT id::text FROM runners WHERE org_id=$1 AND pool_id=$2`, org, pool)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.invalidateTx(ctx, tx, org, id, revoke); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) invalidateTx(ctx context.Context, tx pgx.Tx, org, id string, revoke bool) error {
	if revoke {
		if _, err := tx.Exec(ctx, `UPDATE runners SET state='revoked',version=version+1 WHERE org_id=$1 AND id=$2`, org, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE runner_job_credentials SET revoked_at=clock_timestamp() WHERE org_id=$1 AND runner_id=$2 AND revoked_at IS NULL`, org, id); err != nil {
		return err
	}
	return s.workflow.InvalidateWorkerTx(ctx, tx, org, id)
}
func (s *Service) EnrollToken(ctx context.Context, session auth.Session, org, pool, request string) (Credential, error) {
	var c Credential
	if !auth.ValidID(pool) {
		return c, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if err := checkPoolAccess(ctx, tx, a, pool); err != nil {
			return err
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT state='active' FROM runner_pools WHERE org_id=$1 AND id=$2`, org, pool).Scan(&active); err != nil || !active {
			return auth.ErrForbidden
		}
		id := domain.NewID()
		c.Token = token("enr", org, id)
		c.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
		if _, err := tx.Exec(ctx, `INSERT INTO runner_enrollments(org_id,id,pool_id,token_hash,expires_at,created_by) VALUES($1,$2,$3,$4,$5,$6)`, org, id, pool, hash(c.Token), c.ExpiresAt, a.UserID); err != nil {
			return err
		}
		return audit(ctx, tx, org, a.UserID, "runner.enrollment_created", id, request, map[string]any{"pool_id": pool, "expires_at": c.ExpiresAt})
	})
	if err != nil {
		c.Token = ""
	}
	return c, err
}
func (s *Service) Enroll(ctx context.Context, raw, name string) (Credential, error) {
	var c Credential
	org, id, err := parse(raw, "enr")
	if err != nil {
		return c, err
	}
	if strings.TrimSpace(name) == "" || len(name) > 160 {
		return c, auth.ErrInvalid
	}
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return err
		}
		var digest, pool, owner string
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT e.token_hash,e.pool_id::text,e.created_by::text,e.consumed_at IS NULL AND e.expires_at>clock_timestamp() AND p.state='active' FROM runner_enrollments e JOIN runner_pools p ON p.org_id=e.org_id AND p.id=e.pool_id WHERE e.org_id=$1 AND e.id=$2 FOR UPDATE OF e`, org, id).Scan(&digest, &pool, &owner, &valid); err != nil || !valid || !matches(raw, digest) {
			return auth.ErrUnauthenticated
		}
		var authorized bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m WHERE m.org_id=$1 AND m.user_id=$2 AND m.role='owner' AND (m.all_repositories OR NOT EXISTS(SELECT 1 FROM runner_pool_repositories rp WHERE rp.org_id=m.org_id AND rp.pool_id=$3 AND NOT EXISTS(SELECT 1 FROM member_repositories mr WHERE mr.org_id=m.org_id AND mr.user_id=m.user_id AND mr.repository_id=rp.repository_id) AND NOT EXISTS(SELECT 1 FROM team_repositories tr JOIN team_memberships tm ON tm.org_id=tr.org_id AND tm.team_id=tr.team_id WHERE tm.org_id=m.org_id AND tm.user_id=m.user_id AND tr.repository_id=rp.repository_id))))`, org, owner, pool).Scan(&authorized); err != nil {
			return err
		}
		if !authorized {
			return auth.ErrUnauthenticated
		}
		c.Runner = Runner{ID: domain.NewID(), OrgID: org, PoolID: pool, Name: strings.TrimSpace(name), State: "active", Version: 1, CredentialExpiresAt: time.Now().UTC().Add(24 * time.Hour)}
		c.ExpiresAt = c.Runner.CredentialExpiresAt
		c.Token = token("sup", org, c.Runner.ID)
		if _, err := tx.Exec(ctx, `INSERT INTO runners(org_id,id,pool_id,name,credential_hash,credential_expires_at) VALUES($1,$2,$3,$4,$5,$6)`, org, c.Runner.ID, pool, c.Runner.Name, hash(c.Token), c.ExpiresAt); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE runner_enrollments SET consumed_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, id)
		if err != nil {
			return err
		}
		return audit(ctx, tx, org, owner, "runner.enrolled", c.Runner.ID, "", map[string]string{"pool_id": pool})
	})
	if err != nil {
		c.Token = ""
	}
	return c, err
}
func supervisorTx(ctx context.Context, tx pgx.Tx, org, id, raw string) (Runner, error) {
	var r Runner
	var digest string
	var valid bool
	err := tx.QueryRow(ctx, `SELECT r.id::text,r.org_id::text,r.pool_id::text,r.name,r.state,r.version,r.credential_expires_at,r.credential_hash,r.state='active' AND r.credential_expires_at>clock_timestamp() AND p.state='active' FROM runners r JOIN runner_pools p ON p.org_id=r.org_id AND p.id=r.pool_id WHERE r.org_id=$1 AND r.id=$2 FOR UPDATE OF r`, org, id).Scan(&r.ID, &r.OrgID, &r.PoolID, &r.Name, &r.State, &r.Version, &r.CredentialExpiresAt, &digest, &valid)
	if err != nil || !valid || !matches(raw, digest) {
		return Runner{}, auth.ErrUnauthenticated
	}
	return r, nil
}
func (s *Service) Rotate(ctx context.Context, raw string) (Credential, error) {
	var c Credential
	org, id, err := parse(raw, "sup")
	if err != nil {
		return c, err
	}
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return err
		}
		r, err := supervisorTx(ctx, tx, org, id, raw)
		if err != nil {
			return err
		}
		c.Runner = r
		c.Token = token("sup", org, id)
		c.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
		c.Runner.CredentialExpiresAt = c.ExpiresAt
		c.Runner.Version++
		if err = s.invalidateTx(ctx, tx, org, id, false); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE runners SET credential_hash=$3,credential_expires_at=$4,credential_version=credential_version+1,version=version+1 WHERE org_id=$1 AND id=$2`, org, id, hash(c.Token), c.ExpiresAt)
		if err != nil {
			return err
		}
		return audit(ctx, tx, org, "runner:"+id, "runner.credential_rotated", id, "", map[string]int64{"version": c.Runner.Version})
	})
	if err != nil {
		c.Token = ""
	}
	return c, err
}
func (s *Service) Revoke(ctx context.Context, session auth.Session, org, id string, expected int64, request string) error {
	if !auth.ValidID(id) {
		return auth.ErrInvalid
	}
	return s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		var version int64
		var pool string
		if err := tx.QueryRow(ctx, `SELECT version,pool_id::text FROM runners WHERE org_id=$1 AND id=$2`, org, id).Scan(&version, &pool); err != nil {
			return auth.ErrForbidden
		}
		if err := checkPoolAccess(ctx, tx, a, pool); err != nil {
			return err
		}
		if expected != version {
			return auth.ErrConflict
		}
		if err := s.invalidateTx(ctx, tx, org, id, true); err != nil {
			return err
		}
		return audit(ctx, tx, org, a.UserID, "runner.revoked", id, request, map[string]int64{"version": expected + 1})
	})
}
func (s *Service) CheckRunnerTx(ctx context.Context, tx pgx.Tx, org, id string) error {
	if !auth.ValidID(org) || !auth.ValidID(id) {
		return auth.ErrForbidden
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT r.state='active' AND r.credential_expires_at>clock_timestamp() AND p.state='active' FROM runners r JOIN runner_pools p ON p.org_id=r.org_id AND p.id=r.pool_id WHERE r.org_id=$1 AND r.id=$2`, org, id).Scan(&active); err != nil || !active {
		return auth.ErrForbidden
	}
	return nil
}
func (s *Service) CheckScopeTx(ctx context.Context, tx pgx.Tx, org, kind, id string) error {
	if kind != "runner_pool" {
		return auth.ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runner_pools WHERE org_id=$1 AND id=$2 AND state='active')`, org, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return auth.ErrForbidden
	}
	return nil
}

func (s *Service) Pools(ctx context.Context, session auth.Session, org string, filter PoolFilter, limit int, cursor string) (domain.Page[Pool], error) {
	page := domain.Page[Pool]{Items: []Pool{}}
	if limit < 1 || limit > 200 || (cursor != "" && !auth.ValidID(cursor)) || len(filter.Query) > 200 || (filter.State != "" && filter.State != "active" && filter.State != "draining" && filter.State != "revoked") {
		return page, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT p.id::text,p.org_id::text,p.name,p.state,p.version,coalesce(array_agg(DISTINCT r.repository_id::text ORDER BY r.repository_id::text) FILTER(WHERE r.repository_id IS NOT NULL),'{}'),(SELECT count(*) FROM runners ru WHERE ru.org_id=p.org_id AND ru.pool_id=p.id AND ru.state='active'),(SELECT count(*) FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id WHERE t.org_id=p.org_id AND t.runner_pool_id=p.id AND j.state='running'),p.builtin FROM runner_pools p LEFT JOIN runner_pool_repositories r ON r.org_id=p.org_id AND r.pool_id=p.id WHERE p.org_id=$1 AND p.id::text>$2 AND ($3='' OR p.state=$3) AND ($4='' OR p.name ILIKE '%'||$4||'%') AND ($5 OR NOT EXISTS(SELECT 1 FROM runner_pool_repositories excluded WHERE excluded.org_id=p.org_id AND excluded.pool_id=p.id AND NOT (excluded.repository_id::text=ANY(coalesce($6::text[],'{}'))))) GROUP BY p.org_id,p.id ORDER BY p.id LIMIT $7`, org, cursor, filter.State, filter.Query, a.AllRepositories, a.RepositoryIDs, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Pool
			if err = rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.State, &p.Version, &p.RepositoryIDs, &p.RunnerCount, &p.BusySlots, &p.Builtin); err != nil {
				return err
			}
			page.Items = append(page.Items, p)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		page.Complete = len(page.Items) <= limit
		if !page.Complete {
			page.Items = page.Items[:limit]
			page.NextCursor = page.Items[limit-1].ID
		}
		return nil
	})
	return page, err
}
func (s *Service) Pool(ctx context.Context, session auth.Session, org, id string) (Pool, error) {
	var p Pool
	if !auth.ValidID(id) {
		return p, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if err := checkPoolAccess(ctx, tx, a, id); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT p.id::text,p.org_id::text,p.name,p.state,p.version,coalesce(array_agg(DISTINCT r.repository_id::text ORDER BY r.repository_id::text) FILTER(WHERE r.repository_id IS NOT NULL),'{}'),(SELECT count(*) FROM runners ru WHERE ru.org_id=p.org_id AND ru.pool_id=p.id AND ru.state='active'),(SELECT count(*) FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id WHERE t.org_id=p.org_id AND t.runner_pool_id=p.id AND j.state='running'),p.builtin FROM runner_pools p LEFT JOIN runner_pool_repositories r ON r.org_id=p.org_id AND r.pool_id=p.id WHERE p.org_id=$1 AND p.id=$2 GROUP BY p.org_id,p.id`, org, id).Scan(&p.ID, &p.OrgID, &p.Name, &p.State, &p.Version, &p.RepositoryIDs, &p.RunnerCount, &p.BusySlots, &p.Builtin)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		return err
	})
	return p, err
}

func (s *Service) Runners(ctx context.Context, session auth.Session, org, pool string, limit int, cursor string) (domain.Page[Runner], error) {
	page := domain.Page[Runner]{Items: []Runner{}}
	if !auth.ValidID(pool) || limit < 1 || limit > 200 || (cursor != "" && !auth.ValidID(cursor)) {
		return page, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if err := checkPoolAccess(ctx, tx, a, pool); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT r.id::text,r.org_id::text,r.pool_id::text,r.name,r.state,r.version,r.credential_expires_at,p.name,p.state,r.last_seen_at,r.enrolled_at,(SELECT count(*) FROM workflow_jobs j WHERE j.org_id=r.org_id AND j.lease_owner=r.id::text AND j.state='running'),(SELECT count(*) FROM connection_routes c WHERE c.org_id=r.org_id AND c.runner_id=r.id AND c.revoked_at IS NULL) FROM runners r JOIN runner_pools p ON p.org_id=r.org_id AND p.id=r.pool_id WHERE r.org_id=$1 AND r.pool_id=$2 AND r.id::text>$3 ORDER BY r.id LIMIT $4`, org, pool, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Runner
			if err = rows.Scan(&r.ID, &r.OrgID, &r.PoolID, &r.Name, &r.State, &r.Version, &r.CredentialExpiresAt, &r.PoolName, &r.PoolState, &r.LastSeenAt, &r.EnrolledAt, &r.BusySlots, &r.RouteCount); err != nil {
				return err
			}
			page.Items = append(page.Items, r)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		page.Complete = len(page.Items) <= limit
		if !page.Complete {
			page.Items = page.Items[:limit]
			page.NextCursor = page.Items[limit-1].ID
		}
		return nil
	})
	return page, err
}

func checkPoolAccess(ctx context.Context, tx pgx.Tx, a domain.Actor, pool string) error {
	rows, err := tx.Query(ctx, `SELECT repository_id::text FROM runner_pool_repositories WHERE org_id=$1 AND pool_id=$2`, a.OrgID, pool)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var repo string
		if err = rows.Scan(&repo); err != nil {
			return err
		}
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if count == 0 && (a.Role != domain.Owner || !a.AllRepositories) {
		return auth.ErrForbidden
	}
	return nil
}
