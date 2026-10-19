package insights

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
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

type PortfolioRow struct {
	RepositoryID   string     `json:"repository_id"`
	RepositoryName string     `json:"repository_name"`
	Provider       string     `json:"provider"`
	Accessible     bool       `json:"accessible"`
	OpenFindings   int64      `json:"open_findings"`
	OpenChanges    int64      `json:"open_changes"`
	Blocked        int64      `json:"blocked"`
	LastSyncedAt   *time.Time `json:"last_synced_at"`
	Blocker        string     `json:"blocker,omitempty"`
}

type Capacity struct {
	QueuedJobs    int64 `json:"queued_jobs"`
	RunningJobs   int64 `json:"running_jobs"`
	ActivePools   int64 `json:"active_pools"`
	ActiveRunners int64 `json:"active_runners"`
	ReservedMicro int64 `json:"reserved_micro_usd"`
}

type OverviewDay struct {
	Day         string `json:"day"`
	Findings    int64  `json:"findings"`
	Runs        int64  `json:"runs"`
	Merges      int64  `json:"merges"`
	Deployments int64  `json:"deployments"`
}

type SeverityCount struct {
	Severity string `json:"severity"`
	Count    int64  `json:"count"`
}

type Overview struct {
	Counts    OverviewCounts  `json:"counts"`
	Attention []Attention     `json:"attention"`
	Portfolio []PortfolioRow  `json:"portfolio"`
	Capacity  Capacity        `json:"capacity"`
	Trend     []OverviewDay   `json:"trend"`
	Severity  []SeverityCount `json:"severity"`
}

const activeTaskStates = `('queued','reproducing','planning','repairing','validating','publishing')`
const activeOperationStates = `('requested','dispatching','queued','reconciling','awaiting_gates','running','verifying')`

func (s *Service) Overview(ctx context.Context, session auth.Session, org string) (Overview, error) {
	out := Overview{Attention: []Attention{}, Trend: []OverviewDay{}, Severity: []SeverityCount{}}
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
		if e = rows.Err(); e != nil {
			return e
		}
		portfolio, e := tx.Query(ctx, `SELECT r.id::text,r.name,r.provider,r.accessible,r.last_synced_at,
				(SELECT count(*) FROM maintenance_findings f WHERE f.org_id=r.org_id AND f.repository_id=r.id AND f.state='open'),
				(SELECT count(*) FROM merge_operations m WHERE m.org_id=r.org_id AND m.repository_id=r.id AND m.state IN ('requested','dispatching','queued','reconciling')),
				(SELECT count(*) FROM workflow_tasks t WHERE t.org_id=r.org_id AND t.repository_id=r.id AND t.state='blocked')
			FROM repositories r WHERE r.org_id=$1 AND ($2 OR r.id=ANY($3::uuid[]))
			ORDER BY 6 DESC,7 DESC,r.name LIMIT 50`, org, a.AllRepositories, a.RepositoryIDs)
		if e != nil {
			return e
		}
		defer portfolio.Close()
		for portfolio.Next() {
			var item PortfolioRow
			if e = portfolio.Scan(&item.RepositoryID, &item.RepositoryName, &item.Provider, &item.Accessible, &item.LastSyncedAt, &item.OpenFindings, &item.OpenChanges, &item.Blocked); e != nil {
				return e
			}
			switch {
			case !item.Accessible:
				item.Blocker = "Repository access removed"
			case item.Blocked > 0:
				item.Blocker = "Blocked maintenance work"
			case item.LastSyncedAt == nil || time.Since(*item.LastSyncedAt) > 24*time.Hour:
				item.Blocker = "Inventory is stale"
			}
			out.Portfolio = append(out.Portfolio, item)
		}
		if e = portfolio.Err(); e != nil {
			return e
		}
		trend, e := tx.Query(ctx, `WITH days AS (SELECT to_char(d,'YYYY-MM-DD') AS day,d AT TIME ZONE 'UTC' AS s FROM generate_series(date_trunc('day',now() AT TIME ZONE 'UTC')-interval '13 days',date_trunc('day',now() AT TIME ZONE 'UTC'),interval '1 day') d)
			SELECT day,
				(SELECT count(*) FROM maintenance_findings WHERE org_id=$1 AND first_seen>=s AND first_seen<s+interval '1 day' AND `+repo+`),
				(SELECT count(*) FROM repair_runs WHERE org_id=$1 AND created_at>=s AND created_at<s+interval '1 day' AND `+repo+`),
				(SELECT count(*) FROM merge_operations WHERE org_id=$1 AND state='merged' AND updated_at>=s AND updated_at<s+interval '1 day' AND `+repo+`),
				(SELECT count(*) FROM deployments WHERE org_id=$1 AND first_healthy_at>=s AND first_healthy_at<s+interval '1 day' AND `+repo+`)
			FROM days ORDER BY day`, org, a.AllRepositories, a.RepositoryIDs)
		if e != nil {
			return e
		}
		defer trend.Close()
		for trend.Next() {
			var item OverviewDay
			if e = trend.Scan(&item.Day, &item.Findings, &item.Runs, &item.Merges, &item.Deployments); e != nil {
				return e
			}
			out.Trend = append(out.Trend, item)
		}
		if e = trend.Err(); e != nil {
			return e
		}
		severity, e := tx.Query(ctx, `SELECT severity,count(*) FROM maintenance_findings WHERE org_id=$1 AND state='open' AND `+repo+` GROUP BY 1 ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END,1`, org, a.AllRepositories, a.RepositoryIDs)
		if e != nil {
			return e
		}
		defer severity.Close()
		for severity.Next() {
			var item SeverityCount
			if e = severity.Scan(&item.Severity, &item.Count); e != nil {
				return e
			}
			out.Severity = append(out.Severity, item)
		}
		if e = severity.Err(); e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM workflow_jobs WHERE org_id=$1 AND state='queued'),
			(SELECT count(*) FROM workflow_jobs WHERE org_id=$1 AND state='running'),
			(SELECT count(*) FROM runner_pools WHERE org_id=$1 AND state='active'),
			(SELECT count(*) FROM runners WHERE org_id=$1 AND state='active'),
			(SELECT COALESCE(sum((record->'maximum'->>'micro_usd')::bigint) FILTER (WHERE state IN ('reserved','dispatched','unknown')),0) FROM budget_reservations WHERE org_id=$1)`, org).Scan(&out.Capacity.QueuedJobs, &out.Capacity.RunningJobs, &out.Capacity.ActivePools, &out.Capacity.ActiveRunners, &out.Capacity.ReservedMicro)
	})
	return out, err
}
