package attention

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

type Work struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Repository   string    `json:"repository"`
	Kind         string    `json:"kind"`
	State        string    `json:"state"`
	Bucket       string    `json:"bucket"`
	Title        string    `json:"title"`
	Reason       string    `json:"reason"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"created_at"`
}

const WorkQuery = `WITH work AS (
 SELECT 'run:'||t.id AS id,t.repository_id::text AS repository_id,r.name AS repository,'repair' AS kind,t.state,CASE WHEN t.state='blocked' THEN 'blocked' ELSE 'active' END AS bucket,coalesce(f.title,t.recipe) AS title,t.reason,'runs?run='||t.id AS url,t.created_at
 FROM workflow_tasks t JOIN repositories r ON r.org_id=t.org_id AND r.id=t.repository_id LEFT JOIN repair_runs rr ON rr.org_id=t.org_id AND rr.task_id=t.id LEFT JOIN maintenance_findings f ON f.org_id=rr.org_id AND f.id=rr.finding_id
 WHERE t.org_id=$1 AND ($2 OR t.repository_id=ANY($3::uuid[])) AND t.state IN ('queued','reproducing','planning','repairing','validating','publishing','reconciling','cancelling','blocked')
 UNION ALL
 SELECT 'merge:'||m.id,m.repository_id::text,r.name,'merge',m.state,CASE WHEN m.state='blocked' THEN 'blocked' ELSE 'active' END,'Merge fix #'||m.change_id,m.reason,'changes?repository='||m.repository_id||'&change='||m.change_id,m.created_at
 FROM merge_operations m JOIN repositories r ON r.org_id=m.org_id AND r.id=m.repository_id WHERE m.org_id=$1 AND ($2 OR m.repository_id=ANY($3::uuid[])) AND m.state IN ('requested','dispatching','queued','reconciling','blocked')
 UNION ALL
 SELECT 'deployment:'||d.id,d.repository_id::text,r.name,'native',d.state,CASE WHEN d.state='blocked' THEN 'blocked' ELSE 'active' END,'Deploy to '||d.environment,d.reason,'deployments?deployment='||d.id,d.created_at
 FROM deployments d JOIN repositories r ON r.org_id=d.org_id AND r.id=d.repository_id WHERE d.org_id=$1 AND ($2 OR d.repository_id=ANY($3::uuid[])) AND d.state IN ('requested','dispatching','awaiting_gates','running','verifying','reconciling','recovery_requested','recovering','completed_unverified','blocked')
 UNION ALL
 SELECT 'gitops:'||p.id,p.source_repository_id::text,r.name,'gitops',p.state,CASE WHEN p.state='blocked' THEN 'blocked' ELSE 'active' END,'Deliver to '||p.environment,p.reason,'deployments?promotion='||p.id,p.created_at
 FROM gitops_promotions p JOIN repositories r ON r.org_id=p.org_id AND r.id=p.source_repository_id WHERE p.org_id=$1 AND ($2 OR p.source_repository_id=ANY($3::uuid[])) AND p.state IN ('requested','staged','staging','stage_uncertain','publishing','publish_uncertain','awaiting_merge','verifying','completed_unverified','blocked')
 UNION ALL
 SELECT 'review:'||rr.task_id,rr.repository_id::text,r.name,'review',rr.state,'review',coalesce(f.title,'Review proposed fix'),coalesce(rr.report->>'reason',''),coalesce(nullif(rr.native_change->>'url',''),'runs?run='||rr.task_id),rr.created_at
 FROM repair_runs rr JOIN repositories r ON r.org_id=rr.org_id AND r.id=rr.repository_id LEFT JOIN maintenance_findings f ON f.org_id=rr.org_id AND f.id=rr.finding_id
 WHERE rr.org_id=$1 AND ($2 OR rr.repository_id=ANY($3::uuid[])) AND rr.state='published' AND rr.native_change IS NOT NULL AND coalesce(rr.native_change->>'state','open') IN ('open','opened')
 AND NOT EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=rr.org_id AND m.repository_id=rr.repository_id AND m.change_id=rr.native_change->>'id' AND m.state='merged')
 AND NOT EXISTS(SELECT 1 FROM inventory_changes c WHERE c.org_id=rr.org_id AND c.repository_id=rr.repository_id AND c.snapshot->>'id'=rr.native_change->>'id' AND c.snapshot->>'state' IN ('closed','merged'))
 UNION ALL
	 SELECT 'parked:'||coalesce(t.id::text,f.id::text||':'||f.version::text||':'||a.blocker_kind),f.repository_id::text,r.name,'repair','waiting','blocked',f.title,CASE a.blocker_kind WHEN 'platform' THEN CASE WHEN a.retry_after IS NULL THEN 'Waiting for runner or model service recovery' ELSE 'Retry scheduled after temporary runner or model service failure' END WHEN 'exhausted' THEN 'Reforge will try again when repository evidence or repair tools change' WHEN 'unsupported' THEN 'Waiting for repair tools that support this finding' ELSE 'Waiting for maintenance runner recovery' END,CASE WHEN t.id IS NOT NULL THEN 'runs?run='||t.id ELSE 'repositories?repository='||f.repository_id END,coalesce(t.created_at,a.updated_at)
 FROM autopilot_attempts a LEFT JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id
 WHERE a.org_id=$1 AND ($2 OR f.repository_id=ANY($3::uuid[])) AND f.state='open' AND a.outcome IN ('blocked','retry') AND a.blocker_kind IN ('platform','exhausted','unsupported','restore_runner') AND (t.id IS NULL OR t.state NOT IN ('queued','reproducing','planning','repairing','validating','publishing','reconciling','cancelling','blocked'))
 AND NOT EXISTS(SELECT 1 FROM repair_runs rr WHERE rr.org_id=a.org_id AND rr.task_id=a.task_id AND rr.native_change IS NOT NULL)
) `

func WorkCountsTx(ctx context.Context, tx pgx.Tx, a domain.Actor) (active, blocked, review int64, err error) {
	err = tx.QueryRow(ctx, WorkQuery+`SELECT count(*) FILTER(WHERE bucket='active'),count(*) FILTER(WHERE bucket='blocked'),count(*) FILTER(WHERE bucket='review') FROM work`, a.OrgID, a.AllRepositories, a.RepositoryIDs).Scan(&active, &blocked, &review)
	return
}

func WorkTx(ctx context.Context, tx pgx.Tx, a domain.Actor, bucket string, limit int, cursor string) (domain.Page[Work], error) {
	out := domain.Page[Work]{Items: []Work{}}
	if bucket != "active" && bucket != "blocked" && bucket != "review" || limit < 1 || limit > 100 || len(cursor) > 512 {
		return out, auth.ErrInvalid
	}
	var at *time.Time
	key := ""
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		var parts []string
		if err != nil || json.Unmarshal(raw, &parts) != nil || len(parts) != 2 || len(parts[1]) > 100 {
			return out, auth.ErrInvalid
		}
		parsed, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil || !strings.Contains(parts[1], ":") {
			return out, auth.ErrInvalid
		}
		at = &parsed
		key = parts[1]
	}
	rows, err := tx.Query(ctx, WorkQuery+`SELECT * FROM work WHERE bucket=$4 AND ($5::timestamptz IS NULL OR (created_at,id)<($5,$6)) ORDER BY created_at DESC,id DESC LIMIT $7`, a.OrgID, a.AllRepositories, a.RepositoryIDs, bucket, at, key, limit+1)
	if err != nil {
		return out, err
	}
	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Work])
	if err != nil {
		return out, err
	}
	out.Complete = len(out.Items) <= limit
	if !out.Complete {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		raw, _ := json.Marshal([]string{last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}
