package autopilot

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
)

type Need struct {
	Kind         string `json:"kind"`
	FindingID    string `json:"finding_id"`
	RepositoryID string `json:"repository_id"`
	Repository   string `json:"repository"`
	Title        string `json:"title"`
	Reason       string `json:"reason"`
	URL          string `json:"url,omitempty"`
	Label        string `json:"label,omitempty"`
}

func (s *Service) Needs(ctx context.Context, session auth.Session, org string) ([]Need, error) {
	out := []Need{}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT kind,finding_id,repository_id,repository,title,reason,url,label FROM (
SELECT 'grant' AS kind,f.id::text AS finding_id,f.repository_id::text AS repository_id,r.name AS repository,f.title,f.evidence->'blockers'->>0 AS reason,coalesce(f.evidence->>'action_url','') AS url,coalesce(f.evidence->>'action_label','') AS label,0 AS rank FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.state='open' AND f.evidence->'blockers'->>0 LIKE 'Needs a person%'
UNION ALL SELECT 'review',f.id::text,f.repository_id::text,r.name,f.title,a.merge_reason,coalesce((SELECT rr.native_change->>'url' FROM repair_runs rr WHERE rr.org_id=a.org_id AND rr.task_id=a.task_id),''),'Review pull request',1 FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE a.org_id=$1 AND f.state='open' AND a.merge_reason LIKE 'Awaiting human%'
UNION ALL SELECT 'blocked',f.id::text,f.repository_id::text,r.name,f.title,a.reason,'','',2 FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE a.org_id=$1 AND f.state='open' AND a.outcome='blocked'
) needs ORDER BY rank,repository,title LIMIT 100`, org)
		if err != nil {
			return err
		}
		items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Need])
		if err != nil {
			return err
		}
		for _, item := range items {
			if auth.CanReadRepository(a, item.RepositoryID) {
				out = append(out, item)
			}
		}
		return nil
	})
	return out, err
}
