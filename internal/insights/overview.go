package insights

import (
	"context"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
)

type OverviewCounts struct {
	NeedsDecision          int64 `json:"needs_decision"`
	Running                int64 `json:"running"`
	ReadyForReview         int64 `json:"ready_for_review"`
	Blocked                int64 `json:"blocked"`
	VerifiedDeployments    int64 `json:"verified_deployments"`
	AccessibleRepositories int64 `json:"accessible_repositories"`
	StaleRepositories      int64 `json:"stale_repositories"`
	QueuedJobs             int64 `json:"queued_jobs"`
}

type Attention struct {
	ID             string `json:"id"`
	RepositoryID   string `json:"repository_id"`
	RepositoryName string `json:"repository_name"`
	Title          string `json:"title"`
	Severity       string `json:"severity"`
	State          string `json:"state"`
	AgeSeconds     int64  `json:"age_seconds"`
	AssignedTo     string `json:"assigned_to,omitempty"`
}

type Overview struct {
	Counts    OverviewCounts `json:"counts"`
	Attention []Attention    `json:"attention"`
}

const activeTaskStates = `('queued','reproducing','planning','repairing','validating','publishing')`
const activeOperationStates = `('requested','dispatching','queued','reconciling','awaiting_gates','running','verifying')`

func (s *Service) Overview(ctx context.Context, session auth.Session, org string) (Overview, error) {
	out := Overview{Attention: []Attention{}}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		repo := `($2 OR repository_id=ANY($3::uuid[]))`
		counts := `SELECT
			(SELECT count(*) FROM maintenance_findings WHERE org_id=$1 AND state='open' AND ` + repo + `),
			(SELECT count(*) FROM workflow_tasks WHERE org_id=$1 AND state IN ` + activeTaskStates + ` AND ` + repo + `)
				+(SELECT count(*) FROM merge_operations WHERE org_id=$1 AND state IN ` + activeOperationStates + ` AND ` + repo + `)
				+(SELECT count(*) FROM deployments WHERE org_id=$1 AND state IN ` + activeOperationStates + ` AND ` + repo + `),
			(SELECT count(*) FROM repair_runs WHERE org_id=$1 AND state='published' AND ` + repo + `),
			(SELECT count(*) FROM workflow_tasks WHERE org_id=$1 AND state='blocked' AND ` + repo + `)
				+(SELECT count(*) FROM merge_operations WHERE org_id=$1 AND state='blocked' AND ` + repo + `)
				+(SELECT count(*) FROM deployments WHERE org_id=$1 AND state='blocked' AND ` + repo + `),
			(SELECT count(*) FROM deployments WHERE org_id=$1 AND state='healthy' AND ` + repo + `),
			(SELECT count(*) FROM repositories WHERE org_id=$1 AND accessible AND ($2 OR id=ANY($3::uuid[]))),
			(SELECT count(*) FROM repositories WHERE org_id=$1 AND accessible AND (last_synced_at IS NULL OR last_synced_at<now()-interval '1 day') AND ($2 OR id=ANY($3::uuid[]))),
			(SELECT count(*) FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id WHERE j.org_id=$1 AND j.state='queued' AND ($2 OR t.repository_id=ANY($3::uuid[])))`
		if e := tx.QueryRow(ctx, counts, org, a.AllRepositories, a.RepositoryIDs).Scan(
			&out.Counts.NeedsDecision, &out.Counts.Running, &out.Counts.ReadyForReview, &out.Counts.Blocked,
			&out.Counts.VerifiedDeployments, &out.Counts.AccessibleRepositories, &out.Counts.StaleRepositories, &out.Counts.QueuedJobs,
		); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT f.id::text,f.repository_id::text,coalesce(r.name,''),f.title,f.severity,f.state,extract(epoch FROM now()-f.last_seen)::bigint,coalesce(f.assigned_to::text,'') FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.state='open' AND `+repo+` ORDER BY CASE f.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END,f.last_seen LIMIT 20`, org, a.AllRepositories, a.RepositoryIDs)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var item Attention
			if e = rows.Scan(&item.ID, &item.RepositoryID, &item.RepositoryName, &item.Title, &item.Severity, &item.State, &item.AgeSeconds, &item.AssignedTo); e != nil {
				return e
			}
			out.Attention = append(out.Attention, item)
		}
		return rows.Err()
	})
	return out, err
}
