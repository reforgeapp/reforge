package autopilot

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) fingerprintTx(ctx context.Context, tx pgx.Tx, org, finding string, version int64) (string, error) {
	var fingerprint string
	err := tx.QueryRow(ctx, `SELECT md5(jsonb_build_array($3::text,f.evidence_digest,
	 coalesce((SELECT jsonb_agg(jsonb_build_array(b.scope_kind,b.scope_id::text,v.policy_hash) ORDER BY b.scope_kind,b.scope_id) FROM policy_bindings b JOIN policy_versions v ON v.org_id=b.org_id AND v.id=b.policy_version_id WHERE b.org_id=f.org_id AND (b.scope_kind='organisation' OR b.scope_kind='repository' AND b.scope_id=r.id OR b.scope_kind='team' AND EXISTS(SELECT 1 FROM team_repositories tr WHERE tr.org_id=b.org_id AND tr.team_id=b.scope_id AND tr.repository_id=r.id))),'[]'::jsonb),
	 coalesce((SELECT jsonb_agg(jsonb_build_array(c.id::text,c.kind,c.provider,c.endpoint,c.settings,c.state,c.credential_version) ORDER BY c.id) FROM connections c WHERE c.org_id=f.org_id AND (c.id=r.connection_id OR c.kind IN ('model','agent'))),'[]'::jsonb),
	 coalesce((SELECT jsonb_agg(jsonb_build_array(b.connection_id::text,b.model,b.name,b.config) ORDER BY b.connection_id,b.model,b.name) FROM budget_routes b WHERE b.org_id=f.org_id),'[]'::jsonb),
	 coalesce((SELECT jsonb_agg(jsonb_build_array(l.scope_kind,l.scope_id::text,l.caps,l.paused,l.period,CASE WHEN l.period='custom' THEN l.period_start END,CASE WHEN l.period='custom' THEN l.period_end END) ORDER BY l.scope_kind,l.scope_id) FROM budget_limits l WHERE l.org_id=f.org_id AND (l.scope_kind='organisation' OR l.scope_kind='repository' AND l.scope_id=r.id OR l.scope_kind='team' AND EXISTS(SELECT 1 FROM team_repositories tr WHERE tr.org_id=l.org_id AND tr.team_id=l.scope_id AND tr.repository_id=r.id) OR l.scope_kind='connection' AND (l.scope_id=r.connection_id OR EXISTS(SELECT 1 FROM connections c WHERE c.org_id=l.org_id AND c.id=l.scope_id AND c.kind IN ('model','agent'))))),'[]'::jsonb),
	 coalesce((SELECT jsonb_agg(jsonb_build_array(p.id::text,p.state,p.builtin,coalesce((SELECT jsonb_agg(jsonb_build_array(n.id::text,n.state,n.credential_version,n.credential_expires_at>clock_timestamp(),n.last_seen_at>clock_timestamp()-interval '5 minutes') ORDER BY n.id) FROM runners n WHERE n.org_id=p.org_id AND n.pool_id=p.id),'[]'::jsonb)) ORDER BY p.id) FROM runner_pools p WHERE p.org_id=f.org_id AND (p.builtin OR EXISTS(SELECT 1 FROM runner_pool_repositories pr WHERE pr.org_id=p.org_id AND pr.pool_id=p.id AND pr.repository_id=r.id))),'[]'::jsonb)
	)::text) FROM maintenance_findings f JOIN repositories r ON r.org_id=f.org_id AND r.id=f.repository_id WHERE f.org_id=$1 AND f.id=$2 AND f.version=$4`, org, finding, s.capability(), version).Scan(&fingerprint)
	return fingerprint, err
}

func (s *Service) updatePlatformFailuresTx(ctx context.Context, tx pgx.Tx, org string) error {
	rows, err := tx.Query(ctx, `SELECT a.finding_id::text,a.finding_version,a.task_id::text,a.platform_retries FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version AND f.state='open' AND f.last_seen>clock_timestamp()-interval '1 hour' JOIN workflow_tasks t ON t.org_id=a.org_id AND t.id=a.task_id JOIN workflow_events e ON e.org_id=t.org_id AND e.aggregate_id=t.id AND e.type='task.failed' AND e.data->>'platform_failure'='true' WHERE a.org_id=$1 AND a.outcome='queued' AND t.state='failed' ORDER BY e.id DESC`, org)
	if err != nil {
		return err
	}
	type failed struct {
		finding, task string
		version       int64
		platforms     int
	}
	var failures []failed
	for rows.Next() {
		var f failed
		if err = rows.Scan(&f.finding, &f.version, &f.task, &f.platforms); err != nil {
			rows.Close()
			return err
		}
		failures = append(failures, f)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, f := range failures {
		fingerprint, err := s.fingerprintTx(ctx, tx, org, f.finding, f.version)
		if err != nil {
			return err
		}
		retryAfter := time.Duration(0)
		reason := "Waiting for runner or model service recovery"
		if f.platforms == 0 {
			retryAfter = 5 * time.Minute
			reason = "Runner or provider unavailable; retrying after cooldown"
		} else if f.platforms == 1 {
			retryAfter = 15 * time.Minute
			reason = "Runner or provider unavailable again; retrying after cooldown"
		}
		_, err = tx.Exec(ctx, `UPDATE autopilot_attempts SET outcome='blocked',blocker_kind='platform',reason=$5,retry_after=CASE WHEN $6::bigint>0 THEN clock_timestamp()+make_interval(secs=>$6::bigint) END,platform_retries=LEAST(2,platform_retries+1),max_runs=LEAST(100,max_runs+1),capability=$7,updated_at=clock_timestamp() WHERE org_id=$1 AND finding_id=$2 AND finding_version=$3 AND task_id=$4::uuid AND outcome='queued'`, org, f.finding, f.version, f.task, reason, int64(retryAfter.Seconds()), fingerprint)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) releaseChangedAttemptsTx(ctx context.Context, tx pgx.Tx, org string) error {
	rows, err := tx.Query(ctx, `SELECT a.finding_id::text,a.finding_version,a.capability,a.runs,a.max_runs FROM autopilot_attempts a JOIN maintenance_findings f ON f.org_id=a.org_id AND f.id=a.finding_id AND f.version=a.finding_version WHERE a.org_id=$1 AND f.state='open' AND f.last_seen>clock_timestamp()-interval '1 hour' AND a.outcome='blocked' AND a.blocker_kind IN ('platform','exhausted','unsupported','restore_runner','approve','reconnect','raise_limit') ORDER BY a.updated_at,a.finding_id`, org)
	if err != nil {
		return err
	}
	type blocked struct {
		finding, capability string
		version             int64
		runs, maxRuns       int
	}
	var attempts []blocked
	for rows.Next() {
		var a blocked
		if err = rows.Scan(&a.finding, &a.version, &a.capability, &a.runs, &a.maxRuns); err != nil {
			rows.Close()
			return err
		}
		attempts = append(attempts, a)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, a := range attempts {
		fingerprint, err := s.fingerprintTx(ctx, tx, org, a.finding, a.version)
		if err != nil {
			return err
		}
		if fingerprint == a.capability {
			continue
		}
		if a.runs >= a.maxRuns && a.maxRuns >= 100 {
			_, err = tx.Exec(ctx, `UPDATE autopilot_attempts SET capability=$4,platform_retries=0,reason='No verified fix yet; waiting for new evidence or repair tools',updated_at=clock_timestamp() WHERE org_id=$1 AND finding_id=$2 AND finding_version=$3 AND capability=$5 AND outcome='blocked'`, org, a.finding, a.version, fingerprint, a.capability)
		} else {
			_, err = tx.Exec(ctx, `UPDATE autopilot_attempts SET outcome='retry',blocker_kind='',reason='Repair tools or settings changed; one bounded attempt is available',retry_after=clock_timestamp(),platform_retries=0,max_runs=CASE WHEN runs>=max_runs THEN LEAST(100,max_runs+1) ELSE max_runs END,capability=$4,updated_at=clock_timestamp() WHERE org_id=$1 AND finding_id=$2 AND finding_version=$3 AND capability=$5 AND outcome='blocked'`, org, a.finding, a.version, fingerprint, a.capability)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
