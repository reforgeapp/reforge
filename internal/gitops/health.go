package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/deployment"
	"reforge/internal/forge"
	"reforge/internal/source"
	"time"
)

func (s *Service) ReceiveHealth(ctx context.Context, org, id string, in HealthReport, signature string) error {
	if !auth.ValidID(org) || !auth.ValidID(id) || !auth.ValidID(in.Nonce) || in.OrgID != org || in.PromotionID != id || len(in.Checks) > 50 || len(signature) > 128 {
		return auth.ErrInvalid
	}
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if e := lock(ctx, tx, org); e != nil {
			return e
		}
		p, e := promotionTx(ctx, tx, org, id)
		if e != nil {
			return e
		}
		g, e := gateTx(ctx, tx, org, p.GateID)
		if e != nil {
			return e
		}
		c, e := configTx(ctx, tx, org, p.Environment)
		if e != nil {
			return e
		}
		if p.FinishedAt != nil || p.State != "completed_unverified" && p.State != "verifying" {
			return auth.ErrConflict
		}
		if !deployment.VerifySignature(c.HealthPublicKey, in, signature) {
			return auth.ErrForbidden
		}
		if c.Version != g.Configuration.Version || in.ConfigurationVersion != c.Version || in.Environment != p.Environment || in.SourceSHA != g.Request.SourceSHA || in.ArtifactDigest != g.Request.ArtifactDigest || !source.ValidSHA(p.MergeSHA, "sha1") || in.DeliveryRevision != p.MergeSHA {
			return auth.ErrConflict
		}
		now := time.Now()
		if in.ObservedAt.IsZero() || in.ObservedAt.Before(p.CreatedAt) || in.ObservedAt.After(now.Add(5*time.Second)) || now.Sub(in.ObservedAt) > time.Duration(c.MaxEvidenceAgeSeconds)*time.Second {
			return auth.ErrConflict
		}
		var prior []byte
		e = tx.QueryRow(ctx, `SELECT document FROM gitops_health_reports WHERE org_id=$1 AND promotion_id=$2 AND nonce=$3`, org, id, in.Nonce).Scan(&prior)
		if e == nil {
			var old HealthReport
			if json.Unmarshal(prior, &old) != nil || forge.DeliveryRulesHash(old) != forge.DeliveryRulesHash(in) {
				return auth.ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		var first, last *time.Time
		if e = tx.QueryRow(ctx, `SELECT first_healthy_at,last_health_at FROM gitops_promotions WHERE org_id=$1 AND id=$2`, org, id).Scan(&first, &last); e != nil {
			return e
		}
		if last != nil && !in.ObservedAt.After(*last) {
			return auth.ErrConflict
		}
		good := in.Healthy
		for _, name := range c.HealthChecks {
			good = good && in.Checks[name]
		}
		if !good {
			first = nil
		} else if first == nil || last == nil || in.ObservedAt.Sub(*last) > time.Duration(c.MaxEvidenceAgeSeconds)*time.Second {
			v := in.ObservedAt
			first = &v
		}
		raw, _ := json.Marshal(in)
		if _, e = tx.Exec(ctx, `INSERT INTO gitops_health_reports(org_id,promotion_id,nonce,document) VALUES($1,$2,$3,$4)`, org, id, in.Nonce, raw); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE gitops_promotions SET first_healthy_at=$3,last_health_at=$4,observe_after=now(),version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, first, in.ObservedAt); e != nil {
			return e
		}
		return emit(ctx, tx, org, p.DeliveryRepositoryID, "", "gitops.health", id, p.Version+1, "", map[string]any{"healthy": good, "delivery_revision": in.DeliveryRevision, "observed_at": in.ObservedAt})
	})
}
func healthState(ctx context.Context, tx pgx.Tx, org string, p Promotion, g Gate, c Configuration, revision string) (string, string, error) {
	reason := "Delivery change merged; fresh attributed reconciler health is required"
	if c.Version != g.Configuration.Version {
		return "completed_unverified", "GitOps configuration changed; previous health qualification cannot be reused", nil
	}
	var raw []byte
	var first, last *time.Time
	err := tx.QueryRow(ctx, `SELECT h.document,p.first_healthy_at,p.last_health_at FROM gitops_promotions p JOIN gitops_health_reports h ON h.org_id=p.org_id AND h.promotion_id=p.id WHERE p.org_id=$1 AND p.id=$2 ORDER BY h.received_at DESC LIMIT 1`, org, p.ID).Scan(&raw, &first, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		return "completed_unverified", reason, nil
	}
	if err != nil {
		return "", "", err
	}
	var h HealthReport
	if json.Unmarshal(raw, &h) != nil || last == nil || time.Since(*last) > time.Duration(c.MaxEvidenceAgeSeconds)*time.Second || h.DeliveryRevision != revision || h.SourceSHA != g.Request.SourceSHA || h.ArtifactDigest != g.Request.ArtifactDigest {
		return "completed_unverified", reason, nil
	}
	good := h.Healthy
	for _, name := range c.HealthChecks {
		good = good && h.Checks[name]
	}
	if !good {
		return "failed", "Authenticated reconciler health failed", nil
	}
	window := time.Duration(c.ObservationSeconds) * time.Second
	if first == nil || time.Since(*first) < window || h.ObservedAt.Sub(*first) < window {
		return "verifying", "Fresh reconciler observations have not completed the health window", nil
	}
	return "healthy", "Exact delivery revision, immutable artifact and attributed rollout health verified", nil
}
