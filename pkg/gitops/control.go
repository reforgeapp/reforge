package gitops

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
)

func (s *Service) Cancel(ctx context.Context, session auth.Session, org, id string, expected int64, request string) (Promotion, error) {
	var out Promotion
	if !auth.ValidID(id) || expected < 1 {
		return out, auth.ErrInvalid
	}
	type pending struct {
		id      string
		version int64
	}
	var merges []pending
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = promotionTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !manage(a, out.SourceRepositoryID, out.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		if out.Version != expected {
			return auth.ErrConflict
		}
		if out.FinishedAt != nil {
			return nil
		}
		state, reason, finished := out.State, "Reforge promotion stopped; close the native change to finish cancellation. Native merge may still race", false
		switch state {
		case "requested", "staging", "stage_uncertain", "staged", "blocked":
			state, reason, finished = "cancelled", "Publication cancelled; a candidate branch may remain for native cleanup", true
		}
		if _, err = tx.Exec(ctx, `UPDATE gitops_promotions SET cancel_requested=true,state=$3,reason=$4,finished_at=CASE WHEN $5 THEN now() ELSE finished_at END,version=version+1,updated_at=now(),observe_after=now() WHERE org_id=$1 AND id=$2`, org, id, state, reason, finished); err != nil {
			return err
		}
		if out.Change != nil {
			rows, err := tx.Query(ctx, `SELECT id::text,version FROM merge_operations WHERE org_id=$1 AND repository_id=$2 AND change_id=$3 AND state IN ('requested','dispatching','queued','reconciling','blocked')`, org, out.DeliveryRepositoryID, out.Change.ID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var m pending
				if err = rows.Scan(&m.id, &m.version); err != nil {
					rows.Close()
					return err
				}
				merges = append(merges, m)
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				return err
			}
		}
		out, err = promotionTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		return emit(ctx, tx, org, out.DeliveryRepositoryID, a.UserID, "gitops.cancellation_requested", id, out.Version, request, map[string]any{"state": state})
	})
	if err != nil {
		return out, err
	}
	for _, m := range merges {
		if s.merges != nil {
			if _, err = s.merges.Cancel(ctx, session, org, m.id, m.version, request); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

func (s *Service) MergePreview(ctx context.Context, session auth.Session, org, id, method, request string) (mergecontrol.Gate, error) {
	var out mergecontrol.Gate
	if s.merges == nil {
		return out, &domain.ProviderError{Kind: "unsupported", Message: "Configure the protected merge controller"}
	}
	d, err := s.Get(ctx, session, org, id)
	if err != nil {
		return out, err
	}
	if d.Promotion.Change == nil || d.Promotion.State != "awaiting_merge" || d.Promotion.CancelRequested {
		return out, auth.ErrConflict
	}
	return s.merges.Inspect(ctx, session, org, d.Promotion.DeliveryRepositoryID, d.Promotion.Change.ID, method, request)
}
func (s *Service) RequestMerge(ctx context.Context, session auth.Session, org, id, gateID, key, request string) (mergecontrol.Operation, error) {
	var out mergecontrol.Operation
	if !auth.ValidID(id) || !auth.ValidID(gateID) {
		return out, auth.ErrInvalid
	}
	if s.merges == nil {
		return out, &domain.ProviderError{Kind: "unsupported", Message: "Configure the protected merge controller"}
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		p, err := promotionTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !manage(a, p.SourceRepositoryID, p.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT document FROM merge_gates WHERE org_id=$1 AND id=$2`, org, gateID).Scan(&raw); err != nil {
			return err
		}
		var g mergecontrol.Gate
		if err = json.Unmarshal(raw, &g); err != nil {
			return err
		}
		if p.Change == nil || g.RepositoryID != p.DeliveryRepositoryID || g.Snapshot.Change.ID != p.Change.ID || g.Snapshot.Change.HeadSHA != p.CandidateSHA {
			return auth.ErrConflict
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	return s.merges.Request(ctx, session, org, gateID, key, request)
}
