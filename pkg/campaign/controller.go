package campaign

import (
	"context"
	"errors"
	"fmt"
	"github.com/reforgeapp/reforge/pkg/heartbeat"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func lock(ctx context.Context, tx pgx.Tx, org string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id)
}
func (s *Service) pauseTx(ctx context.Context, tx pgx.Tx, org string, c Campaign, reason string) error {
	if !active(c.State) {
		return nil
	}
	if _, e := tx.Exec(ctx, `UPDATE campaign_members SET stop_complete=false WHERE org_id=$1 AND campaign_id=$2`, org, c.ID); e != nil {
		return e
	}
	if e := s.workflow.SetCampaignPauseTx(ctx, tx, org, c.ID, true, c.RequestedBy, c.ID); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE campaigns SET state='paused',reason=$3,version=version+1,observing_since=NULL,updated_at=now() WHERE org_id=$1 AND id=$2`, org, c.ID, reason)
	if e != nil {
		return e
	}
	return emit(ctx, tx, org, c.RequestedBy, "campaign.paused", c.ID, c.ID, c.Version+1, map[string]any{"reason": reason})
}

func (s *Service) claim(ctx context.Context, org string) (Campaign, string, error) {
	var c Campaign
	token := domain.NewID()
	e := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if e := lock(ctx, tx, org); e != nil {
			return e
		}
		var id string
		e := tx.QueryRow(ctx, `UPDATE campaigns SET controller_id=$2,controller_until=now()+interval '75 seconds',observe_due=now()+interval '5 seconds' WHERE org_id=$1 AND id=(SELECT c.id FROM campaigns c WHERE c.org_id=$1 AND c.state IN ('canary','observing','expanding','paused','cancelled') AND (c.controller_until IS NULL OR c.controller_until<now()) AND c.observe_due<=now() AND (c.state IN ('canary','observing','expanding') OR EXISTS(SELECT 1 FROM campaign_members m WHERE m.org_id=c.org_id AND m.campaign_id=c.id AND (m.action_id IS NOT NULL OR m.gate_id IS NOT NULL) AND NOT m.stop_complete AND m.state NOT IN ('excluded','failed','cancelled'))) ORDER BY c.observe_due,c.id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id::text`, org, token).Scan(&id)
		if e != nil {
			return e
		}
		c, e = load(ctx, tx, org, id)
		return e
	})
	return c, token, e
}
func fence(ctx context.Context, tx pgx.Tx, org, id, token string) error {
	var valid bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campaigns WHERE org_id=$1 AND id=$2 AND controller_id=$3 AND controller_until>now())`, org, id, token).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return workflow.ErrFence
	}
	return nil
}

func (s *Service) stageTx(ctx context.Context, tx pgx.Tx, org string, c Campaign, members []Member) (bool, error) {
	failures, total := 0, len(members)
	current, finished := 0, 0
	for _, m := range members {
		if m.State == "failed" || m.State == "cancelled" {
			failures++
		}
		if !m.ResumeRequested && (m.Halt || m.State == "blocked" || m.State == "unknown" || (m.Canary && (m.State == "failed" || m.State == "blocked" || m.State == "unknown" || m.State == "cancelled"))) {
			return true, s.pauseTx(ctx, tx, org, c, "Member "+m.RepositoryName+" requires review: "+m.Reason)
		}
		if m.Stage == c.Stage && m.State != "excluded" {
			current++
			if m.State == "succeeded" || m.State == "failed" {
				finished++
			}
		}
	}
	if failures > c.Spec.FailureLimit || failures*100 > c.Spec.FailurePercent*total {
		return true, s.pauseTx(ctx, tx, org, c, "Campaign failure threshold exceeded; review all recorded members before continuing")
	}
	if current == 0 || finished != current {
		return false, nil
	}
	_, a, e := s.authorityTx(ctx, tx, org, c.ID)
	if e != nil {
		return true, s.pauseTx(ctx, tx, org, c, "Campaign approval or repository access changed; review authority before resuming")
	}
	for _, m := range members {
		if m.State != "succeeded" || (!m.Canary && m.Stage != c.Stage) {
			continue
		}
		latest, e := s.snapshotMember(ctx, tx, org, a, c.Spec, m)
		if e != nil {
			return false, e
		}
		if e = changedPins(m, latest); e != nil {
			return true, s.pauseTx(ctx, tx, org, c, "Completed canary or stage inputs changed; create a new reviewed campaign")
		}
	}
	if c.Kind == "repair" && c.Spec.ObservationSeconds > 0 {
		if c.ObservingSince == nil {
			_, e := tx.Exec(ctx, `UPDATE campaigns SET state='observing',observing_since=now(),version=version+1,reason='Validated outcomes are under observation',updated_at=now() WHERE org_id=$1 AND id=$2`, org, c.ID)
			if e == nil {
				e = emit(ctx, tx, org, c.RequestedBy, "campaign.observing", c.ID, c.ID, c.Version+1, map[string]any{"stage": c.Stage})
			}
			return true, e
		}
		if time.Since(*c.ObservingSince) < time.Duration(c.Spec.ObservationSeconds)*time.Second {
			return false, nil
		}
		deadline := c.ObservingSince.Add(time.Duration(c.Spec.ObservationSeconds) * time.Second)
		for _, m := range members {
			if m.Stage == c.Stage && m.State == "succeeded" && m.ObservedAt.Before(deadline) {
				return false, nil
			}
		}
	}
	next := 0
	for _, m := range members {
		if m.State == "pending" && m.Stage > c.Stage && (next == 0 || m.Stage < next) {
			next = m.Stage
		}
	}
	state, reason := "completed", "All eligible members reached the reviewed outcome; exclusions and failures remain recorded"
	if next > 0 {
		state, reason = "expanding", "Previous stage passed; next bounded stage is eligible"
	} else {
		next = c.Stage
	}
	_, e = tx.Exec(ctx, `UPDATE campaigns SET state=$3,stage=$4,reason=$5,version=version+1,observing_since=NULL,updated_at=now() WHERE org_id=$1 AND id=$2`, org, c.ID, state, next, reason)
	if e != nil {
		return true, e
	}
	return true, emit(ctx, tx, org, c.RequestedBy, "campaign."+state, c.ID, c.ID, c.Version+1, map[string]any{"stage": next})
}

func (s *Service) selectWork(ctx context.Context, org string, c Campaign, token string) (Campaign, Member, string, error) {
	var chosen Member
	purpose := ""
	e := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if e := lock(ctx, tx, org); e != nil {
			return e
		}
		if e := fence(ctx, tx, org, c.ID, token); e != nil {
			return e
		}
		var e error
		c, e = load(ctx, tx, org, c.ID)
		if e != nil {
			return e
		}
		if active(c.State) {
			if _, _, e = s.authorityTx(ctx, tx, org, c.ID); e != nil {
				if e = s.pauseTx(ctx, tx, org, c, "Campaign approval expired or initiating identity lost access; review authority before resuming"); e != nil {
					return e
				}
				c, e = load(ctx, tx, org, c.ID)
				if e != nil {
					return e
				}
			}
		}
		members, e := memberRows(ctx, tx, org, c.ID, "", 1001)
		if e != nil {
			return e
		}
		if active(c.State) {
			if e = s.budgetReady(ctx, tx, org, c); e != nil {
				return s.pauseTx(ctx, tx, org, c, "Campaign budget is unavailable, paused or has unknown usage; reconcile Usage before resuming")
			}
			advanced, e := s.stageTx(ctx, tx, org, c, members)
			if e != nil {
				return e
			}
			if advanced {
				return nil
			}
		}
		if !active(c.State) {
			chosen, e = scanMember(tx.QueryRow(ctx, `SELECT `+memberColumns+` FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND (action_id IS NOT NULL OR gate_id IS NOT NULL) AND NOT stop_complete AND state NOT IN ('excluded','failed','cancelled') ORDER BY observe_due,id LIMIT 1`, org, c.ID))
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			purpose = "cancel"
			return e
		}
		running := 0
		for _, m := range members {
			if m.Stage == c.Stage && (m.State == "queued" || m.State == "running" || m.State == "observing") {
				running++
			}
		}
		if withinWindow(c.Spec, time.Now()) {
			for _, m := range members {
				if m.Stage == c.Stage && (m.ResumeRequested || (m.State == "pending" && running < c.Spec.Concurrency)) {
					chosen = m
					purpose = "dispatch"
					return nil
				}
			}
		}
		chosen, e = scanMember(tx.QueryRow(ctx, `SELECT `+memberColumns+` FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND stage<=$3 AND action_id IS NOT NULL AND state NOT IN ('excluded','failed','cancelled') ORDER BY observe_due,id LIMIT 1`, org, c.ID, c.Stage))
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		purpose = "observe"
		return e
	})
	return c, chosen, purpose, e
}

func executionState(state string) bool {
	switch state {
	case "pending", "queued", "running", "observing", "succeeded", "failed", "blocked", "unknown", "cancelled":
		return true
	}
	return false
}
func (s *Service) record(ctx context.Context, org string, c Campaign, m Member, token, purpose string, result Execution, callErr error) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if e := lock(ctx, tx, org); e != nil {
			return e
		}
		if e := fence(ctx, tx, org, c.ID, token); e != nil {
			return e
		}
		current, e := memberTx(ctx, tx, org, c.ID, m.ID)
		if e != nil {
			return e
		}
		if callErr != nil {
			result.State = "unknown"
			result.Halt = true
			result.Reason = "Operation could not be confirmed; inspect its evidence and reconcile before resuming"
			if purpose == "dispatch" && current.ActionID == "" && result.ActionID == "" {
				result.State = "blocked"
				result.Reason = "Admission failed; review current credentials, policy, capacity and pinned evidence"
			}
		}
		if !executionState(result.State) {
			return fmt.Errorf("invalid campaign execution state")
		}
		if result.ActionID != "" && current.ActionID != "" && result.ActionID != current.ActionID {
			return auth.ErrConflict
		}
		if result.ActionID == "" {
			result.ActionID = current.ActionID
		}
		if result.State == "succeeded" && c.Kind != "repair" && result.VerifiedWindowSeconds < c.Spec.ObservationSeconds {
			result.State = "unknown"
			result.Halt = true
			result.Reason = "Native health has not verified the campaign observation window"
		}
		_, e = tx.Exec(ctx, `UPDATE campaign_members SET state=$4,reason=$5,action_id=coalesce(NULLIF($6,'')::uuid,action_id),halt=$7,stop_complete=CASE WHEN $8='cancel' AND $4 IN ('succeeded','failed','cancelled') THEN true ELSE stop_complete END,resume_requested=CASE WHEN $8='dispatch' THEN false ELSE resume_requested END,succeeded_at=CASE WHEN $4='succeeded' THEN coalesce(succeeded_at,now()) ELSE NULL END,observe_due=now()+interval '5 seconds',updated_at=now() WHERE org_id=$1 AND campaign_id=$2 AND id=$3`, org, c.ID, m.ID, result.State, result.Reason, result.ActionID, result.Halt, purpose)
		if e != nil {
			return e
		}
		if current.State != result.State || current.Reason != result.Reason || current.ActionID != result.ActionID {
			var version int64
			if e = tx.QueryRow(ctx, `UPDATE campaigns SET version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 RETURNING version`, org, c.ID).Scan(&version); e != nil {
				return e
			}
			if e = emit(ctx, tx, org, c.RequestedBy, "campaign.member", c.ID, c.ID, version, map[string]any{"member_id": m.ID, "repository_id": m.RepositoryID, "state": result.State, "action_id": result.ActionID, "reason": result.Reason}); e != nil {
				return e
			}
		}
		if active(c.State) && (result.Halt || (m.Canary && (result.State == "failed" || result.State == "cancelled"))) {
			currentCampaign, e := load(ctx, tx, org, c.ID)
			if e != nil {
				return e
			}
			return s.pauseTx(ctx, tx, org, currentCampaign, result.Reason)
		}
		return nil
	})
}

func (s *Service) Step(ctx context.Context, org string) error {
	if s.executor == nil {
		return errors.New("campaign executor is unavailable")
	}
	c, token, e := s.claim(ctx, org)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.db.Tenant(cleanup, org, "", func(tx pgx.Tx) error {
			_, e := tx.Exec(cleanup, `UPDATE campaigns SET controller_id=NULL,controller_until=NULL WHERE org_id=$1 AND id=$2 AND controller_id=$3`, org, c.ID, token)
			return e
		})
	}()
	c, m, purpose, e := s.selectWork(ctx, org, c, token)
	if e != nil || purpose == "" {
		return e
	}
	session, e := s.automationSession(ctx, org, c.ID, purpose, token)
	if e != nil {
		return e
	}
	var result Execution
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	switch purpose {
	case "dispatch":
		result, e = s.executor.Dispatch(bounded, session, org, c, m)
	case "observe":
		result, e = s.executor.Observe(bounded, session, org, c, m)
	case "cancel":
		result, e = s.executor.Cancel(bounded, session, org, c, m)
	}
	persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	return s.record(persist, org, c, m, token, purpose, result, e)
}

func (s *Service) Run(ctx context.Context) error {
	if s.executor == nil {
		return errors.New("campaign executor is unavailable")
	}
	jobs := make(chan string)
	completed := make(chan string, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for org := range jobs {
				_ = s.Step(ctx, org)
				select {
				case completed <- org:
				case <-ctx.Done():
				}
			}
		}()
	}
	defer func() { close(jobs); wg.Wait() }()
	scheduled := map[string]bool{}
	cursor := "00000000-0000-0000-0000-000000000000"
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case org := <-completed:
			delete(scheduled, org)
			continue
		case <-ticker.C:
		}
		heartbeat.Beat("campaigns", 100*time.Millisecond, nil)
		rows, e := s.db.Pool.Query(ctx, `SELECT org_id::text FROM inventory_tenants WHERE org_id>$1::uuid ORDER BY org_id LIMIT 64`, cursor)
		if e != nil {
			continue
		}
		orgs := []string{}
		for rows.Next() {
			var org string
			if e = rows.Scan(&org); e != nil {
				break
			}
			orgs = append(orgs, org)
		}
		rowErr := rows.Err()
		rows.Close()
		if e != nil || rowErr != nil {
			continue
		}
		if len(orgs) == 0 {
			cursor = "00000000-0000-0000-0000-000000000000"
			continue
		}
		for _, org := range orgs {
			cursor = org
			if scheduled[org] {
				continue
			}
			dispatched := false
			for !dispatched {
				select {
				case jobs <- org:
					scheduled[org] = true
					dispatched = true
				case done := <-completed:
					delete(scheduled, done)
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}
