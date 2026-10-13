package gitops

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/environmenthealth"
)

func recordCurrentHealth(ctx context.Context, tx pgx.Tx, org string, p Promotion, g Gate, c Configuration, h HealthReport) error {
	good := h.Healthy
	for _, name := range c.HealthChecks {
		good = good && h.Checks[name]
	}
	return environmenthealth.RecordTx(ctx, tx, environmenthealth.Observation{OrgID: org, RepositoryID: p.SourceRepositoryID, Environment: p.Environment, Kind: "gitops", OperationID: p.ID, ActorID: p.RequestedBy, ConfigurationVersion: c.Version, ReleaseCreatedAt: p.CreatedAt, SourceSHA: g.Request.SourceSHA, ArtifactDigest: g.Request.ArtifactDigest, ConnectionID: g.SourceConnectionID, ConnectionVersion: g.SourceConnectionVersion, Healthy: good, ObservedAt: h.ObservedAt, ExpiresAt: h.ObservedAt.Add(time.Duration(c.MaxEvidenceAgeSeconds) * time.Second), Document: h})
}

func recordRolloutHealth(ctx context.Context, tx pgx.Tx, org string, p Promotion, g Gate, c Configuration) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT document FROM gitops_health_reports WHERE org_id=$1 AND promotion_id=$2 ORDER BY received_at DESC LIMIT 1`, org, p.ID).Scan(&raw); err != nil {
		return err
	}
	var h HealthReport
	if err := json.Unmarshal(raw, &h); err != nil {
		return err
	}
	return recordCurrentHealth(ctx, tx, org, p, g, c, h)
}
