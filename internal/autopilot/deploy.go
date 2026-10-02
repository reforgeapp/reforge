package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/auth"
	"reforge/internal/deployment"
	"reforge/internal/domain"
	"reforge/internal/maintenance/discovery"
)

func (s *Service) observeDeployments(ctx context.Context, session auth.Session, org string) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT ON (d.repository_id,d.environment) d.id::text,d.repository_id::text,d.environment,d.reason,g.document,r.default_branch FROM deployments d JOIN deployment_gates g ON g.org_id=d.org_id AND g.id=d.gate_id JOIN repositories r ON r.org_id=d.org_id AND r.id=d.repository_id WHERE d.org_id=$1 AND d.state IN ('failed','recovery_failed') AND d.finished_at>clock_timestamp()-interval '7 days' AND NOT EXISTS(SELECT 1 FROM deployments later WHERE later.org_id=d.org_id AND later.repository_id=d.repository_id AND later.environment=d.environment AND later.state IN ('healthy','recovered') AND later.created_at>d.created_at) ORDER BY d.repository_id,d.environment,d.created_at DESC`, org)
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
		seen := []string{}
		for _, o := range observations {
			f, err := discovery.ObserveTx(ctx, tx, org, o, session.User.ID, "autopilot-deployments")
			if err != nil {
				return err
			}
			seen = append(seen, f.ID)
		}
		_, err = tx.Exec(ctx, `UPDATE maintenance_findings SET state='resolved',reason='Environment healthy again',version=version+1 WHERE org_id=$1 AND source='deployment' AND state='open' AND NOT(id=ANY($2::uuid[]))`, org, seen)
		return err
	})
}

func (s *Service) rollback(ctx context.Context, session auth.Session, org string, c candidate) (string, error) {
	if s.Deployments == nil {
		return "Rollback unavailable: deployments are not configured", nil
	}
	var failed, env, known string
	var raw []byte
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT f.evidence->>'deployment',d.environment FROM maintenance_findings f JOIN deployments d ON d.org_id=f.org_id AND d.id=(f.evidence->>'deployment')::uuid WHERE f.org_id=$1 AND f.id=$2`, org, c.finding).Scan(&failed, &env); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT d.id::text,g.document FROM deployments d JOIN deployment_gates g ON g.org_id=d.org_id AND g.id=d.gate_id WHERE d.org_id=$1 AND d.repository_id=$2 AND d.environment=$3 AND d.state IN ('healthy','recovered') ORDER BY d.created_at DESC LIMIT 1`, org, c.repository, env).Scan(&known, &raw)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "Rollback unavailable: no earlier healthy deployment in this environment", nil
	}
	if err != nil {
		return "", err
	}
	var gate deployment.Gate
	if err = json.Unmarshal(raw, &gate); err != nil {
		return "", err
	}
	request := gate.Request
	request.RecoveryOf, request.RestoreDeploymentID = failed, known
	preview, err := s.Deployments.Preview(ctx, session, org, env, request, domain.StableID("autopilot-rollback", failed))
	if err != nil {
		return "Rollback unavailable: " + err.Error(), nil
	}
	if len(preview.Blockers) > 0 {
		return "Rollback unavailable: " + preview.Blockers[0], nil
	}
	if _, err = s.Deployments.Request(ctx, session, org, preview.ID, domain.StableID("autopilot-rollback-request", failed, preview.ID), "autopilot"); err != nil {
		return "Rollback unavailable: " + err.Error(), nil
	}
	return "", s.record(ctx, org, c, "", "retry", "Rollback requested to deployment "+known, 30*time.Minute)
}
