package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/heartbeat"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
	"time"
)

func (s *Service) ObserveAs(ctx context.Context, session auth.Session, org, id string) (Promotion, error) {
	detail, err := s.Get(ctx, session, org, id)
	if err != nil {
		return Promotion{}, err
	}
	actor, err := s.auth.ResolveActor(ctx, session, org)
	if err != nil {
		return detail.Promotion, err
	}
	if !manage(actor, detail.Promotion.SourceRepositoryID, detail.Promotion.DeliveryRepositoryID) {
		return detail.Promotion, auth.ErrForbidden
	}
	return s.Observe(ctx, org, id)
}
func (s *Service) Observe(ctx context.Context, org, id string) (Promotion, error) {
	var out Promotion
	var g Gate
	if !auth.ValidID(org) || !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var e error
		out, e = promotionTx(ctx, tx, org, id)
		if e != nil {
			return e
		}
		g, e = gateTx(ctx, tx, org, out.GateID)
		return e
	})
	if err != nil {
		return out, err
	}
	if out.FinishedAt != nil || out.State == "blocked" || out.State == "cancelled" || out.State == "healthy" || out.State == "recovered" || out.State == "failed" || out.State == "recovery_failed" || out.State == "requested" || out.State == "staged" {
		return out, nil
	}
	if (out.State == "staging" || out.State == "publishing") && time.Since(out.UpdatedAt) < time.Minute {
		return out, nil
	}
	next, reason := out.State, out.Reason
	candidate := out.CandidateSHA
	native := out.Change
	merged := out.MergeSHA
	check := s.readCheck(org, g, nil)
	switch out.State {
	case "staging", "stage_uncertain":
		result, e := s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: g.Delivery, Ref: out.Branch}}, check)
		if e != nil {
			return out, e
		}
		proposed := out
		proposed.CandidateSHA = result.SHA
		if e = s.verifyCandidate(ctx, org, g, proposed, check); e != nil {
			return out, e
		}
		candidate = result.SHA
		next, reason = "staged", "Recovered immutable manifest candidate; continue to publish its protected change"
	case "publishing", "publish_uncertain":
		var publishID string
		if e := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT publish_dispatch_id::text FROM gitops_promotions WHERE org_id=$1 AND id=$2`, org, out.ID).Scan(&publishID)
		}); e != nil {
			return out, e
		}
		result, e := s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeFindChange, Find: &privateconnector.FindChangeArgs{Repository: g.Delivery, OperationID: publishID, HeadBranch: out.Branch, TargetBranch: g.Configuration.TargetBranch}}, check)
		if e != nil {
			return out, e
		}
		if result.Change == nil {
			return out, nil
		}
		if !changeMatches(g, out, *result.Change, publishID) {
			return out, auth.ErrConflict
		}
		native = result.Change
		next, reason = "awaiting_merge", "Recovered the original protected delivery change; publication was not repeated"
	default:
		if native == nil {
			return out, auth.ErrConflict
		}
		result, e := s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: g.Delivery, ChangeID: native.ID}}, check)
		if e != nil {
			return out, e
		}
		if result.Change == nil {
			return out, auth.ErrConflict
		}
		c := result.Change
		if c.ID != native.ID || c.Repository != g.Delivery || c.HeadRepository != g.Delivery || c.TargetRepository != g.Delivery || c.HeadSHA != candidate || c.HeadBranch != out.Branch || c.TargetBranch != g.Configuration.TargetBranch {
			return out, auth.ErrConflict
		}
		native = c
		switch c.State {
		case "open", "opened":
			next, reason = "awaiting_merge", "Native delivery change awaits checks, approvals and protected merge"
		case "closed":
			next, reason = "cancelled", "Native delivery change closed without promotion"
		case "merged":
			if !source.ValidSHA(c.MergeSHA, "sha1") || merged != "" && merged != c.MergeSHA {
				return out, auth.ErrConflict
			}
			merged = c.MergeSHA
			content, e := s.manifestFile(ctx, org, g, merged, check)
			if e != nil {
				return out, e
			}
			image, e := ReadManifestField(content, g.Configuration.Pointer)
			if e != nil || image != g.After {
				return out, auth.ErrConflict
			}
			next, reason = "completed_unverified", "Protected delivery change merged; reconciler rollout health remains unverified"
		default:
			return out, auth.ErrConflict
		}
	}
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if e := lock(ctx, tx, org); e != nil {
			return e
		}
		current, e := promotionTx(ctx, tx, org, out.ID)
		if e != nil {
			return e
		}
		if current.Version != out.Version {
			return auth.ErrConflict
		}
		cfg, e := configTx(ctx, tx, org, out.Environment)
		if e != nil {
			return e
		}
		if next == "completed_unverified" {
			next, reason, e = healthState(ctx, tx, org, current, g, cfg, merged)
			if e != nil {
				return e
			}
		}
		expired := time.Since(out.CreatedAt) > time.Duration(cfg.DeadlineSeconds)*time.Second
		if expired && (next == "verifying" || next == "completed_unverified") {
			next, reason = "completed_unverified", "Delivery change merged; attributed reconciler health did not complete before the deadline"
		}
		finished := next == "healthy" || next == "failed" || next == "cancelled" || expired && next == "completed_unverified"
		if out.RecoveryOf != "" {
			if next == "healthy" {
				next = "recovered"
			}
			if next == "failed" {
				next = "recovery_failed"
			}
		}
		if current.CancelRequested && next == "awaiting_merge" {
			reason = "Reforge promotion stopped; close the native change to finish cancellation. Native merge may still race"
		}
		if current.CancelRequested && merged != "" {
			reason = "Native merge completed despite cancellation; " + reason
		}
		var raw []byte
		if native != nil {
			raw, _ = json.Marshal(native)
		}
		_, e = tx.Exec(ctx, `UPDATE gitops_promotions SET state=$3,reason=$4,candidate_sha=$5,native_change=COALESCE($6,native_change),merge_sha=$7,finished_at=CASE WHEN $8 THEN now() ELSE finished_at END,observe_after=now()+interval '20 seconds',version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, out.ID, next, reason, candidate, raw, merged, finished)
		if e != nil {
			return e
		}
		out, e = promotionTx(ctx, tx, org, out.ID)
		if e != nil {
			return e
		}
		return emit(ctx, tx, org, out.DeliveryRepositoryID, "", "gitops.observed", out.ID, out.Version, "", map[string]any{"state": next, "delivery_revision": merged})
	})
	return out, err
}
func (s *Service) Run(ctx context.Context) error {
	cursor := "00000000-0000-0000-0000-000000000000"
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		heartbeat.Beat("promotions", time.Second, nil)
		var org string
		err := s.db.Pool.QueryRow(ctx, `SELECT org_id::text FROM inventory_tenants WHERE org_id>$1::uuid ORDER BY org_id LIMIT 1`, cursor).Scan(&org)
		if errors.Is(err, pgx.ErrNoRows) {
			cursor = "00000000-0000-0000-0000-000000000000"
			continue
		}
		if err != nil {
			continue
		}
		cursor = org
		var id string
		err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `UPDATE gitops_promotions SET observe_after=now()+interval '45 seconds' WHERE org_id=$1 AND id=(SELECT id FROM gitops_promotions WHERE org_id=$1 AND finished_at IS NULL AND state IN ('staging','stage_uncertain','publishing','publish_uncertain','awaiting_merge','verifying','completed_unverified') AND observe_after<=now() ORDER BY observe_after,id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id::text`, org).Scan(&id)
		})
		if err != nil {
			continue
		}
		bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
		_, _ = s.Observe(bounded, org, id)
		cancel()
	}
}
