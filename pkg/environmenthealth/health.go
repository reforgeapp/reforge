package environmenthealth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
)

type Observation struct {
	OrgID, RepositoryID, Environment, Kind, OperationID, SourceSHA, ArtifactDigest, ConnectionID, ActorID string
	ConfigurationVersion, ConnectionVersion                                                               int64
	ReleaseCreatedAt, ObservedAt, ExpiresAt                                                               time.Time
	Healthy                                                                                               bool
	Document                                                                                              any
}

func RecordTx(ctx context.Context, tx pgx.Tx, in Observation) error {
	raw, err := json.Marshal(in.Document)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO environment_health(org_id,repository_id,environment,kind,operation_id,configuration_version,release_created_at,source_sha,artifact_digest,healthy,observed_at,expires_at,document) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(org_id,repository_id,environment,kind) DO UPDATE SET operation_id=EXCLUDED.operation_id,configuration_version=EXCLUDED.configuration_version,release_created_at=EXCLUDED.release_created_at,source_sha=EXCLUDED.source_sha,artifact_digest=EXCLUDED.artifact_digest,healthy=EXCLUDED.healthy,observed_at=EXCLUDED.observed_at,expires_at=EXCLUDED.expires_at,document=EXCLUDED.document WHERE environment_health.release_created_at<=EXCLUDED.release_created_at AND environment_health.observed_at<EXCLUDED.observed_at`, in.OrgID, in.RepositoryID, in.Environment, in.Kind, in.OperationID, in.ConfigurationVersion, in.ReleaseCreatedAt, in.SourceSHA, in.ArtifactDigest, in.Healthy, in.ObservedAt, in.ExpiresAt, raw)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	sourceID := "health:" + in.Kind + ":" + in.Environment
	if in.Healthy {
		_, err = tx.Exec(ctx, `UPDATE maintenance_findings SET state='resolved',reason=$4,version=version+1 WHERE org_id=$1 AND repository_id=$2 AND source='environment_health' AND source_id=$3 AND state='open'`, in.OrgID, in.RepositoryID, sourceID, "Application health verified again: "+in.OperationID+" at "+in.ObservedAt.UTC().Format(time.RFC3339))
		return err
	}
	var branch string
	if err := tx.QueryRow(ctx, `SELECT default_branch FROM repositories WHERE org_id=$1 AND id=$2`, in.OrgID, in.RepositoryID).Scan(&branch); err != nil {
		return err
	}
	_, err = discovery.ObserveTx(ctx, tx, in.OrgID, discovery.Observation{RepositoryID: in.RepositoryID, Source: "environment_health", SourceID: sourceID, Category: "deploy_failure", Severity: "high", Title: "Application failing in " + in.Environment, Evidence: discovery.Evidence{Provenance: "Authenticated ongoing " + in.Kind + " health at " + in.ObservedAt.UTC().Format(time.RFC3339), ConnectionID: in.ConnectionID, ConnectionVersion: in.ConnectionVersion, HeadSHA: in.SourceSHA, TargetSHA: in.SourceSHA, TargetBranch: branch, Deployment: in.OperationID, Complete: true, Review: &discovery.ReviewEvidence{Confidence: "high", Objective: "Restore application health in " + in.Environment, Detail: "Current release failed its configured application health checks"}}}, in.ActorID, "environment-health")
	return err
}

const CurrentPredicate = `(h.expires_at>clock_timestamp() AND CASE h.kind WHEN 'native' THEN EXISTS(SELECT 1 FROM deployment_configurations c WHERE c.org_id=h.org_id AND c.repository_id=h.repository_id AND c.environment=h.environment AND c.version=h.configuration_version) AND NOT EXISTS(SELECT 1 FROM deployments d WHERE d.org_id=h.org_id AND d.repository_id=h.repository_id AND d.environment=h.environment AND coalesce(d.native_result->>'id','')<>'' AND (d.created_at,d.id)>(h.release_created_at,h.operation_id)) ELSE EXISTS(SELECT 1 FROM gitops_configurations c WHERE c.org_id=h.org_id AND c.source_repository_id=h.repository_id AND c.environment=h.environment AND c.version=h.configuration_version) AND NOT EXISTS(SELECT 1 FROM gitops_promotions p WHERE p.org_id=h.org_id AND p.source_repository_id=h.repository_id AND p.environment=h.environment AND p.merge_sha<>'' AND (p.created_at,p.id)>(h.release_created_at,h.operation_id)) END)`
