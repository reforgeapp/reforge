package campaign

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/domain"
)

func active(state string) bool {
	return state == "canary" || state == "observing" || state == "expanding"
}

func (s *Service) budgetReady(ctx context.Context, tx pgx.Tx, org string, c Campaign) error {
	if c.Kind != "repair" {
		return nil
	}
	var ready bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM budget_limits WHERE org_id=$1 AND scope_kind='campaign' AND scope_id=$2 AND NOT paused AND caps<>'{}'::jsonb)`, org, c.ID).Scan(&ready)
	if e != nil {
		return e
	}
	if !ready {
		return budget.ErrUnknown
	}
	var unknown bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM budget_reservations WHERE org_id=$1 AND record->>'campaign_id'=$2 AND state='unknown')`, org, c.ID).Scan(&unknown)
	if e != nil {
		return e
	}
	if unknown {
		return fmt.Errorf("%w: campaign has unknown billed usage; reconcile it before resuming", budget.ErrCapacity)
	}
	return nil
}

func (s *Service) validatePending(ctx context.Context, tx pgx.Tx, org string, a domain.Actor, c Campaign, members []Member) error {
	for _, m := range members {
		if m.State == "excluded" {
			continue
		}
		if m.Canary && (m.State == "failed" || m.State == "cancelled") {
			return fmt.Errorf("%w: failed canary requires a new campaign with renewed canaries", auth.ErrConflict)
		}
		current, e := s.snapshotMember(ctx, tx, org, a, c.Spec, m)
		if e != nil {
			return e
		}
		if e = changedPins(m, current); e != nil {
			return e
		}
	}
	return s.budgetReady(ctx, tx, org, c)
}

func (s *Service) Control(ctx context.Context, session auth.Session, org, id, action, reason, decision string, expected int64, request string) (Campaign, error) {
	var out Campaign
	if session.AutomationID() != "" {
		return out, auth.ErrForbidden
	}
	if !auth.ValidID(id) || expected < 1 || !validReason(reason) {
		return out, auth.ErrInvalid
	}
	if action != "start" && action != "pause" && action != "resume" && action != "cancel" {
		return out, auth.ErrInvalid
	}
	if action == "resume" && decision != "continue_current_stage" {
		return out, auth.ErrInvalid
	}
	e := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		if e := readable(ctx, tx, org, id, a); e != nil {
			return e
		}
		var e error
		out, e = load(ctx, tx, org, id)
		if e != nil {
			return e
		}
		if out.Version != expected {
			return auth.ErrConflict
		}
		if out.State == "completed" || out.State == "cancelled" || out.State == "failed" {
			return auth.ErrConflict
		}
		members, e := memberRows(ctx, tx, org, id, "", 1001)
		if e != nil {
			return e
		}
		state := out.State
		switch action {
		case "start":
			if state != "planned" {
				return auth.ErrConflict
			}
			if e = s.validatePending(ctx, tx, org, a, out, members); e != nil {
				return e
			}
			state = "canary"
			out.Stage = 1
		case "pause":
			if !active(state) {
				return auth.ErrConflict
			}
			state = "paused"
		case "resume":
			if state != "paused" {
				return auth.ErrConflict
			}
			if e = s.validatePending(ctx, tx, org, a, out, members); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE campaign_members SET resume_requested=true,halt=false WHERE org_id=$1 AND campaign_id=$2 AND stage=$3 AND state IN ('queued','running','observing','blocked','unknown')`, org, id, out.Stage); e != nil {
				return e
			}
			state = "expanding"
			if out.Stage == 1 {
				state = "canary"
			}
		case "cancel":
			state = "cancelled"
		}
		if s.workflow == nil {
			return errors.New("campaign workflow authority is unavailable")
		}
		if e = s.workflow.SetCampaignPauseTx(ctx, tx, org, id, state == "paused" || state == "cancelled", a.UserID, request); e != nil {
			return e
		}
		if state == "paused" || state == "cancelled" {
			if _, e = tx.Exec(ctx, `UPDATE campaign_members SET stop_complete=false WHERE org_id=$1 AND campaign_id=$2`, org, id); e != nil {
				return e
			}
		}
		if state == "cancelled" {
			if _, e = tx.Exec(ctx, `UPDATE campaign_members SET state='cancelled',reason='Campaign cancelled before dispatch',updated_at=now() WHERE org_id=$1 AND campaign_id=$2 AND state='pending' AND action_id IS NULL AND gate_id IS NULL`, org, id); e != nil {
				return e
			}
		}
		_, e = tx.Exec(ctx, `UPDATE campaigns SET state=$3,reason=$4,stage=$5,version=version+1,observing_since=NULL,observe_due=now(),controller_id=NULL,controller_until=NULL,requested_by=$6,requested_session_id=$7,grant_expires_at=CASE WHEN $8 THEN now()+interval '30 days' ELSE grant_expires_at END,updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, state, strings.TrimSpace(reason), out.Stage, a.UserID, session.ID, action == "start" || action == "resume")
		if e != nil {
			return e
		}
		out, e = load(ctx, tx, org, id)
		if e != nil {
			return e
		}
		out.Counts, e = countTx(ctx, tx, org, id)
		if e != nil {
			return e
		}
		return emit(ctx, tx, org, a.UserID, "campaign."+action, id, request, out.Version, map[string]any{"reason": reason, "state": state, "stage_decision": decision, "stage": out.Stage})
	})
	return out, e
}
