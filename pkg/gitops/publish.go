package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/source"
	"time"
)

func (s *Service) Request(ctx context.Context, session auth.Session, org, gateID, key, request string) (Promotion, error) {
	var out Promotion
	if !auth.ValidID(gateID) || key == "" || len(key) > 128 {
		return out, auth.ErrInvalid
	}
	fresh := false
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		existing, e := scanPromotion(tx.QueryRow(ctx, `SELECT `+promotionColumns+` FROM gitops_promotions WHERE org_id=$1 AND idempotency_key=$2`, org, key))
		if e == nil {
			if !manage(a, existing.SourceRepositoryID, existing.DeliveryRepositoryID) {
				return auth.ErrForbidden
			}
			if existing.GateID != gateID {
				return auth.ErrConflict
			}
			out = existing
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		g, e := gateTx(ctx, tx, org, gateID)
		if e != nil {
			return e
		}
		c := g.Configuration
		if !manage(a, c.SourceRepositoryID, c.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		if !g.ExpiresAt.After(time.Now()) || g.Decision.Outcome != "allow" || len(g.Blockers) > 0 || !source.ValidSHA(g.TargetSHA, "sha1") || len(g.PatchedManifest) == 0 {
			return auth.ErrConflict
		}
		if e = s.checkGateAuthority(ctx, tx, org, g.ID); e != nil {
			return e
		}
		if e = s.currentTx(ctx, tx, org, g, ""); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO gitops_promotions(org_id,id,environment,source_repository_id,delivery_repository_id,gate_id,requested_by,idempotency_key,state,branch,target_branch,manifest_path,pointer,recovery_of) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'requested',$9,$10,$11,$12,NULLIF($13,'')::uuid)`, org, g.OperationID, c.Environment, c.SourceRepositoryID, c.DeliveryRepositoryID, g.ID, a.UserID, key, "reforge/promote/"+g.OperationID, c.TargetBranch, c.ManifestPath, c.Pointer, g.Request.RecoveryOf); e != nil {
			return e
		}
		out, e = promotionTx(ctx, tx, org, g.OperationID)
		if e != nil {
			return e
		}
		fresh = true
		return emit(ctx, tx, org, c.DeliveryRepositoryID, a.UserID, "gitops.requested", out.ID, out.Version, request, map[string]any{"source_repository_id": c.SourceRepositoryID, "environment": c.Environment})
	})
	if err != nil || !fresh {
		return out, err
	}
	return s.Continue(ctx, session, org, out.ID, request)
}
func (s *Service) Continue(ctx context.Context, session auth.Session, org, id, request string) (Promotion, error) {
	detail, err := s.Get(ctx, session, org, id)
	if err != nil {
		return Promotion{}, err
	}
	out := detail.Promotion
	if out.CancelRequested {
		return out, auth.ErrConflict
	}
	if out.State == "requested" {
		out, err = s.dispatch(ctx, session, org, out, detail.Gate, true, request)
		if err != nil {
			return out, err
		}
	}
	if out.State == "staged" {
		check := s.readCheck(org, detail.Gate, &session)
		if err = s.verifyCandidate(ctx, org, detail.Gate, out, check); err != nil {
			return out, err
		}
		return s.dispatch(ctx, session, org, out, detail.Gate, false, request)
	}
	return out, nil
}
func (s *Service) readCheck(org string, g Gate, session *auth.Session) func(context.Context, pgx.Tx, connections.Connection) error {
	return func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if session != nil {
			a, e := s.auth.ActorTx(ctx, tx, *session, org)
			if e != nil {
				return e
			}
			if !auth.CanReadRepository(a, g.Configuration.SourceRepositoryID) || !auth.CanReadRepository(a, g.Configuration.DeliveryRepositoryID) {
				return auth.ErrForbidden
			}
		}
		for _, item := range []struct {
			repo, id string
			ref      forge.RepoRef
		}{{g.Configuration.SourceRepositoryID, g.SourceConnectionID, g.Source}, {g.Configuration.DeliveryRepositoryID, g.DeliveryConnectionID, g.Delivery}} {
			ref, id, e := repository(ctx, tx, org, item.repo)
			if e != nil {
				return e
			}
			if ref != item.ref || id != item.id {
				return auth.ErrConflict
			}
		}
		if c.ID != g.SourceConnectionID && c.ID != g.DeliveryConnectionID {
			return auth.ErrForbidden
		}
		return nil
	}
}
func (s *Service) dispatch(ctx context.Context, session auth.Session, org string, out Promotion, g Gate, stage bool, request string) (Promotion, error) {
	expected, state, column := "staged", "publishing", "publish_dispatch_id"
	if stage {
		expected, state, column = "requested", "staging", "stage_dispatch_id"
	}
	dispatchID := domain.NewID()
	op := privateconnector.Operation{ID: dispatchID, Kind: privateconnector.ForgeCreateChange, Create: &forge.CreateChangeRequest{Repository: g.Delivery, Title: "Promote " + g.Configuration.Environment, Body: "Source: " + g.Request.SourceSHA + "\nArtifact: " + g.Request.ArtifactDigest + "\nEnvironment: " + g.Configuration.Environment, HeadBranch: out.Branch, TargetBranch: g.Configuration.TargetBranch, ExpectedHeadSHA: out.CandidateSHA, OperationID: dispatchID}}
	if stage {
		op = privateconnector.Operation{ID: dispatchID, Kind: privateconnector.ForgeUpdateBranch, Branch: &forge.UpdateBranchRequest{Repository: g.Delivery, Branch: out.Branch, BaseSHA: g.TargetSHA, Message: "Promote " + g.Configuration.Environment + " to " + g.Request.ArtifactDigest, Edits: []forge.FileEdit{{Path: g.Configuration.ManifestPath, Content: g.PatchedManifest}}, OperationID: dispatchID}}
	}
	if g.Request.RecoveryOf != "" && op.Create != nil {
		op.Create.Title = "Recover " + g.Configuration.Environment
		op.Create.Body += "\nRecovery of: " + g.Request.RecoveryOf
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, e := s.auth.ActorTx(ctx, tx, session, org)
		if e != nil {
			return e
		}
		if !manage(a, out.SourceRepositoryID, out.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		if e = originalAuthority(ctx, tx, org, out.RequestedBy, out.SourceRepositoryID, out.DeliveryRepositoryID); e != nil {
			return e
		}
		if c.ID != g.DeliveryConnectionID {
			return auth.ErrForbidden
		}
		if e = s.checkGateAuthority(ctx, tx, org, g.ID); e != nil {
			return e
		}
		if e = s.currentTx(ctx, tx, org, g, c.ID); e != nil {
			return e
		}
		var active bool
		e = tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM gitops_promotions WHERE org_id=$1 AND id=$2 AND state=$3 AND %s=$4 AND NOT cancel_requested AND finished_at IS NULL)`, column), org, out.ID, state, dispatchID).Scan(&active)
		if e != nil {
			return e
		}
		if !active {
			return auth.ErrConflict
		}
		return nil
	}
	claimed := false
	authorize := func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		tag, e := tx.Exec(ctx, fmt.Sprintf(`UPDATE gitops_promotions SET state=$3,%s=$4,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 AND state=$5 AND %s IS NULL AND NOT cancel_requested`, column, column), org, out.ID, state, dispatchID, expected)
		if e != nil {
			return "", e
		}
		if tag.RowsAffected() != 1 {
			return "", auth.ErrConflict
		}
		if e = check(ctx, tx, c); e != nil {
			return "", e
		}
		claimed = true
		return dispatchID, nil
	}
	result, callErr := s.providers.Write(ctx, org, g.DeliveryConnectionID, op, authorize, check)
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err := s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		if e := lock(persist, tx, org); e != nil {
			return e
		}
		current, e := promotionTx(persist, tx, org, out.ID)
		if e != nil {
			return e
		}
		var recorded string
		if e = tx.QueryRow(persist, fmt.Sprintf(`SELECT COALESCE(%s::text,'') FROM gitops_promotions WHERE org_id=$1 AND id=$2`, column), org, out.ID).Scan(&recorded); e != nil {
			return e
		}
		if recorded != "" && recorded != dispatchID {
			out = current
			return nil
		}
		if current.State != expected && current.State != state {
			out = current
			return nil
		}
		next, reason := "publish_uncertain", "Publication outcome requires canonical observation; the request will not be repeated"
		if stage {
			next, reason = "stage_uncertain", "Candidate branch outcome requires canonical observation; the request will not be repeated"
		}
		if !claimed {
			next, reason = "blocked", "Current source and delivery authority could not be established"
		}
		candidate := current.CandidateSHA
		var native []byte
		if claimed && callErr == nil {
			if stage && source.ValidSHA(result.SHA, "sha1") {
				candidate = result.SHA
				next, reason = "staged", "Immutable manifest candidate created; publication remains separate"
			}
			if !stage && result.Change != nil && changeMatches(g, current, *result.Change, dispatchID) {
				native, _ = json.Marshal(result.Change)
				next, reason = "awaiting_merge", "Protected delivery change awaits native checks and approvals"
			}
		}
		_, e = tx.Exec(persist, `UPDATE gitops_promotions SET state=$3,reason=$4,candidate_sha=$5,native_change=COALESCE($6,native_change),observe_after=now(),version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, org, out.ID, next, reason, candidate, native)
		if e != nil {
			return e
		}
		out, e = promotionTx(persist, tx, org, out.ID)
		if e != nil {
			return e
		}
		return emit(persist, tx, org, out.DeliveryRepositoryID, "", "gitops.dispatched", out.ID, out.Version, request, map[string]any{"state": next})
	})
	return out, err
}
func changeMatches(g Gate, p Promotion, c forge.Change, dispatch string) bool {
	return c.ID != "" && c.Repository == g.Delivery && c.HeadRepository == g.Delivery && c.TargetRepository == g.Delivery && c.HeadSHA == p.CandidateSHA && c.HeadBranch == p.Branch && c.TargetBranch == g.Configuration.TargetBranch && c.OperationID == dispatch
}
