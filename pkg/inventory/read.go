package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func (s *Service) Jobs(ctx context.Context, session auth.Session, org string, limit int, cursor string) (domain.Page[Job], error) {
	out := domain.Page[Job]{Items: []Job{}}
	if !validPage(limit, cursor) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+jobColumns+` FROM inventory_jobs WHERE org_id=$1 AND ($2='' OR id>nullif($2,'')::uuid) ORDER BY id LIMIT $3`, org, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			j, err := scanJob(rows)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, j)
		}
		return rows.Err()
	})
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	out.Complete = out.NextCursor == ""
	return out, err
}
func (s *Service) Job(ctx context.Context, session auth.Session, org, id string) (Job, error) {
	var j Job
	if !auth.ValidID(id) {
		return j, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		var err error
		j, err = scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM inventory_jobs WHERE org_id=$1 AND id=$2`, org, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		return err
	})
	return j, err
}
func (s *Service) Candidates(ctx context.Context, session auth.Session, org, id string, limit int, cursor string) (domain.Page[forge.Repository], error) {
	out := domain.Page[forge.Repository]{Items: []forge.Repository{}}
	if !auth.ValidID(id) || limit < 1 || limit > 200 || cursor != "" && !validNative(cursor) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := owner(a); err != nil {
			return err
		}
		var found bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_jobs WHERE org_id=$1 AND id=$2 AND kind='scan')`, org, id).Scan(&found); err != nil {
			return err
		}
		if !found {
			return auth.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT repository FROM inventory_candidates WHERE org_id=$1 AND job_id=$2 AND native_id COLLATE "C">$3 COLLATE "C" ORDER BY native_id COLLATE "C" LIMIT $4`, org, id, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var body []byte
			var r forge.Repository
			if err = rows.Scan(&body); err != nil {
				return err
			}
			if err = json.Unmarshal(body, &r); err != nil {
				return err
			}
			out.Items = append(out.Items, r)
		}
		return rows.Err()
	})
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].NativeID
	}
	out.Complete = out.NextCursor == ""
	return out, err
}

const repositoryColumns = `r.id::text,r.org_id::text,coalesce(r.connection_id::text,''),r.native_id,r.name,r.url,r.default_branch,r.provider,r.archived,r.paused,r.accessible,r.last_synced_at,r.version,coalesce((SELECT array_agg(tr.team_id::text ORDER BY tr.team_id) FROM team_repositories tr WHERE tr.org_id=r.org_id AND tr.repository_id=r.id),'{}'),CASE WHEN s.repository_id IS NULL THEN 'unknown' WHEN s.connection_version<>c.version OR s.namespace<>coalesce(c.settings->>'namespace','') OR c.state IN ('revoked','disabled') OR r.last_synced_at<clock_timestamp()-interval '1 hour' THEN 'stale' ELSE s.state END,CASE WHEN s.connection_version<>c.version OR s.namespace<>coalesce(c.settings->>'namespace','') THEN 'Connection configuration changed; run a complete sync' ELSE coalesce(s.reason,'No completed inventory observation') END,coalesce(s.connection_version,0),s.changes_observed_at,coalesce((SELECT p.version FROM workflow_pauses p WHERE p.org_id=r.org_id AND p.scope_kind='repository' AND p.scope_id=r.id::text),0)`
const repositoryJoin = ` FROM repositories r LEFT JOIN inventory_repository_state s ON s.org_id=r.org_id AND s.repository_id=r.id LEFT JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id `

func scanRepository(row pgx.Row) (Repository, error) {
	var r Repository
	err := row.Scan(&r.ID, &r.OrgID, &r.ConnectionID, &r.NativeID, &r.Name, &r.URL, &r.DefaultBranch, &r.Provider, &r.Archived, &r.Paused, &r.Accessible, &r.LastSyncedAt, &r.Version, &r.TeamIDs, &r.SyncState, &r.SyncReason, &r.ConnectionVersion, &r.ChangesObservedAt, &r.PauseVersion)
	return r, err
}
func (s *Service) Repositories(ctx context.Context, session auth.Session, org string, limit int, cursor string) (domain.Page[Repository], error) {
	return s.RepositoriesFiltered(ctx, session, org, limit, cursor, RepositoryFilter{})
}
func (s *Service) RepositoriesFiltered(ctx context.Context, session auth.Session, org string, limit int, cursor string, filter RepositoryFilter) (domain.Page[Repository], error) {
	out := domain.Page[Repository]{Items: []Repository{}}
	filter.Query = strings.TrimSpace(filter.Query)
	if len(filter.Query) > 256 || filter.TeamID != "" && !auth.ValidID(filter.TeamID) || filter.Provider != "" && filter.Provider != "github" && filter.Provider != "gitlab" && filter.Provider != "gitea" {
		return out, auth.ErrInvalid
	}
	switch filter.Status {
	case "", "active", "archived", "paused", "missing", "stale":
	default:
		return out, auth.ErrInvalid
	}
	if !validPage(limit, cursor) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT `+repositoryColumns+repositoryJoin+` WHERE r.org_id=$1 AND ($2 OR r.id=ANY($3::uuid[])) AND ($4='' OR r.id>nullif($4,'')::uuid) AND ($6='' OR strpos(lower(r.name),lower($6))>0) AND ($7='' OR r.provider=$7) AND ($8='' OR EXISTS(SELECT 1 FROM team_repositories tr WHERE tr.org_id=r.org_id AND tr.repository_id=r.id AND tr.team_id=nullif($8,'')::uuid)) AND ($9='' OR $9='archived' AND r.archived OR $9='paused' AND r.paused OR $9='missing' AND NOT r.accessible OR $9='stale' AND (s.repository_id IS NULL OR s.state<>'fresh' OR s.connection_version<>c.version OR s.namespace<>coalesce(c.settings->>'namespace','') OR c.state IN ('revoked','disabled') OR r.last_synced_at<clock_timestamp()-interval '1 hour') OR $9='active' AND r.accessible AND NOT r.archived AND NOT r.paused) ORDER BY r.id LIMIT $5`, org, a.AllRepositories, a.RepositoryIDs, cursor, limit+1, filter.Query, filter.Provider, filter.TeamID, filter.Status)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRepository(rows)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, r)
		}
		return rows.Err()
	})
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	out.Complete = out.NextCursor == ""
	return out, err
}
func (s *Service) Repository(ctx context.Context, session auth.Session, org, id string) (Repository, error) {
	var r Repository
	if !auth.ValidID(id) {
		return r, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, id) {
			return auth.ErrForbidden
		}
		var err error
		r, err = scanRepository(tx.QueryRow(ctx, `SELECT `+repositoryColumns+repositoryJoin+` WHERE r.org_id=$1 AND r.id=$2`, org, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		return err
	})
	return r, err
}
func (s *Service) Changes(ctx context.Context, session auth.Session, org, repo string, limit int, cursor string) (ChangePage, error) {
	out := ChangePage{Page: domain.Page[forge.Change]{Items: []forge.Change{}}, SnapshotState: "stale"}
	if !auth.ValidID(repo) || limit < 1 || limit > 200 || cursor != "" && !validNative(cursor) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		err := tx.QueryRow(ctx, `SELECT s.changes_observed_at,s.connection_version,CASE WHEN r.accessible AND NOT r.archived AND s.state='fresh' AND r.last_synced_at>clock_timestamp()-interval '1 hour' AND s.changes_observed_at>clock_timestamp()-interval '15 minutes' AND s.connection_version=c.version AND s.namespace=coalesce(c.settings->>'namespace','') AND c.state NOT IN ('revoked','disabled') THEN 'fresh' ELSE 'stale' END FROM inventory_repository_state s JOIN connections c ON c.org_id=s.org_id AND c.id=s.connection_id JOIN repositories r ON r.org_id=s.org_id AND r.id=s.repository_id WHERE s.org_id=$1 AND s.repository_id=$2`, org, repo).Scan(&out.ObservedAt, &out.ConnectionVersion, &out.SnapshotState)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT snapshot FROM inventory_changes WHERE org_id=$1 AND repository_id=$2 AND native_id COLLATE "C">$3 COLLATE "C" ORDER BY native_id COLLATE "C" LIMIT $4`, org, repo, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var body []byte
			var c forge.Change
			if err = rows.Scan(&body); err != nil {
				return err
			}
			if err = json.Unmarshal(body, &c); err != nil {
				return err
			}
			out.Items = append(out.Items, c)
		}
		return rows.Err()
	})
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	out.Complete = out.NextCursor == ""
	return out, err
}
