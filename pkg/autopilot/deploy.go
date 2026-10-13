package autopilot

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/delivery"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/gitops"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/release"
)

func (s *Service) observeDeployments(ctx context.Context, session auth.Session, org string) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT ON (d.repository_id,d.environment) d.id::text,d.repository_id::text,d.environment,d.reason,g.document,r.default_branch FROM deployments d JOIN deployment_gates g ON g.org_id=d.org_id AND g.id=d.gate_id JOIN repositories r ON r.org_id=d.org_id AND r.id=d.repository_id WHERE d.org_id=$1 AND ($2::uuid[] IS NULL OR d.repository_id=ANY($2)) AND d.state IN ('failed','recovery_failed') AND NOT EXISTS(SELECT 1 FROM deployments later WHERE later.org_id=d.org_id AND later.repository_id=d.repository_id AND later.environment=d.environment AND later.state IN ('healthy','recovered') AND later.created_at>d.created_at) ORDER BY d.repository_id,d.environment,d.created_at DESC`, org, session.AutomationRepositories())
		if err != nil {
			return err
		}
		observations := []discovery.Observation{}
		for rows.Next() {
			var id, repo, env, reason, branch string
			var raw []byte
			if err = rows.Scan(&id, &repo, &env, &reason, &raw, &branch); err != nil {
				rows.Close()
				return err
			}
			var gate deployment.Gate
			if json.Unmarshal(raw, &gate) != nil {
				continue
			}
			evidence := discovery.Evidence{Provenance: "deployment " + id, ConnectionID: gate.ConnectionID, ConnectionVersion: gate.ConnectionVersion, HeadSHA: gate.Request.SourceSHA, TargetSHA: gate.Request.SourceSHA, TargetBranch: branch, Deployment: id, Complete: true, Review: &discovery.ReviewEvidence{Confidence: "high", Objective: "Restore " + env + ": roll back to the last healthy deployment, or fix the cause and redeploy", Detail: reason}}
			observations = append(observations, discovery.Observation{RepositoryID: repo, Source: "deployment", SourceID: "deploy:" + env, Category: "deploy_failure", Severity: "high", Title: "Deployment to " + env + " failed", Evidence: evidence})
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, o := range observations {
			_, err := discovery.ObserveTx(ctx, tx, org, o, session.User.ID, "autopilot-deployments")
			if err != nil {
				return err
			}
		}
		gitopsRows, err := tx.Query(ctx, `SELECT DISTINCT ON (p.source_repository_id,p.environment) p.id::text,p.source_repository_id::text,p.environment,p.reason,g.document,r.default_branch FROM gitops_promotions p JOIN gitops_gates g ON g.org_id=p.org_id AND g.id=p.gate_id JOIN repositories r ON r.org_id=p.org_id AND r.id=p.source_repository_id WHERE p.org_id=$1 AND ($2::uuid[] IS NULL OR p.source_repository_id=ANY($2)) AND p.state IN ('failed','recovery_failed') AND NOT EXISTS(SELECT 1 FROM gitops_promotions newer WHERE newer.org_id=p.org_id AND newer.source_repository_id=p.source_repository_id AND newer.environment=p.environment AND newer.state IN ('healthy','recovered') AND (newer.created_at,newer.id)>(p.created_at,p.id)) ORDER BY p.source_repository_id,p.environment,p.created_at DESC,p.id DESC LIMIT 100`, org, session.AutomationRepositories())
		if err != nil {
			return err
		}
		var failedPromotions []discovery.Observation
		for gitopsRows.Next() {
			var id, repo, env, reason, branch string
			var raw []byte
			if err = gitopsRows.Scan(&id, &repo, &env, &reason, &raw, &branch); err != nil {
				gitopsRows.Close()
				return err
			}
			var gate gitops.Gate
			if err = json.Unmarshal(raw, &gate); err != nil {
				gitopsRows.Close()
				return err
			}
			evidence := discovery.Evidence{Provenance: "GitOps promotion " + id, ConnectionID: gate.SourceConnectionID, ConnectionVersion: gate.SourceConnectionVersion, HeadSHA: gate.Request.SourceSHA, TargetSHA: gate.Request.SourceSHA, TargetBranch: branch, Deployment: id, Complete: true, ActionURL: "gitops?promotion=" + id, ActionLabel: "Restore previous working version", Review: &discovery.ReviewEvidence{Confidence: "high", Objective: "Restore " + env, Detail: reason}}
			failedPromotions = append(failedPromotions, discovery.Observation{RepositoryID: repo, Source: "gitops_deployment", SourceID: "gitops:" + env, Category: "deploy_failure", Severity: "high", Title: "Deployment to " + env + " failed", Evidence: evidence})
		}
		err = gitopsRows.Err()
		gitopsRows.Close()
		if err != nil {
			return err
		}
		for _, o := range failedPromotions {
			if _, err = discovery.ObserveTx(ctx, tx, org, o, session.User.ID, "autopilot-gitops-deployments"); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE maintenance_findings f SET state='resolved',reason='Environment healthy again: verified GitOps release',version=f.version+1 FROM gitops_promotions failed WHERE f.org_id=$1 AND ($2::uuid[] IS NULL OR f.repository_id=ANY($2)) AND f.source='gitops_deployment' AND f.state='open' AND failed.org_id=f.org_id AND failed.source_repository_id=f.repository_id AND failed.id::text=f.evidence->>'deployment' AND EXISTS(SELECT 1 FROM gitops_promotions recovery WHERE recovery.org_id=failed.org_id AND recovery.source_repository_id=failed.source_repository_id AND recovery.environment=failed.environment AND recovery.state IN ('healthy','recovered') AND (recovery.created_at,recovery.id)>(failed.created_at,failed.id) AND NOT EXISTS(SELECT 1 FROM gitops_promotions later WHERE later.org_id=recovery.org_id AND later.source_repository_id=recovery.source_repository_id AND later.environment=recovery.environment AND later.state IN ('failed','recovery_failed') AND (later.created_at,later.id)>(recovery.created_at,recovery.id)))`, org, session.AutomationRepositories()); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `WITH recovered AS (
SELECT f.id,recovery.id AS deployment_id FROM maintenance_findings f
JOIN deployments failed ON failed.org_id=f.org_id AND failed.id::text=f.evidence->>'deployment' AND failed.repository_id=f.repository_id AND f.source_id='deploy:'||failed.environment
JOIN LATERAL (SELECT d.id FROM deployments d WHERE d.org_id=f.org_id AND d.repository_id=f.repository_id AND d.environment=failed.environment AND d.state IN ('healthy','recovered') AND d.created_at>failed.created_at AND NOT EXISTS(SELECT 1 FROM deployments newer WHERE newer.org_id=d.org_id AND newer.repository_id=d.repository_id AND newer.environment=d.environment AND newer.state IN ('failed','recovery_failed') AND newer.created_at>d.created_at) ORDER BY d.created_at DESC LIMIT 1) recovery ON true
WHERE f.org_id=$1 AND ($2::uuid[] IS NULL OR f.repository_id=ANY($2)) AND f.source='deployment' AND f.state='open'
) UPDATE maintenance_findings f SET state='resolved',reason='Environment healthy again: deployment '||recovered.deployment_id::text,version=f.version+1 FROM recovered WHERE f.org_id=$1 AND f.id=recovered.id`, org, session.AutomationRepositories())
		return err
	})
}

func (s *Service) rollback(ctx context.Context, session auth.Session, org string, c candidate) (string, error) {
	var id, kind string
	var version int64
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT d.id::text,d.kind,d.version FROM maintenance_findings f JOIN LATERAL (SELECT id,version,'native' AS kind FROM deployments WHERE org_id=f.org_id AND repository_id=f.repository_id AND id::text=f.evidence->>'deployment' UNION ALL SELECT id,version,'gitops' FROM gitops_promotions WHERE org_id=f.org_id AND source_repository_id=f.repository_id AND id::text=f.evidence->>'deployment') d ON true WHERE f.org_id=$1 AND f.id=$2`, org, c.finding).Scan(&id, &kind, &version)
	})
	if err != nil {
		return "Rollback unavailable: release evidence could not be resolved", err
	}
	commands := s.Delivery
	if commands == nil {
		commands = &delivery.Service{Releases: &release.Service{DB: s.db, Auth: s.auth}, Native: s.Deployments, GitOps: s.Promotions}
	}
	result, err := commands.Restore(ctx, session, org, kind, id, version, "autopilot-recovery")
	if err != nil {
		return "Rollback unavailable: " + err.Error(), nil
	}
	return "", s.record(ctx, org, c, "", "retry", "Recovery requested: "+result.ID, 30*time.Minute)
}

func (s *Service) promote(ctx context.Context, session auth.Session, org string) error {
	if s.Promotions == nil {
		return nil
	}
	type pending struct {
		env     string
		request gitops.PreviewRequest
	}
	var work []pending
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.environment,g.document->'request' FROM gitops_configurations c CROSS JOIN LATERAL (SELECT gg.document FROM gitops_promotions p JOIN gitops_gates gg ON gg.org_id=p.org_id AND gg.id=p.gate_id WHERE p.org_id=c.org_id AND p.environment=c.document->>'promote_from' AND p.source_repository_id=c.source_repository_id AND p.state IN ('healthy','recovered') ORDER BY p.created_at DESC LIMIT 1) g WHERE c.org_id=$1 AND ($2::uuid[] IS NULL OR c.source_repository_id=ANY($2)) AND coalesce(c.document->>'promote_from','')<>'' AND coalesce((c.document->>'enabled')::boolean,false) AND NOT EXISTS(SELECT 1 FROM gitops_promotions t JOIN gitops_gates tg ON tg.org_id=t.org_id AND tg.id=t.gate_id WHERE t.org_id=c.org_id AND t.environment=c.environment AND (t.finished_at IS NULL OR tg.document->'request'->>'artifact_digest'=g.document->'request'->>'artifact_digest' AND t.state NOT IN ('failed','recovery_failed','cancelled','blocked')))`, org, session.AutomationRepositories())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p pending
			var raw []byte
			if err = rows.Scan(&p.env, &raw); err != nil {
				return err
			}
			if json.Unmarshal(raw, &p.request) == nil {
				p.request.RecoveryOf, p.request.RestorePromotionID = "", ""
				work = append(work, p)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, p := range work {
		gate, err := s.Promotions.Preview(ctx, session, org, p.env, p.request, domain.StableID("autopilot-promote", p.env, p.request.ArtifactDigest))
		if err != nil || len(gate.Blockers) > 0 {
			reason := ""
			if err != nil {
				reason = err.Error()
			} else {
				reason = gate.Blockers[0]
			}
			slog.InfoContext(ctx, "autopilot promotion held", "org_id", org, "environment", p.env, "reason", reason)
			continue
		}
		if _, err = s.Promotions.Request(ctx, session, org, gate.ID, domain.StableID("autopilot-promote-request", p.env, gate.ID), "autopilot"); err != nil {
			slog.WarnContext(ctx, "autopilot promotion request failed", "org_id", org, "environment", p.env, "error", err)
		}
	}
	return nil
}
