package inventory

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

func currentScan(ctx context.Context, tx pgx.Tx, j Job) error {
	var current bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_jobs parent WHERE parent.org_id=$1 AND parent.id=$2 AND parent.kind='scan' AND parent.state='complete' AND parent.updated_at>clock_timestamp()-interval '1 hour' AND NOT EXISTS(SELECT 1 FROM inventory_jobs newer WHERE newer.org_id=parent.org_id AND newer.connection_id=parent.connection_id AND newer.kind='scan' AND (newer.state IN ('queued','running') OR newer.state='complete' AND newer.updated_at>parent.updated_at)))`, j.OrgID, j.ParentID).Scan(&current)
	if err == nil && !current {
		return ErrStale
	}
	return err
}
func (s *Service) importPage(ctx context.Context, tx pgx.Tx, j Job, c connections.Connection) error {
	if j.Pages >= 1000 {
		return ErrIncomplete
	}
	if err := currentScan(ctx, tx, j); err != nil {
		return err
	}
	var plan importPlan
	if err := json.Unmarshal(j.Input, &plan); err != nil {
		return err
	}
	for _, id := range plan.TeamIDs {
		var version int64
		if err := tx.QueryRow(ctx, `SELECT version FROM teams WHERE org_id=$1 AND id=$2 FOR UPDATE`, j.OrgID, id).Scan(&version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return auth.ErrForbidden
			}
			return err
		}
		if version != plan.TeamVersions[id] {
			return ErrStale
		}
	}
	rows, err := tx.Query(ctx, `SELECT repository FROM inventory_candidates WHERE org_id=$1 AND job_id=$2 AND native_id COLLATE "C">$3 COLLATE "C" AND ($4 OR native_id=ANY($5::text[])) ORDER BY native_id COLLATE "C" LIMIT 101`, j.OrgID, j.ParentID, j.Cursor, plan.All, plan.NativeIDs)
	if err != nil {
		return err
	}
	batch := []forge.Repository{}
	for rows.Next() {
		var body []byte
		var r forge.Repository
		if err = rows.Scan(&body); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(body, &r); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	complete := len(batch) <= 100
	if !complete {
		batch = batch[:100]
	}
	changedTeams := map[string]bool{}
	for _, r := range batch {
		var id string
		var version int64
		err = tx.QueryRow(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name,url,default_branch,provider,archived,accessible,last_synced_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,true,clock_timestamp()) ON CONFLICT(org_id,connection_id,native_id) DO UPDATE SET name=excluded.name,url=excluded.url,default_branch=excluded.default_branch,archived=excluded.archived,accessible=true,last_synced_at=clock_timestamp(),version=repositories.version+1 RETURNING id::text,version`, j.OrgID, domain.NewID(), j.ConnectionID, r.NativeID, r.FullName, r.URL, r.DefaultBranch, c.Provider, r.Archived).Scan(&id, &version)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO inventory_repository_state(org_id,repository_id,connection_id,namespace,connection_version,scan_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(org_id,repository_id) DO UPDATE SET namespace=excluded.namespace,connection_version=excluded.connection_version,scan_id=excluded.scan_id,state='fresh',reason='',refresh_due=clock_timestamp()`, j.OrgID, id, j.ConnectionID, j.Namespace, j.ConnectionVersion, j.ParentID)
		if err != nil {
			return err
		}
		if err = emit(ctx, tx, j, id, "repository.synced", version); err != nil {
			return err
		}
		for _, team := range plan.TeamIDs {
			tag, err := tx.Exec(ctx, `INSERT INTO team_repositories(org_id,team_id,repository_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, j.OrgID, team, id)
			if err != nil {
				return err
			}
			if tag.RowsAffected() > 0 {
				changedTeams[team] = true
			}
		}
	}
	for team := range changedTeams {
		_, err = tx.Exec(ctx, `UPDATE teams SET version=version+1 WHERE org_id=$1 AND id=$2`, j.OrgID, team)
		if err != nil {
			return err
		}
		plan.TeamVersions[team]++
		if err = audit(ctx, tx, j.OrgID, j.RequestedBy, "inventory.team_grant", team, j.ID, map[string]any{"version": plan.TeamVersions[team], "job_id": j.ID}); err != nil {
			return err
		}
	}
	cursor := ""
	if len(batch) > 0 {
		cursor = batch[len(batch)-1].NativeID
	}
	body, _ := json.Marshal(plan)
	if err = audit(ctx, tx, j.OrgID, j.RequestedBy, "inventory.import_batch", j.ID, j.ID, map[string]any{"version": j.Version + 1, "processed": len(batch)}); err != nil {
		return err
	}
	return progress(ctx, tx, j, cursor, "", len(batch), complete, body)
}
