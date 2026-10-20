package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/secrets"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

type Service struct {
	db     *store.Store
	auth   *auth.Service
	vault  *secrets.Vault
	reader Reader
	decode Decoder
}

func New(db *store.Store, identity *auth.Service, vault *secrets.Vault, reader Reader, decode Decoder) *Service {
	return &Service{db: db, auth: identity, vault: vault, reader: reader, decode: decode}
}
func owner(a domain.Actor) error {
	if a.Role != domain.Owner || !a.AllRepositories {
		return auth.ErrForbidden
	}
	return nil
}
func lockOrg(ctx context.Context, tx pgx.Tx, org string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id)
}
func connection(ctx context.Context, tx pgx.Tx, org, id string) (connections.Connection, error) {
	var c connections.Connection
	var settings []byte
	err := tx.QueryRow(ctx, `SELECT id::text,org_id::text,kind,provider,state,version,settings FROM connections WHERE org_id=$1 AND id=$2 FOR SHARE`, org, id).Scan(&c.ID, &c.OrgID, &c.Kind, &c.Provider, &c.State, &c.Version, &settings)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, auth.ErrForbidden
	}
	if err != nil {
		return c, err
	}
	if c.Kind != "forge" || c.State == "revoked" || c.State == "disabled" {
		return c, ErrStale
	}
	if err = json.Unmarshal(settings, &c.Settings); err != nil {
		return c, err
	}
	return c, nil
}
func audit(ctx context.Context, tx pgx.Tx, org, actor, action, id, request string, details ...any) error {
	data := []byte(`{}`)
	if len(details) > 0 {
		var err error
		data, err = json.Marshal(details[0])
		if err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), org, actor, action, id, request, data)
	return err
}

const jobColumns = `id::text,org_id::text,connection_id::text,connection_version,namespace,kind,coalesce(repository_id::text,''),coalesce(parent_id::text,''),coalesce(requested_by::text,''),input,state,reason,cursor,phase,pages,processed,failures,fencing_token,lease_owner,lease_until,available_at,version,created_at,updated_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.OrgID, &j.ConnectionID, &j.ConnectionVersion, &j.Namespace, &j.Kind, &j.RepositoryID, &j.ParentID, &j.RequestedBy, &j.Input, &j.State, &j.Reason, &j.Cursor, &j.Phase, &j.Pages, &j.Processed, &j.Failures, &j.Fence, &j.LeaseOwner, &j.LeaseUntil, &j.AvailableAt, &j.Version, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}
func loadJob(ctx context.Context, tx pgx.Tx, org, id string) (Job, error) {
	j, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM inventory_jobs WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, id))
	if errors.Is(err, pgx.ErrNoRows) {
		err = auth.ErrForbidden
	}
	return j, err
}
func enqueue(ctx context.Context, tx pgx.Tx, c connections.Connection, kind, repo, parent, actor string, input []byte) (Job, error) {
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM inventory_jobs WHERE org_id=$1 AND state IN ('queued','running')`, c.OrgID).Scan(&count); err != nil {
		return Job{}, err
	}
	if count >= 1000 {
		return Job{}, ErrBusy
	}
	if len(input) == 0 {
		input = []byte(`{}`)
	}
	if kind == "scan" {
		if _, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET state='stale',reason='Full inventory scan pending' WHERE org_id=$1 AND connection_id=$2 AND namespace=$3`, c.OrgID, c.ID, c.Settings.Namespace); err != nil {
			return Job{}, err
		}
	}
	return scanJob(tx.QueryRow(ctx, `INSERT INTO inventory_jobs(org_id,id,connection_id,connection_version,namespace,kind,repository_id,parent_id,requested_by,input) VALUES($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid,nullif($8,'')::uuid,nullif($9,'')::uuid,$10) RETURNING `+jobColumns, c.OrgID, domain.NewID(), c.ID, c.Version, c.Settings.Namespace, kind, repo, parent, actor, input))
}
func (s *Service) StartSync(ctx context.Context, session auth.Session, org string, in SyncInput, request string) (Job, error) {
	var j Job
	if !auth.ValidID(in.ConnectionID) || len(in.Namespace) > 256 {
		return j, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		c, err := connection(ctx, tx, org, in.ConnectionID)
		if err != nil {
			return err
		}
		if in.Namespace != "" && in.Namespace != c.Settings.Namespace {
			return auth.ErrInvalid
		}
		var existing string
		err = tx.QueryRow(ctx, `SELECT id::text FROM inventory_jobs WHERE org_id=$1 AND connection_id=$2 AND kind='scan' AND state IN ('queued','running')`, org, c.ID).Scan(&existing)
		if err == nil {
			j, err = loadJob(ctx, tx, org, existing)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO inventory_sources(org_id,connection_id,namespace,poll_due) VALUES($1,$2,$3,clock_timestamp()+interval '15 minutes') ON CONFLICT(org_id,connection_id) DO UPDATE SET namespace=excluded.namespace,poll_due=excluded.poll_due`, org, c.ID, c.Settings.Namespace)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_repository_state SET refresh_due=clock_timestamp(),changes_observed_at=NULL WHERE org_id=$1 AND connection_id=$2`, org, c.ID); err != nil {
			return err
		}
		j, err = enqueue(ctx, tx, c, "scan", "", "", a.UserID, nil)
		if err != nil {
			return err
		}
		return audit(ctx, tx, org, a.UserID, "inventory.sync", j.ID, request, map[string]any{"version": j.Version, "connection_version": c.Version, "session_id": a.SessionID})
	})
	return j, err
}
func (s *Service) StartImport(ctx context.Context, session auth.Session, org, syncID string, in ImportInput, expected int64, request string) (Job, error) {
	var j Job
	if !auth.ValidID(syncID) || expected < 1 || in.All == (len(in.NativeIDs) > 0) || len(in.NativeIDs) > 10000 || len(in.TeamIDs) > 100 {
		return j, auth.ErrInvalid
	}
	ids := map[string]bool{}
	for _, id := range in.NativeIDs {
		if !validNative(id) || ids[id] {
			return j, auth.ErrInvalid
		}
		ids[id] = true
	}
	ids = map[string]bool{}
	for _, id := range in.TeamIDs {
		if !auth.ValidID(id) || ids[id] {
			return j, auth.ErrInvalid
		}
		ids[id] = true
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		source, err := loadJob(ctx, tx, org, syncID)
		if err != nil {
			return err
		}
		if source.Kind != "scan" || source.State != "complete" || source.Version != expected {
			return auth.ErrConflict
		}
		if err := currentScan(ctx, tx, Job{OrgID: org, ParentID: source.ID}); err != nil {
			return err
		}
		c, err := connection(ctx, tx, org, source.ConnectionID)
		if err != nil {
			return err
		}
		if c.Version != source.ConnectionVersion || c.Settings.Namespace != source.Namespace {
			return ErrStale
		}
		if !in.All {
			var n int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM inventory_candidates WHERE org_id=$1 AND job_id=$2 AND native_id=ANY($3::text[])`, org, syncID, in.NativeIDs).Scan(&n); err != nil {
				return err
			}
			if n != len(in.NativeIDs) {
				return auth.ErrInvalid
			}
		}
		plan := importPlan{ImportInput: in, TeamVersions: map[string]int64{}}
		for _, id := range in.TeamIDs {
			var version int64
			if err = tx.QueryRow(ctx, `SELECT version FROM teams WHERE org_id=$1 AND id=$2 FOR SHARE`, org, id).Scan(&version); err != nil {
				return auth.ErrForbidden
			}
			plan.TeamVersions[id] = version
		}
		body, _ := json.Marshal(plan)
		j, err = enqueue(ctx, tx, c, "import", "", syncID, a.UserID, body)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		return audit(ctx, tx, org, a.UserID, "inventory.import", j.ID, request, map[string]any{"version": j.Version, "source_sync_id": syncID, "source_version": expected, "selection_digest": hex.EncodeToString(digest[:]), "session_id": a.SessionID})
	})
	return j, err
}
func (s *Service) Cancel(ctx context.Context, session auth.Session, org, id string, expected int64, request string) (Job, error) {
	var j Job
	if !auth.ValidID(id) || expected < 1 {
		return j, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		var err error
		j, err = loadJob(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if j.Version != expected || j.State != "queued" && j.State != "running" {
			return auth.ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE inventory_jobs SET state='cancelled',reason='Cancelled by owner',fencing_token=fencing_token+1,lease_until=NULL,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, id)
		if err != nil {
			return err
		}
		if err = audit(ctx, tx, org, a.UserID, "inventory.cancel", id, request, map[string]any{"version": j.Version + 1, "session_id": a.SessionID}); err != nil {
			return err
		}
		j, err = loadJob(ctx, tx, org, id)
		return err
	})
	return j, err
}
func validNative(v string) bool {
	return v != "" && len(v) <= 256 && !strings.ContainsAny(v, "\x00\r\n")
}
func validPage(limit int, cursor string) bool {
	return limit >= 1 && limit <= 200 && (cursor == "" || auth.ValidID(cursor))
}
func RequireFreshTx(ctx context.Context, tx pgx.Tx, org, repo string) error {
	var fresh *bool
	err := tx.QueryRow(ctx, `SELECT r.accessible AND NOT r.archived AND s.state='fresh' AND s.connection_version=c.version AND s.namespace=coalesce(c.settings->>'namespace','') AND c.state NOT IN ('revoked','disabled') AND r.last_synced_at>clock_timestamp()-interval '1 hour' AND s.changes_observed_at>clock_timestamp()-interval '15 minutes' FROM repositories r JOIN inventory_repository_state s ON s.org_id=r.org_id AND s.repository_id=r.id JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2 FOR SHARE OF r,s,c`, org, repo).Scan(&fresh)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (fresh == nil || !*fresh) {
		return ErrStale
	}
	return err
}
func retryDelay(failures int, requested time.Duration) time.Duration {
	delay := time.Second * time.Duration(1<<min(failures+1, 10))
	if requested > delay {
		delay = requested
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	return delay
}

func emit(ctx context.Context, tx pgx.Tx, j Job, repo, eventType string, version int64) error {
	aggregate, kind := j.ID, "inventory_job"
	if repo != "" {
		aggregate, kind = repo, "repository"
	}
	data, _ := json.Marshal(map[string]string{"kind": j.Kind})
	return workflow.EmitTx(ctx, tx, domain.Event{OrgID: j.OrgID, RepositoryID: repo, Type: eventType, AggregateType: kind, AggregateID: aggregate, AggregateVersion: version, RequestID: j.ID, DataVersion: 1, Data: data})
}
