package attention

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

type Need struct {
	ID             string     `json:"id"`
	Kind           string     `json:"kind"`
	FindingID      string     `json:"finding_id"`
	FindingVersion int64      `json:"finding_version"`
	RepositoryID   string     `json:"repository_id"`
	Repository     string     `json:"repository"`
	Title          string     `json:"title"`
	Reason         string     `json:"reason"`
	URL            string     `json:"url,omitempty"`
	Label          string     `json:"label,omitempty"`
	Owner          string     `json:"owner"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty"`
}

type Page struct {
	domain.Page[Need]
	Total int64 `json:"total"`
}

const Query = `WITH managed AS (
 SELECT r.*,coalesce(rp.document#>>'{defaults,model}',op.document#>>'{defaults,model}','') AS model,
 coalesce(rp.document#>>'{defaults,route}',op.document#>>'{defaults,route}','') AS route
 FROM repositories r
 LEFT JOIN policy_bindings rb ON rb.org_id=r.org_id AND rb.scope_kind='repository' AND rb.scope_id=r.id
 LEFT JOIN policy_versions rp ON rp.org_id=rb.org_id AND rp.id=rb.policy_version_id
 LEFT JOIN policy_bindings ob ON ob.org_id=r.org_id AND ob.scope_kind='organisation'
 LEFT JOIN policy_versions op ON op.org_id=ob.org_id AND op.id=ob.policy_version_id
 WHERE r.org_id=$1 AND ($2 OR r.id=ANY($3::uuid[])) AND r.accessible AND NOT r.archived
 AND (NOT coalesce((op.document->>'repository_setup_required')::boolean,false) OR rb.policy_version_id IS NOT NULL)
 AND NOT coalesce(rp.document->'deny' ? 'repair',false) AND NOT coalesce(op.document->'deny' ? 'repair',false)
 AND (EXISTS(SELECT 1 FROM autopilot_requests q WHERE q.org_id=r.org_id AND q.repository_id=r.id AND (q.continuous OR q.requested_at>now()-interval '1 day')) OR EXISTS(SELECT 1 FROM autopilot_settings s WHERE s.org_id=r.org_id AND s.enabled))
 AND EXISTS(SELECT 1 FROM maintenance_findings f WHERE f.org_id=r.org_id AND f.repository_id=r.id AND f.state='open')
), limits AS (
 SELECT l.*,coalesce((l.held->>'micro_usd')::bigint,0)+coalesce((s.amount->>'micro_usd')::bigint,0) AS used
 FROM budget_limits l LEFT JOIN budget_spend s ON s.org_id=l.org_id AND s.scope_kind=l.scope_kind AND s.scope_id=l.scope_id AND s.period_start=CASE l.period WHEN 'daily' THEN date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' WHEN 'monthly' THEN date_trunc('month',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' ELSE l.period_start END
 WHERE l.org_id=$1
), budget_scopes AS (
 SELECT r.id AS repository_id,l.* FROM managed r LEFT JOIN limits l ON l.org_id=r.org_id AND l.scope_kind='organisation'
 UNION ALL SELECT r.id,l.* FROM managed r JOIN limits l ON l.org_id=r.org_id AND l.scope_kind='repository' AND l.scope_id=r.id
), needs AS (
 SELECT 'grant:'||f.id AS id,'grant' AS kind,f.id::text AS finding_id,f.version AS finding_version,f.repository_id::text AS repository_id,r.name AS repository,f.title,coalesce(f.evidence->'blockers'->>0,'Access requires attention') AS reason,coalesce(f.evidence->>'action_url','') AS url,coalesce(f.evidence->>'action_label','Grant access') AS label,'owner' AS owner,NULL::timestamptz AS next_retry_at
 FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND ($2 OR f.repository_id=ANY($3::uuid[])) AND f.state='open' AND f.category='provider_access' AND jsonb_array_length(coalesce(f.evidence->'blockers','[]'))>0
 UNION ALL
 SELECT 'attempt:'||f.id,a.blocker_kind,f.id::text,f.version,f.repository_id::text,r.name,f.title,a.reason,CASE a.blocker_kind WHEN 'restore_runner' THEN 'runners' WHEN 'raise_limit' THEN 'usage?scope_kind=repository&scope_id='||f.repository_id WHEN 'reconnect' THEN 'repositories?repository='||f.repository_id WHEN 'approve' THEN 'repositories?repository='||f.repository_id ELSE 'findings?finding='||f.id END,CASE a.blocker_kind WHEN 'exhausted' THEN 'Review evidence' WHEN 'restore_runner' THEN 'Restore runner' WHEN 'raise_limit' THEN 'Adjust spending limit' WHEN 'reconnect' THEN 'Reconnect' ELSE 'Review blocker' END,CASE WHEN a.blocker_kind IN ('restore_runner','unsupported') THEN 'operator' ELSE 'owner' END,a.retry_after
 FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id
 WHERE a.org_id=$1 AND ($2 OR f.repository_id=ANY($3::uuid[])) AND f.state='open' AND a.blocker_kind<>'' AND a.outcome IN ('blocked','retry')
 AND NOT EXISTS(SELECT 1 FROM workflow_tasks t WHERE t.org_id=a.org_id AND t.id=a.task_id AND t.state IN ('queued','reproducing','planning','repairing','validating','publishing','reconciling'))
 AND (a.blocker_kind NOT IN ('restore_runner','reconnect','raise_limit') OR a.retry_after>now())
 UNION ALL
 SELECT 'review:'||f.id,'review',f.id::text,f.version,f.repository_id::text,r.name,f.title,a.merge_reason,coalesce(rr.native_change->>'url',''),'Review pull request','reviewer',a.merge_after
 FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id JOIN repair_runs rr ON rr.org_id=a.org_id AND rr.task_id=a.task_id
 WHERE a.org_id=$1 AND ($2 OR f.repository_id=ANY($3::uuid[])) AND f.state='open' AND a.merge_needs_human AND coalesce(rr.native_change->>'state','open') IN ('open','opened')
 AND NOT EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=rr.org_id AND m.repository_id=rr.repository_id AND m.change_id=rr.native_change->>'id' AND m.state='merged')
 AND NOT EXISTS(SELECT 1 FROM inventory_changes c WHERE c.org_id=rr.org_id AND c.repository_id=rr.repository_id AND c.snapshot->>'id'=rr.native_change->>'id' AND c.snapshot->>'state' IN ('closed','merged'))
 UNION ALL
 SELECT 'bot:'||b.repository_id||':'||b.change_id,'review','',0,b.repository_id::text,r.name,coalesce(c.snapshot->>'title','Review dependency update'),b.reason,coalesce(c.snapshot->>'url',''),'Review pull request','reviewer',b.merge_after
 FROM autopilot_bot_merges b JOIN inventory_changes c ON c.org_id=b.org_id AND c.repository_id=b.repository_id AND c.snapshot->>'id'=b.change_id JOIN repositories r ON r.org_id=b.org_id AND r.id=b.repository_id
 WHERE b.org_id=$1 AND ($2 OR b.repository_id=ANY($3::uuid[])) AND b.needs_human AND c.snapshot->>'state' IN ('open','opened')
 AND NOT EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=b.org_id AND m.repository_id=b.repository_id AND m.change_id=b.change_id AND m.state='merged')
 UNION ALL
 SELECT 'host:'||r.id,'reconnect','',0,r.id::text,r.name,'Reconnect code host','Code host access is unavailable','connections?connection='||r.connection_id,'Reconnect','operator',NULL FROM managed r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE c.state<>'healthy'
 UNION ALL
 SELECT 'budget:'||r.id||':'||coalesce(l.scope_kind,'organisation'),'raise_limit','',0,r.id::text,r.name,'Adjust maintenance spending limit',CASE WHEN l.paused THEN 'Spending paused' WHEN l.caps->>'micro_usd' IS NULL THEN 'Dollar ceiling missing' ELSE 'Spending allowance used or outside its budget period' END,'usage?scope_kind='||coalesce(l.scope_kind,'organisation')||'&scope_id='||coalesce(l.scope_id,r.org_id),'Adjust spending limit','owner',NULL
 FROM managed r JOIN budget_scopes l ON l.repository_id=r.id
 WHERE l.org_id IS NULL OR l.paused OR l.scope_kind='organisation' AND l.caps->>'micro_usd' IS NULL OR l.used>=(l.caps->>'micro_usd')::bigint OR l.period='custom' AND (now()<l.period_start OR now()>=l.period_end)
 UNION ALL
 SELECT 'runner:'||r.id,'restore_runner','',0,r.id::text,r.name,'Restore maintenance runner','No live runner can execute this repository','runners','Restore runner','operator',NULL FROM managed r
 WHERE NOT EXISTS(SELECT 1 FROM runner_pools p JOIN runners n ON n.org_id=p.org_id AND n.pool_id=p.id WHERE p.org_id=r.org_id AND p.state='active' AND n.state='active' AND n.credential_expires_at>now() AND n.last_seen_at>now()-interval '5 minutes' AND (p.builtin OR EXISTS(SELECT 1 FROM runner_pool_repositories pr WHERE pr.org_id=p.org_id AND pr.pool_id=p.id AND pr.repository_id=r.id)))
 UNION ALL
 SELECT 'model:'||r.id,'reconnect','',0,r.id::text,r.name,'Connect approved maintenance model','Healthy approved model with known pricing unavailable','repositories?repository='||r.id,'Set up maintenance','operator',NULL FROM managed r
 WHERE NOT EXISTS(SELECT 1 FROM connections c JOIN budget_routes b ON b.org_id=c.org_id AND b.connection_id=c.id AND b.model=c.settings->>'model' WHERE c.org_id=r.org_id AND c.kind='model' AND c.state='healthy' AND b.config->>'mode'='priced' AND NOT coalesce((b.config->>'paused')::boolean,false) AND (r.model='' OR r.model=c.settings->>'model' AND r.route=b.name))
) `

func CountTx(ctx context.Context, tx pgx.Tx, a domain.Actor) (int64, error) {
	var count int64
	err := tx.QueryRow(ctx, Query+`SELECT count(*) FROM needs`, a.OrgID, a.AllRepositories, a.RepositoryIDs).Scan(&count)
	return count, err
}

func ListTx(ctx context.Context, tx pgx.Tx, a domain.Actor, limit int, cursor string) (Page, error) {
	out := Page{Page: domain.Page[Need]{Items: []Need{}}}
	if limit < 1 || limit > 100 || len(cursor) > 512 {
		return out, auth.ErrInvalid
	}
	key := ""
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(raw) > 300 || !strings.Contains(string(raw), ":") {
			return out, auth.ErrInvalid
		}
		key = string(raw)
	}
	rows, err := tx.Query(ctx, Query+`SELECT * FROM needs WHERE id>$4 ORDER BY id LIMIT $5`, a.OrgID, a.AllRepositories, a.RepositoryIDs, key, limit+1)
	if err != nil {
		return out, err
	}
	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Need])
	if err != nil {
		return out, err
	}
	out.Complete = len(out.Items) <= limit
	if !out.Complete {
		out.Items = out.Items[:limit]
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(out.Items[limit-1].ID))
	}
	out.Total, err = CountTx(ctx, tx, a)
	return out, err
}
