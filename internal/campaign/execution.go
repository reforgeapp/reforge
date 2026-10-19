package campaign

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/deployment"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/gitops"
	"github.com/reforgeapp/reforge/internal/maintenance/repair"
	"github.com/reforgeapp/reforge/internal/mergecontrol"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type nativeExecutor struct {
	campaigns  *Service
	repairs    *repair.Service
	pipelines  *deployment.Service
	promotions *gitops.Service
	merges     *mergecontrol.Service
}

func (s *Service) ConfigureExecution(repairs *repair.Service, pipelines *deployment.Service, promotions *gitops.Service, merges *mergecontrol.Service) {
	s.executor = &nativeExecutor{s, repairs, pipelines, promotions, merges}
}

func (s *Service) admitTx(ctx context.Context, tx pgx.Tx, org string, c Campaign, m Member) error {
	current, a, e := s.authorityTx(ctx, tx, org, c.ID)
	if e != nil {
		return e
	}
	if current.Stage != m.Stage || !withinWindow(current.Spec, time.Now()) {
		return workflow.ErrPaused
	}
	latest, e := memberTx(ctx, tx, org, c.ID, m.ID)
	if e != nil {
		return e
	}
	if latest.State == "excluded" || latest.State == "cancelled" || latest.State == "failed" || latest.State == "succeeded" {
		return auth.ErrConflict
	}
	snapshot, e := s.snapshotMember(ctx, tx, org, a, current.Spec, latest)
	if e != nil {
		return e
	}
	if e = changedPins(latest, snapshot); e != nil {
		return e
	}
	if current.Stage > 1 {
		rows, e := tx.Query(ctx, `SELECT `+memberColumns+` FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND stage=1 AND state='succeeded'`, org, c.ID)
		if e != nil {
			return e
		}
		canaries := []Member{}
		for rows.Next() {
			m, e := scanMember(rows)
			if e != nil {
				rows.Close()
				return e
			}
			canaries = append(canaries, m)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, m := range canaries {
			next, e := s.snapshotMember(ctx, tx, org, a, current.Spec, m)
			if e != nil {
				return e
			}
			if e = changedPins(m, next); e != nil {
				return e
			}
		}
	}
	return s.budgetReady(ctx, tx, org, current)
}

func (n *nativeExecutor) intent(ctx context.Context, session auth.Session, org string, c Campaign, m Member) (string, error) {
	var gate string
	e := n.campaigns.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		return tx.QueryRow(ctx, `SELECT coalesce(gate_id::text,'') FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND id=$3`, org, c.ID, m.ID).Scan(&gate)
	})
	return gate, e
}
func (n *nativeExecutor) bindGate(ctx context.Context, session auth.Session, org string, c Campaign, m Member, gate string) error {
	return n.campaigns.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if e := n.campaigns.admitTx(ctx, tx, org, c, m); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE campaign_members SET gate_id=$4,updated_at=now() WHERE org_id=$1 AND campaign_id=$2 AND id=$3 AND gate_id IS NULL AND action_id IS NULL`, org, c.ID, m.ID, gate)
		if e == nil && tag.RowsAffected() != 1 {
			return auth.ErrConflict
		}
		return e
	})
}
func (n *nativeExecutor) Dispatch(ctx context.Context, session auth.Session, org string, c Campaign, m Member) (Execution, error) {
	if c.Kind == "repair" {
		if m.ActionID != "" {
			if m.ResumeRequested {
				t, e := n.campaigns.workflow.Get(ctx, session, org, m.ActionID)
				if e != nil {
					return Execution{}, e
				}
				if t.State == domain.TaskBlocked || t.State == domain.TaskReconciling {
					if _, e = n.campaigns.workflow.Resume(ctx, session, org, t.ID, t.Version, c.ID); e != nil {
						return Execution{}, e
					}
				}
			}
			return n.Observe(ctx, session, org, c, m)
		}
		in := *m.Input.Repair
		p, e := n.repairs.Preview(ctx, session, org, in)
		if e != nil {
			return Execution{}, e
		}
		if len(p.Blockers) > 0 {
			return Execution{State: "blocked", Reason: strings.Join(p.Blockers, "; "), Halt: true}, nil
		}
		in.PlanDigest = p.Context.Plan.Digest
		in.IdempotencyKey = "campaign/" + m.ID
		run, e := n.repairs.EnqueueCampaign(ctx, session, org, in, c.ID, c.ID, func(ctx context.Context, tx pgx.Tx, t workflow.Task) error {
			if e := n.campaigns.admitTx(ctx, tx, org, c, m); e != nil {
				return e
			}
			if t.CampaignID != c.ID || t.RepositoryID != m.RepositoryID || t.StartingPolicyHash != m.Pins["policy/"+m.RepositoryID] {
				return auth.ErrConflict
			}
			tag, e := tx.Exec(ctx, `UPDATE campaign_members SET action_id=$4,state='queued',updated_at=now() WHERE org_id=$1 AND campaign_id=$2 AND id=$3 AND action_id IS NULL`, org, c.ID, m.ID, t.ID)
			if e == nil && tag.RowsAffected() != 1 {
				return auth.ErrConflict
			}
			return e
		})
		if e != nil {
			return Execution{}, e
		}
		if run.Task.CampaignID != c.ID || run.Task.RepositoryID != m.RepositoryID {
			return Execution{}, auth.ErrConflict
		}
		m.ActionID = run.Task.ID
		return n.Observe(ctx, session, org, c, m)
	}
	gate, e := n.intent(ctx, session, org, c, m)
	if e != nil {
		return Execution{}, e
	}
	if gate == "" {
		if c.Kind == "pipeline" {
			p, e := n.pipelines.Preview(ctx, session, org, m.Input.Environment, *m.Input.Pipeline, c.ID)
			if e != nil {
				return Execution{}, e
			}
			if len(p.Blockers) > 0 || p.Decision.Outcome != "allow" {
				return Execution{State: "blocked", Reason: strings.Join(p.Blockers, "; "), Halt: true}, nil
			}
			gate = p.ID
		} else {
			p, e := n.promotions.Preview(ctx, session, org, m.Input.Environment, *m.Input.GitOps, c.ID)
			if e != nil {
				return Execution{}, e
			}
			if len(p.Blockers) > 0 || p.Decision.Outcome != "allow" {
				return Execution{State: "blocked", Reason: strings.Join(p.Blockers, "; "), Halt: true}, nil
			}
			gate = p.ID
		}
		if e = n.bindGate(ctx, session, org, c, m, gate); e != nil {
			return Execution{}, e
		}
	}
	if c.Kind == "pipeline" {
		o, e := n.pipelines.Request(ctx, session, org, gate, "campaign/"+m.ID, c.ID)
		if e == nil && o.State == "requested" {
			o, e = n.pipelines.Continue(ctx, session, org, o.ID, c.ID)
		}
		if e != nil {
			return Execution{ActionID: o.ID}, e
		}
		return pipelineExecution(o, c), nil
	}
	o, e := n.promotions.Request(ctx, session, org, gate, "campaign/"+m.ID, c.ID)
	if e == nil && (o.State == "requested" || o.State == "staged") {
		o, e = n.promotions.Continue(ctx, session, org, o.ID, c.ID)
	}
	if e != nil {
		return Execution{ActionID: o.ID}, e
	}
	return promotionExecution(o, c), nil
}

func pipelineExecution(o deployment.Operation, c Campaign) Execution {
	e := Execution{ActionID: o.ID, Reason: o.Reason, State: "running"}
	switch o.State {
	case "healthy":
		e.State = "succeeded"
		e.VerifiedWindowSeconds = c.Spec.ObservationSeconds
	case "failed":
		e.State = "failed"
		e.Halt = o.Native == nil || o.Native.State != "failed"
	case "recovery_failed", "recovered":
		e.State = "failed"
		e.Halt = true
	case "blocked":
		e.State = "blocked"
		e.Halt = true
	case "cancelled":
		e.State = "cancelled"
	case "reconciling":
		e.State = "unknown"
		e.Halt = true
	case "verifying", "completed_unverified":
		e.State = "observing"
		if o.FinishedAt != nil {
			e.State = "unknown"
			e.Halt = true
		}
	}
	return e
}
func promotionExecution(o gitops.Promotion, c Campaign) Execution {
	e := Execution{ActionID: o.ID, Reason: o.Reason, State: "running"}
	switch o.State {
	case "healthy":
		e.State = "succeeded"
		e.VerifiedWindowSeconds = c.Spec.ObservationSeconds
	case "failed", "recovery_failed", "recovered":
		e.State = "failed"
		e.Halt = true
	case "blocked":
		e.State = "blocked"
		e.Halt = true
	case "cancelled":
		e.State = "cancelled"
	case "stage_uncertain", "publish_uncertain":
		e.State = "unknown"
		e.Halt = true
	case "verifying", "completed_unverified":
		e.State = "observing"
		if o.FinishedAt != nil {
			e.State = "unknown"
			e.Halt = true
		}
	}
	return e
}
func (n *nativeExecutor) findAction(ctx context.Context, session auth.Session, org string, c Campaign, m Member) (string, error) {
	if m.ActionID != "" {
		return m.ActionID, nil
	}
	var id string
	e := n.campaigns.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		query := `SELECT id::text FROM deployments WHERE org_id=$1 AND gate_id=(SELECT gate_id FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND id=$3)`
		if c.Kind == "gitops" {
			query = `SELECT id::text FROM gitops_promotions WHERE org_id=$1 AND gate_id=(SELECT gate_id FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND id=$3)`
		}
		if c.Kind == "repair" {
			query = `SELECT action_id::text FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND id=$3 AND action_id IS NOT NULL`
		}
		e := tx.QueryRow(ctx, query, org, c.ID, m.ID).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		return e
	})
	return id, e
}
func (n *nativeExecutor) Observe(ctx context.Context, session auth.Session, org string, c Campaign, m Member) (Execution, error) {
	id, e := n.findAction(ctx, session, org, c, m)
	if e != nil {
		return Execution{}, e
	}
	if id == "" {
		return Execution{State: "pending", Reason: "No execution has been admitted"}, nil
	}
	if c.Kind == "pipeline" {
		o, e := n.pipelines.ObserveAs(ctx, session, org, id)
		return pipelineExecution(o, c), e
	}
	if c.Kind == "gitops" {
		o, e := n.promotions.ObserveAs(ctx, session, org, id)
		return promotionExecution(o, c), e
	}
	r, e := n.repairs.Get(ctx, session, org, id)
	if e != nil {
		return Execution{}, e
	}
	out := Execution{ActionID: id, State: "running", Reason: r.Task.Reason}
	switch r.Task.State {
	case domain.TaskQueued:
		out.State = "queued"
	case domain.TaskBlocked:
		out.State = "blocked"
		out.Halt = true
	case domain.TaskReconciling:
		out.State = "unknown"
		out.Halt = true
	case domain.TaskCancelled:
		out.State = "cancelled"
	case domain.TaskFailed:
		out.State = "failed"
	}
	if r.State == "published" && r.Change != nil && r.Change.HeadSHA == r.CandidateSHA && r.Report != nil && repair.Verified(r.Context.Plan, r.Report.Baseline, r.CandidateChecks) {
		native, e := n.repairs.ObserveChange(ctx, session, org, id)
		if e != nil {
			return out, e
		}
		if native.HeadSHA != r.CandidateSHA || native.HeadRepository != r.Context.Repository || native.TargetRepository != r.Context.Repository || native.HeadBranch != r.Branch || native.TargetBranch != r.Context.Finding.Evidence.TargetBranch || native.State == "closed" || native.Draft {
			return Execution{ActionID: id, State: "failed", Reason: "Published candidate changed or closed during observation; renewed canaries are required", Halt: true}, nil
		}
		if native.State != "open" && native.State != "merged" {
			return Execution{ActionID: id, State: "unknown", Reason: "Canonical change state is unverified", Halt: true}, nil
		}
		if c.Spec.Success == "published" {
			out.State = "succeeded"
			out.Halt = false
			return out, nil
		}
		var merged bool
		e = n.campaigns.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM merge_operations WHERE org_id=$1 AND repository_id=$2 AND change_id=$3 AND state='merged' AND native_result->>'head_sha'=$4 AND coalesce(native_result->>'merge_sha','')<>'')`, org, m.RepositoryID, r.Change.ID, r.CandidateSHA).Scan(&merged)
		})
		if e != nil {
			return out, e
		}
		out.State = "observing"
		out.Reason = "Waiting for canonical protected merge evidence"
		if merged && native.State == "merged" && native.MergeSHA != "" {
			out.State = "succeeded"
			out.Halt = false
		}
	}
	return out, nil
}

func (n *nativeExecutor) Cancel(ctx context.Context, session auth.Session, org string, c Campaign, m Member) (Execution, error) {
	id, e := n.findAction(ctx, session, org, c, m)
	if e != nil {
		return Execution{}, e
	}
	if id == "" {
		state := "pending"
		if c.State == "cancelled" {
			state = "cancelled"
		}
		return Execution{State: state, Reason: "No execution was admitted"}, nil
	}
	if c.Kind == "pipeline" {
		d, e := n.pipelines.Get(ctx, session, org, id)
		if e != nil {
			return Execution{}, e
		}
		o := d.Operation
		if o.FinishedAt == nil && !o.CancelRequested && ((o.Native != nil && o.Native.ID != "") || o.State == "requested") {
			o, e = n.pipelines.Cancel(ctx, session, org, id, o.Version, c.ID)
		}
		return pipelineExecution(o, c), e
	}
	if c.Kind == "gitops" {
		d, e := n.promotions.Get(ctx, session, org, id)
		if e != nil {
			return Execution{}, e
		}
		o := d.Promotion
		if o.FinishedAt == nil && !o.CancelRequested {
			o, e = n.promotions.Cancel(ctx, session, org, id, o.Version, c.ID)
		}
		return promotionExecution(o, c), e
	}
	r, e := n.repairs.Get(ctx, session, org, id)
	if e != nil {
		return Execution{}, e
	}
	if c.State == "cancelled" && r.Task.State != domain.TaskCompleted && r.Task.State != domain.TaskCancelled && r.Task.State != domain.TaskFailed {
		if _, e = n.campaigns.workflow.Cancel(ctx, session, org, id, r.Task.Version, c.ID); e != nil {
			return Execution{}, e
		}
	}
	if r.Change != nil && n.merges != nil {
		var mergeID string
		var version int64
		e = n.campaigns.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			e := tx.QueryRow(ctx, `SELECT id::text,version FROM merge_operations WHERE org_id=$1 AND repository_id=$2 AND change_id=$3 AND state IN ('requested','dispatching','queued','reconciling') AND NOT cancel_requested ORDER BY created_at LIMIT 1`, org, m.RepositoryID, r.Change.ID).Scan(&mergeID, &version)
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			return e
		})
		if e != nil {
			return Execution{}, e
		}
		if mergeID != "" {
			if _, e = n.merges.Cancel(ctx, session, org, mergeID, version, c.ID); e != nil {
				return Execution{}, e
			}
		}
	}
	return n.Observe(ctx, session, org, c, m)
}
