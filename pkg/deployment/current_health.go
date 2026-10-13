package deployment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/environmenthealth"
)

func recordCurrentHealth(ctx context.Context, tx pgx.Tx, org string, o Operation, g Gate, c Configuration, h HealthReport) error {
	good := h.Healthy
	for _, name := range c.HealthChecks {
		good = good && h.Checks[name]
	}
	return environmenthealth.RecordTx(ctx, tx, environmenthealth.Observation{OrgID: org, RepositoryID: o.RepositoryID, Environment: o.Environment, Kind: "native", OperationID: o.ID, ActorID: o.RequestedBy, ConfigurationVersion: c.Version, ReleaseCreatedAt: o.CreatedAt, SourceSHA: g.Request.SourceSHA, ArtifactDigest: g.Request.ArtifactDigest, ConnectionID: g.ConnectionID, ConnectionVersion: g.ConnectionVersion, Healthy: good, ObservedAt: h.ObservedAt, ExpiresAt: h.ObservedAt.Add(time.Duration(c.MaxEvidenceAgeSeconds) * time.Second), Document: h})
}

func recordRolloutHealth(ctx context.Context, tx pgx.Tx, org string, o Operation, g Gate, c Configuration) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT document FROM deployment_health_reports WHERE org_id=$1 AND deployment_id=$2 ORDER BY received_at DESC LIMIT 1`, org, o.ID).Scan(&raw); err != nil {
		return err
	}
	var h HealthReport
	if err := json.Unmarshal(raw, &h); err != nil {
		return err
	}
	return recordCurrentHealth(ctx, tx, org, o, g, c, h)
}
