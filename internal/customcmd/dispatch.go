package customcmd

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/domain"
	"reforge/internal/runner"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

type Dispatcher struct {
	db       *store.Store
	runners  *runner.Service
	profiles *Service
	budgets  *budget.Service
}

func NewDispatcher(db *store.Store, runners *runner.Service, profiles *Service, budgets *budget.Service) *Dispatcher {
	return &Dispatcher{db: db, runners: runners, profiles: profiles, budgets: budgets}
}

type Authorized struct {
	RunID   string      `json:"run_id"`
	Profile ProfileSpec `json:"profile"`
}

type ReportInput struct {
	RunID  string  `json:"run_id"`
	State  string  `json:"state"`
	Reason string  `json:"reason"`
	Usage  Usage   `json:"usage"`
	Events []Event `json:"events"`
}

var dispatchStates = map[string]bool{"completed_unverified": true, "failed": true, "unknown": true, "timed_out": true, "cancelled": true, "blocked": true}

func taskCreator(ctx context.Context, tx pgx.Tx, org, task string) (string, error) {
	var creator string
	err := tx.QueryRow(ctx, `SELECT created_by::text FROM workflow_tasks WHERE org_id=$1 AND id=$2`, org, task).Scan(&creator)
	return creator, err
}

func (d *Dispatcher) Authorize(ctx context.Context, credential string) (Authorized, error) {
	var out Authorized
	err := d.runners.WithJob(ctx, credential, "repair.custom", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		if t.State != domain.TaskPlanning && t.State != domain.TaskRepairing {
			return workflow.ErrPolicy
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT context FROM repair_runs WHERE org_id=$1 AND task_id=$2`, l.OrgID, l.TaskID).Scan(&raw); err != nil {
			return err
		}
		var stored struct {
			PolicyHash    string       `json:"policy_hash"`
			CustomProfile *ProfileSpec `json:"custom_profile"`
		}
		if err := json.Unmarshal(raw, &stored); err != nil || stored.CustomProfile == nil {
			return ErrNoProfile
		}
		if stored.PolicyHash != t.PolicyHash {
			return workflow.ErrPolicy
		}
		var lockedProfile string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM custom_profiles WHERE org_id=$1 AND id=$2 FOR UPDATE`, l.OrgID, stored.CustomProfile.ID).Scan(&lockedProfile); err != nil {
			return err
		}
		profile, err := d.profiles.Bind(ctx, tx, l.OrgID, stored.CustomProfile.ID, stored.CustomProfile.Version)
		if err != nil {
			return err
		}
		if profile.ImageDigest != stored.CustomProfile.ImageDigest {
			return ErrNotApproved
		}
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM custom_profile_runs WHERE org_id=$1 AND profile_id=$2 AND state IN ('queued','running')`, l.OrgID, profile.ID).Scan(&active); err != nil {
			return err
		}
		if active >= profile.Concurrency {
			return ErrCapacity
		}
		var model string
		if err := tx.QueryRow(ctx, `SELECT coalesce(settings->>'model','') FROM connections WHERE org_id=$1 AND id=$2`, l.OrgID, t.ModelConnectionID).Scan(&model); err != nil || model == "" {
			return ErrNoProfile
		}
		route, err := d.budgets.RouteTx(ctx, tx, l.OrgID, t.ModelConnectionID, model, t.ModelRoute)
		if err != nil {
			return err
		}
		var runID, creator string
		err = tx.QueryRow(ctx, `INSERT INTO custom_profile_runs(org_id,id,profile_id,profile_version,image_digest,task_id,attempt_id,repository_id,requested_by,state) SELECT $1,$2,$3,$4,$5,$6,$7,$8,t.created_by,'running' FROM workflow_tasks t WHERE t.org_id=$1 AND t.id=$6 ON CONFLICT(org_id,task_id,attempt_id) DO UPDATE SET state='running',reason='',usage='{}'::jsonb,events='[]'::jsonb,updated_at=now(),completed_at=NULL RETURNING id::text,requested_by::text`, l.OrgID, domain.NewID(), profile.ID, profile.Version, profile.ImageDigest, l.TaskID, l.AttemptID, l.RepositoryID).Scan(&runID, &creator)
		if err != nil {
			return err
		}
		quote := budget.Quote{OperationID: runID, Model: model, Route: t.ModelRoute, RouteVersion: route.Version, InputTokens: 0, MaxOutputTokens: int64(profile.MaxTurns), MaxMilliseconds: int64(profile.MaxWallSeconds) * 1000, MaxRequests: 1}
		reservation, err := d.budgets.ReserveTx(ctx, tx, budget.Lease(l), quote)
		if err != nil {
			return err
		}
		if _, err = d.budgets.MarkDispatchedTx(ctx, tx, budget.Lease(l), reservation.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE custom_profile_runs SET reservation_id=$3 WHERE org_id=$1 AND id=$2`, l.OrgID, runID, reservation.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,'custom_profile.dispatched',$4,$4,$5)`, domain.NewID(), l.OrgID, creator, runID, mustJSON(map[string]any{"profile_id": profile.ID, "profile_version": profile.Version, "image_digest": profile.ImageDigest})); err != nil {
			return err
		}
		out = Authorized{RunID: runID, Profile: SpecFromProfile(profile)}
		return nil
	})
	return out, err
}

func (d *Dispatcher) Report(ctx context.Context, credential string, in ReportInput) (Authorized, error) {
	var out Authorized
	if !auth.ValidID(in.RunID) || !dispatchStates[in.State] || len(in.Reason) > 2000 || len(in.Events) > 512 {
		return out, ErrInvalid
	}
	usage, err := json.Marshal(in.Usage)
	if err != nil {
		return out, ErrInvalid
	}
	events, err := json.Marshal(in.Events)
	if err != nil || len(events) > 1<<20 {
		return out, ErrInvalid
	}
	err = d.runners.WithJob(ctx, credential, "repair.custom", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		tag, err := tx.Exec(ctx, `UPDATE custom_profile_runs SET state=$4,reason=$5,usage=$6,events=$7,completed_at=clock_timestamp(),updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND attempt_id=$3 AND id=$8 AND state='running'`, l.OrgID, l.TaskID, l.AttemptID, in.State, in.Reason, usage, events, in.RunID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			var state, reason string
			if err := tx.QueryRow(ctx, `SELECT state,reason FROM custom_profile_runs WHERE org_id=$1 AND task_id=$2 AND attempt_id=$3 AND id=$4`, l.OrgID, l.TaskID, l.AttemptID, in.RunID).Scan(&state, &reason); err != nil {
				return err
			}
			if state == in.State && reason == in.Reason {
				return nil
			}
			return auth.ErrConflict
		}
		var reservationID string
		if err := tx.QueryRow(ctx, `SELECT coalesce(reservation_id::text,'') FROM custom_profile_runs WHERE org_id=$1 AND id=$2`, l.OrgID, in.RunID).Scan(&reservationID); err != nil {
			return err
		}
		if reservationID != "" {
			if in.Usage.Known {
				actual := budget.Amount{Tokens: in.Usage.Tokens, MicroUSD: in.Usage.MicroUSD, Milliseconds: in.Usage.Milliseconds, Requests: in.Usage.Requests}
				if _, err = d.budgets.SettleTx(ctx, tx, l.OrgID, reservationID, budget.Settlement{Known: true, Actual: actual, Reference: "custom-profile:" + in.RunID}); err != nil {
					return err
				}
			} else if _, err = d.budgets.MarkUnknownTx(ctx, tx, l.OrgID, reservationID, "custom-profile:"+in.RunID); err != nil {
				return err
			}
		}
		creator, err := taskCreator(ctx, tx, l.OrgID, l.TaskID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,'custom_profile.reported',$4,$4,$5)`, domain.NewID(), l.OrgID, creator, in.RunID, mustJSON(map[string]any{"state": in.State, "reason": in.Reason}))
		return err
	})
	return out, err
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte(`{}`)
	}
	return raw
}
