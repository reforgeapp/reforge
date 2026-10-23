package insights

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/environmenthealth"
	"github.com/reforgeapp/reforge/pkg/maintenance/attention"
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

type MaintenanceProgress struct {
	RepositoryID   string `json:"repository_id"`
	RepositoryName string `json:"repository_name"`
	State          string `json:"state"`
	Count          int64  `json:"count"`
}

type Overview struct {
	Maintenance []MaintenanceProgress `json:"maintenance"`
	Counts      OverviewCounts        `json:"counts"`
	Attention   []Attention           `json:"attention"`
	Portfolio   []PortfolioRow        `json:"portfolio"`
	Capacity    Capacity              `json:"capacity"`
	Trend       []OverviewDay         `json:"trend"`
	Severity    []SeverityCount       `json:"severity"`
}

func (s *Service) Overview(ctx context.Context, session auth.Session, org string) (Overview, error) {
	out := Overview{Maintenance: []MaintenanceProgress{}, Attention: []Attention{}, Trend: []OverviewDay{}, Severity: []SeverityCount{}}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		repo := `($2 OR repository_id=ANY($3::uuid[]))`
		var err error
		out.Counts.NeedsDecision, err = attention.CountTx(ctx, tx, a)
		if err != nil {
			return err
		}
		out.Counts.Running, out.Counts.Blocked, out.Counts.ReadyForReview, err = attention.WorkCountsTx(ctx, tx, a)
		if err != nil {
			return err
		}
		counts := `SELECT
			(SELECT count(*) FROM environment_health h WHERE h.org_id=$1 AND h.healthy AND ` + environmenthealth.CurrentPredicate + ` AND ` + repo + `),
			(SELECT count(*) FROM repositories WHERE org_id=$1 AND accessible AND ($2 OR id=ANY($3::uuid[]))),
			(SELECT count(*) FROM repositories WHERE org_id=$1 AND accessible AND (last_synced_at IS NULL OR last_synced_at<now()-interval '1 day') AND ($2 OR id=ANY($3::uuid[]))),
			(SELECT count(*) FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id WHERE j.org_id=$1 AND j.state='queued' AND ($2 OR t.repository_id=ANY($3::uuid[])))`
		if err = tx.QueryRow(ctx, counts, org, a.AllRepositories, a.RepositoryIDs).Scan(&out.Counts.VerifiedDeployments, &out.Counts.AccessibleRepositories, &out.Counts.StaleRepositories, &out.Counts.QueuedJobs); err != nil {
			return err
		}
		rows, e := tx.Query(ctx, `SELECT f.id::text,f.repository_id::text,coalesce(r.name,''),f.title,f.severity,f.state,extract(epoch FROM now()-f.first_seen)::bigint,coalesce(f.assigned_to::text,'') FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.state='open' AND `+repo+` ORDER BY CASE f.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END,f.last_seen LIMIT 20`, org, a.AllRepositories, a.RepositoryIDs)
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
		portfolio, e := tx.Query(ctx, attention.WorkQuery+`SELECT r.id::text,r.name,r.provider,r.accessible,r.last_synced_at,
				(SELECT count(*) FROM maintenance_findings f WHERE f.org_id=r.org_id AND f.repository_id=r.id AND f.state='open'),
				(SELECT count(*) FROM inventory_changes c WHERE c.org_id=r.org_id AND c.repository_id=r.id AND c.snapshot->>'state' IN ('open','opened')),
				(SELECT count(*) FROM work w WHERE w.repository_id=r.id::text AND w.bucket='blocked')
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
				(SELECT count(*) FROM deployments WHERE org_id=$1 AND state IN ('healthy','recovered') AND first_healthy_at>=s AND first_healthy_at<s+interval '1 day' AND `+repo+`)+(SELECT count(*) FROM gitops_promotions WHERE org_id=$1 AND state IN ('healthy','recovered') AND first_healthy_at>=s AND first_healthy_at<s+interval '1 day' AND ($2 OR source_repository_id=ANY($3::uuid[])))
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
		progress, e := tx.Query(ctx, `WITH progress AS (
   SELECT f.repository_id,r.name,CASE
    WHEN t.state='queued' AND (t.reason ILIKE '%budget%' OR t.reason ILIKE '%provider limits%') THEN 'waiting_limit'
    WHEN t.state IN ('reproducing','planning','repairing','validating','publishing','reconciling','cancelling') THEN 'fixing'
    WHEN t.state='queued' THEN 'queued'
    WHEN EXISTS(SELECT 1 FROM repair_runs rr WHERE rr.org_id=f.org_id AND rr.finding_id=f.id AND rr.state='published' AND rr.native_change IS NOT NULL AND coalesce(rr.native_change->>'state','open') IN ('open','opened')
     AND NOT EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=rr.org_id AND m.repository_id=rr.repository_id AND m.change_id=rr.native_change->>'id' AND m.state='merged')
     AND NOT EXISTS(SELECT 1 FROM inventory_changes c WHERE c.org_id=rr.org_id AND c.repository_id=rr.repository_id AND c.snapshot->>'id'=rr.native_change->>'id' AND c.snapshot->>'state' IN ('closed','merged'))) THEN 'checking'
    WHEN a.outcome='blocked' OR a.blocker_kind IN ('platform','exhausted','unsupported','restore_runner') THEN 'waiting'
    ELSE 'scheduled' END AS state
   FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id
   LEFT JOIN LATERAL (SELECT a.* FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version ORDER BY a.updated_at DESC LIMIT 1) a ON true
   LEFT JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id
   WHERE f.org_id=$1 AND f.state='open' AND f.category<>'repository_review' AND ($2 OR f.repository_id=ANY($3::uuid[]))
  ) SELECT repository_id::text,name,state,count(*) FROM progress GROUP BY repository_id,name,state ORDER BY name,state`, org, a.AllRepositories, a.RepositoryIDs)
		if e != nil {
			return e
		}
		out.Maintenance, e = pgx.CollectRows(progress, pgx.RowToStructByPos[MaintenanceProgress])
		if e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id WHERE j.org_id=$1 AND j.state='queued' AND ($2 OR t.repository_id=ANY($3::uuid[]))),
			(SELECT count(*) FROM workflow_jobs j JOIN workflow_tasks t ON t.org_id=j.org_id AND t.id=j.task_id WHERE j.org_id=$1 AND j.state='running' AND ($2 OR t.repository_id=ANY($3::uuid[]))),
			(SELECT count(*) FROM runner_pools p WHERE p.org_id=$1 AND p.state='active' AND ($2 OR p.builtin OR EXISTS(SELECT 1 FROM runner_pool_repositories pr WHERE pr.org_id=p.org_id AND pr.pool_id=p.id AND pr.repository_id=ANY($3::uuid[])))),
			(SELECT count(*) FROM runners n JOIN runner_pools p ON p.org_id=n.org_id AND p.id=n.pool_id WHERE n.org_id=$1 AND n.state='active' AND p.state='active' AND n.credential_expires_at>now() AND n.last_seen_at>now()-interval '5 minutes' AND ($2 OR p.builtin OR EXISTS(SELECT 1 FROM runner_pool_repositories pr WHERE pr.org_id=p.org_id AND pr.pool_id=p.id AND pr.repository_id=ANY($3::uuid[])))),
			(SELECT COALESCE(sum((record->'maximum'->>'micro_usd')::bigint) FILTER (WHERE state IN ('reserved','dispatched','unknown')),0) FROM budget_reservations WHERE org_id=$1 AND ($2 OR repository_id=ANY($3::uuid[])))`, org, a.AllRepositories, a.RepositoryIDs).Scan(&out.Capacity.QueuedJobs, &out.Capacity.RunningJobs, &out.Capacity.ActivePools, &out.Capacity.ActiveRunners, &out.Capacity.ReservedMicro)
	})
	return out, err
}
