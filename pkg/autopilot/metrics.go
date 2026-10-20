package autopilot

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

type Metric struct {
	RepositoryID     string   `json:"repository_id"`
	Repository       string   `json:"repository"`
	Class            string   `json:"class"`
	Open             int      `json:"open"`
	Blocked          int      `json:"blocked"`
	Fixed            int      `json:"fixed"`
	MedianHoursToFix *float64 `json:"median_hours_to_fix,omitempty"`
	AutoMerged       int      `json:"auto_merged"`
	Regressed        int      `json:"regressed"`
	Reverted         int      `json:"reverted"`
}

func (s *Service) Metrics(ctx context.Context, session auth.Session, org string) ([]Metric, error) {
	out := []Metric{}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT r.id::text,r.name,f.category,
count(DISTINCT f.id) FILTER (WHERE f.state='open'),
count(DISTINCT f.id) FILTER (WHERE f.state='open' AND (jsonb_array_length(coalesce(f.evidence->'blockers','[]'))>0 OR EXISTS(SELECT 1 FROM autopilot_attempts a WHERE a.org_id=f.org_id AND a.finding_id=f.id AND a.finding_version=f.version AND a.outcome='blocked'))),
count(DISTINCT rr.task_id) FILTER (WHERE rr.native_change->>'state'='merged'),
percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM rr.post_merge_since-f.first_seen)/3600) FILTER (WHERE rr.post_merge_since IS NOT NULL),
count(DISTINCT rr.task_id) FILTER (WHERE rr.native_change->>'state'='merged' AND EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=rr.org_id AND m.repository_id=rr.repository_id AND m.change_id=rr.native_change->>'id' AND m.state='merged')),
count(DISTINCT rr.task_id) FILTER (WHERE rr.post_merge_state='regressed'),
count(DISTINCT rr.task_id) FILTER (WHERE rr.native_change->>'state'='merged' AND rr.report->>'reason' ILIKE '%revert%')
FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id LEFT JOIN repair_runs rr ON rr.org_id=f.org_id AND rr.finding_id=f.id
WHERE f.org_id=$1 AND f.first_seen>clock_timestamp()-interval '90 days' AND f.category<>'repository_review'
GROUP BY r.id,r.name,f.category ORDER BY r.name,f.category LIMIT 1000`, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m Metric
			if err = rows.Scan(&m.RepositoryID, &m.Repository, &m.Class, &m.Open, &m.Blocked, &m.Fixed, &m.MedianHoursToFix, &m.AutoMerged, &m.Regressed, &m.Reverted); err != nil {
				return err
			}
			if auth.CanReadRepository(a, m.RepositoryID) {
				out = append(out, m)
			}
		}
		return rows.Err()
	})
	return out, err
}
