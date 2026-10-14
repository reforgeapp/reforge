package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/workflow"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
)

func (s *Service) deliver(ctx context.Context, session auth.Session, org string) error {
	if s.Delivery == nil {
		return nil
	}
	var ids []string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM (SELECT l.id::text,(l.document#>>'{document,issued_at}')::timestamptz AS issued_at,row_number() OVER(PARTITION BY l.repository_id,l.environment,l.kind ORDER BY (l.document#>>'{document,issued_at}')::timestamptz DESC,l.id DESC) AS position
FROM release_receipts l JOIN inventory_changes c ON c.org_id=l.org_id AND c.repository_id=l.repository_id AND c.snapshot->>'merge_sha'=l.source_sha AND c.snapshot->>'state'='merged'
WHERE l.org_id=$1 AND ($2::uuid[] IS NULL OR l.repository_id=ANY($2)) AND (l.document#>>'{document,expires_at}')::timestamptz>clock_timestamp()
AND (EXISTS(SELECT 1 FROM repair_runs rr WHERE rr.org_id=l.org_id AND rr.repository_id=l.repository_id AND rr.state='published' AND rr.native_change->>'id'=c.snapshot->>'id') OR EXISTS(SELECT 1 FROM merge_operations m WHERE m.org_id=l.org_id AND m.repository_id=l.repository_id AND m.change_id=c.snapshot->>'id' AND m.state='merged'))
AND CASE l.kind WHEN 'native' THEN EXISTS(SELECT 1 FROM deployment_configurations cfg WHERE cfg.org_id=l.org_id AND cfg.environment=l.environment AND cfg.repository_id=l.repository_id AND cfg.version=l.configuration_version AND (cfg.document->>'enabled')::boolean) AND NOT EXISTS(SELECT 1 FROM deployments d JOIN deployment_gates g ON g.org_id=d.org_id AND g.id=d.gate_id WHERE d.org_id=l.org_id AND d.repository_id=l.repository_id AND d.environment=l.environment AND g.document#>>'{request,source_sha}'=l.source_sha AND g.document#>>'{request,artifact_digest}'=l.artifact_digest)
ELSE EXISTS(SELECT 1 FROM gitops_configurations cfg WHERE cfg.org_id=l.org_id AND cfg.environment=l.environment AND cfg.source_repository_id=l.repository_id AND cfg.version=l.configuration_version AND (cfg.document->>'enabled')::boolean AND coalesce(cfg.document->>'promote_from','')='') AND NOT EXISTS(SELECT 1 FROM gitops_promotions p JOIN gitops_gates g ON g.org_id=p.org_id AND g.id=p.gate_id WHERE p.org_id=l.org_id AND p.source_repository_id=l.repository_id AND p.environment=l.environment AND g.document#>>'{request,source_sha}'=l.source_sha AND g.document#>>'{request,artifact_digest}'=l.artifact_digest) END
) pending WHERE position<=3 ORDER BY issued_at DESC,id DESC LIMIT 10`, org, session.AutomationRepositories())
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		receipt, err := s.Delivery.Releases.Get(ctx, session, org, id)
		if err != nil {
			return err
		}
		delegated := false
		if err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			resolved, err := s.policies.ResolveTx(ctx, tx, org, receipt.Provenance.Document.RepositoryID)
			delegated = err == nil && !resolved.Paused && len(resolved.Problems) == 0 && !slices.Contains(resolved.Policy.Deny, policy.Deploy)
			return err
		}); err != nil {
			return err
		}
		if !delegated {
			continue
		}
		result, err := s.Delivery.Deploy(ctx, session, org, id, "autopilot-release")
		var provider *domain.ProviderError
		if errors.As(err, &provider) && provider.Kind == "release_superseded" {
			continue
		}
		if err != nil {
			slog.InfoContext(ctx, "autopilot release held", "org_id", org, "receipt_id", id, "reason", err)
		}
		if recordErr := s.deliveryOutcome(ctx, session, org, id, result.ID, err); recordErr != nil {
			return recordErr
		}
	}
	return nil
}

func (s *Service) deliveryOutcome(ctx context.Context, session auth.Session, org, id, operation string, failure error) error {
	receipt, err := s.Delivery.Releases.Get(ctx, session, org, id)
	if err != nil {
		return err
	}
	p := receipt.Provenance.Document
	scope := "delivery:" + p.Kind + ":" + p.Environment
	return s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, p.RepositoryID) {
			return auth.ErrForbidden
		}
		if failure != nil {
			var connection, branch string
			var version int64
			if err := tx.QueryRow(ctx, `SELECT r.connection_id::text,r.default_branch,c.version FROM repositories r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2`, org, p.RepositoryID).Scan(&connection, &branch, &version); err != nil {
				return err
			}
			reason := "Delivery unavailable; operator must review execution evidence"
			var provider *domain.ProviderError
			if errors.As(failure, &provider) {
				reason = provider.Message
			}
			if errors.Is(failure, auth.ErrForbidden) {
				reason = "Delivery permission unavailable; ask repository owner"
			}
			if errors.Is(failure, auth.ErrConflict) {
				reason = "Release or delivery settings changed; run Check now"
			}
			_, err := discovery.ObserveTx(ctx, tx, org, discovery.Observation{RepositoryID: p.RepositoryID, Source: "release_delivery", SourceID: scope, Category: "provider_access", Severity: "medium", Title: "Connect delivery to " + p.Environment, Evidence: discovery.Evidence{ConnectionID: connection, ConnectionVersion: version, TargetBranch: branch, HeadSHA: p.SourceSHA, TargetSHA: p.SourceSHA, Complete: false, Blockers: []string{reason}, ActionURL: "repositories?repository=" + p.RepositoryID, ActionLabel: "Connect deployment", ReleaseReceipt: id}}, a.UserID, "autopilot-release")
			return err
		}
		rows, err := tx.Query(ctx, `UPDATE maintenance_findings SET state='resolved',version=version+1,reason='Delivery recorded; follow operation outcome',evidence=jsonb_set(evidence,'{delivery_operation}',to_jsonb($4::text),true) WHERE org_id=$1 AND repository_id=$2 AND source='release_delivery' AND source_id=$3 AND state='open' RETURNING id::text,version`, org, p.RepositoryID, scope, operation)
		if err != nil {
			return err
		}
		type resolved struct {
			ID      string
			Version int64
		}
		items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[resolved])
		if err != nil {
			return err
		}
		for _, item := range items {
			data, _ := json.Marshal(map[string]string{"operation_id": operation, "release_receipt": id})
			if _, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data,repository_id) VALUES($1,$2,$3,'finding.resolved',$4,'autopilot-release',$5,$6)`, domain.NewID(), org, a.UserID, item.ID, data, p.RepositoryID); err != nil {
				return err
			}
			if err = workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, RepositoryID: p.RepositoryID, Type: "finding.resolved", AggregateType: "finding", AggregateID: item.ID, AggregateVersion: item.Version, DataVersion: 1, Data: data, RequestID: "autopilot-release"}); err != nil {
				return err
			}
		}
		return nil
	})
}
