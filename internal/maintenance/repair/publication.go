package repair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
	"reforge/internal/workflow"
	"strings"
	"time"
)

type writer interface {
	Write(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) (string, error), func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error)
}

func (s *Service) CheckStage(ctx context.Context, tx pgx.Tx, t workflow.Task, p policy.Resolved) error {
	if t.State != domain.TaskValidating {
		return workflow.ErrPolicy
	}
	for _, deny := range p.Policy.Deny {
		if deny == policy.Publish {
			return workflow.ErrPolicy
		}
	}
	r, err := loadRun(ctx, tx, t.OrgID, t.ID)
	if err != nil {
		return err
	}
	if r.Report == nil || r.Report.State != "validated" || r.Context.PolicyHash != p.Hash || (r.State != "validated" && r.State != "publishing") {
		return workflow.ErrPolicy
	}
	return nil
}
func (s *Service) CheckPublish(ctx context.Context, tx pgx.Tx, t workflow.Task, p policy.Resolved) error {
	r, err := loadRun(ctx, tx, t.OrgID, t.ID)
	if err != nil {
		return err
	}
	if t.State != domain.TaskPublishing || r.CandidateSHA == "" || r.Report == nil || !Verified(r.Context.Plan, r.Report.Baseline, r.CandidateChecks) {
		return workflow.ErrPolicy
	}
	binding := policy.Binding{Head: r.CandidateSHA, Target: r.Context.Plan.TargetSHA, Tested: r.CandidateSHA, PolicyHash: p.Hash}
	now := time.Now().UTC()
	facts := []policy.Evidence{}
	for _, id := range []string{"execution_authority", "validation", "branch_ownership", "exact_head_guard"} {
		facts = append(facts, policy.Evidence{ID: id, State: "satisfied", Binding: binding, ObservedAt: now, Reference: "repair-run:" + t.ID + "/" + id})
	}
	var money, concurrency, attempts, open int64
	if err = tx.QueryRow(ctx, `SELECT coalesce(sum(coalesce((record->'actual'->>'micro_usd')::bigint,(record->'maximum'->>'micro_usd')::bigint,0)),0) FROM budget_reservations WHERE org_id=$1 AND task_id=$2 AND state<>'cancelled'`, t.OrgID, t.ID).Scan(&money); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM workflow_jobs WHERE org_id=$1 AND state='running' AND lease_expires_at>clock_timestamp()`, t.OrgID).Scan(&concurrency); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM workflow_attempts WHERE org_id=$1 AND task_id=$2`, t.OrgID, t.ID).Scan(&attempts); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM maintenance_repairs WHERE org_id=$1 AND active`, t.OrgID).Scan(&open); err != nil {
		return err
	}
	files, lines := int64(len(r.Report.Patches)), int64(r.Context.Plan.MaxChangedLines)
	paths := []string{}
	for _, patch := range r.Report.Patches {
		paths = append(paths, patch.Path)
	}
	decision := policy.Evaluate(p, policy.Input{Action: policy.Publish, Recipe: t.Recipe, Model: r.Context.Model, Route: t.ModelConnectionID + "/" + t.ModelRoute, Current: binding, StartingPolicyHash: t.StartingPolicyHash, Paths: paths, Now: now, Evidence: facts, Usage: policy.Limits{Budget: &money, Concurrency: &concurrency, Attempts: &attempts, OpenChanges: &open, ChangedFiles: &files, ChangedLines: &lines}})
	if decision.Outcome != "allow" {
		return workflow.ErrPolicy
	}
	return nil
}
func (s *Service) checkNative(ctx context.Context, credential string, r Run) error {
	c := r.Context
	authorize := func(ctx context.Context, tx pgx.Tx, connection connections.Connection) error {
		if c.ConnectionID != connection.ID || c.ConnectionVersion != connection.Version {
			return auth.ErrConflict
		}
		return s.runners.WithJobTx(ctx, tx, credential, "repair.source", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
			if t.RepositoryID != c.Finding.RepositoryID {
				return auth.ErrForbidden
			}
			return nil
		})
	}
	if c.FollowUpBranch != "" {
		current, err := s.reader.Read(ctx, c.Finding.OrgID, c.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: c.Repository, Ref: c.FollowUpBranch}}, authorize)
		if err != nil {
			return err
		}
		if current.SHA != c.Plan.TargetSHA && (r.CandidateSHA == "" || current.SHA != r.CandidateSHA) {
			return discoveryStale()
		}
		return nil
	}
	current, err := s.reader.Read(ctx, c.Finding.OrgID, c.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: c.Repository, Ref: c.Finding.Evidence.TargetBranch}}, authorize)
	if err != nil {
		return err
	}
	if current.SHA != c.Plan.TargetSHA {
		return discoveryStale()
	}
	if c.Finding.Evidence.Change != nil {
		current, err = s.reader.Read(ctx, c.Finding.OrgID, c.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: c.Repository, ChangeID: c.Finding.Evidence.Change.ID}}, authorize)
		if err != nil {
			return err
		}
		if current.Change == nil || current.Change.State != "open" || current.Change.HeadSHA != c.Plan.BaselineSHA || current.Change.TargetSHA != c.Plan.TargetSHA {
			return discoveryStale()
		}
	}
	return nil
}
func discoveryStale() error {
	return errors.New("native source or target moved; refresh discovery before publication")
}
func (s *Service) Stage(ctx context.Context, credential string) (Run, error) {
	var r Run
	var lease workflow.Lease
	err := s.runners.WithJob(ctx, credential, "repair.stage", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		lease = l
		var err error
		r, err = loadRun(ctx, tx, l.OrgID, l.TaskID)
		r.Task = t
		return err
	})
	if err != nil {
		return r, err
	}
	if r.CandidateSHA != "" {
		return r, nil
	}
	if err = s.checkNative(ctx, credential, r); err != nil {
		return r, err
	}
	branch, expected := stageBranch(r, lease.TaskID), ""
	if r.Context.FollowUpBranch != "" {
		expected = r.Context.Plan.TargetSHA
	}
	patchBody, _ := json.Marshal(r.Report.Patches)
	payload, _ := json.Marshal(map[string]string{"branch": branch, "base": r.Context.Plan.TargetSHA, "patch_sha256": hashBytes(patchBody)})
	var intent workflow.Intent
	err = s.runners.WithJob(ctx, credential, "repair.stage", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		var err error
		intent, err = s.workflow.PrepareIntentTx(ctx, tx, l, "stage", "candidate", payload)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE repair_runs SET branch=$3,state='publishing',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND state IN ('validated','publishing')`, l.OrgID, l.TaskID, branch)
		return err
	})
	if err != nil {
		return r, err
	}
	edits := make([]forge.FileEdit, 0, len(r.Report.Patches))
	for _, patch := range r.Report.Patches {
		edits = append(edits, forge.FileEdit{Path: patch.Path, Content: patch.Content, Delete: patch.Delete})
	}
	op := privateconnector.Operation{ID: intent.OperationID, Kind: privateconnector.ForgeUpdateBranch, Branch: &forge.UpdateBranchRequest{Repository: r.Context.Repository, Branch: branch, BaseSHA: r.Context.Plan.TargetSHA, ExpectedOldSHA: expected, Message: changeTitle(r), Edits: edits, OperationID: intent.OperationID}}
	result, err := s.dispatch(ctx, credential, r, lease, intent, op)
	if err != nil {
		return r, err
	}
	if !source.ValidSHA(result.SHA, "sha1") {
		return r, workflow.ErrReconciliation
	}
	err = s.db.Tenant(ctx, lease.OrgID, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET candidate_sha=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND (candidate_sha='' OR candidate_sha=$3)`, lease.OrgID, lease.TaskID, result.SHA)
		return err
	})
	if err != nil {
		return r, err
	}
	r.CandidateSHA = result.SHA
	r.Branch = branch
	r.Context.NativeHeadSHA = result.SHA
	return r, nil
}
func stageBranch(r Run, task string) string {
	if r.Context.FollowUpBranch != "" {
		return r.Context.FollowUpBranch
	}
	return "reforge/repair/" + task
}
func changeTitle(r Run) string {
	if r.Report == nil || r.Report.Mode != "ci" || strings.TrimSpace(r.Report.Reason) == "" {
		return "Repair: " + r.Context.Finding.Title
	}
	title := []rune(strings.TrimSpace(strings.SplitN(strings.TrimSpace(r.Report.Reason), "\n", 2)[0]))
	if len(title) > 100 {
		title = append(title[:99], '…')
	}
	return string(title)
}
func (s *Service) dispatch(ctx context.Context, credential string, r Run, l workflow.Lease, intent workflow.Intent, op privateconnector.Operation) (privateconnector.Result, error) {
	var out privateconnector.Result
	w, ok := s.reader.(writer)
	if !ok {
		return out, privateconnector.ErrUnsupported
	}
	if intent.State == "succeeded" {
		if op.Kind == privateconnector.ForgeUpdateBranch && source.ValidSHA(intent.Evidence, "sha1") {
			out.SHA = intent.Evidence
			return out, nil
		}
		return out, workflow.ErrReconciliation
	}
	if intent.State != "pending" && intent.State != "absent" {
		return out, workflow.ErrReconciliation
	}
	dispatched := false
	out, err := w.Write(ctx, l.OrgID, r.Context.ConnectionID, op, func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		if c.Version != r.Context.ConnectionVersion {
			return "", auth.ErrConflict
		}
		err := s.runners.WithJobTx(ctx, tx, credential, intent.Kind, func(tx pgx.Tx, lease workflow.Lease, t workflow.Task) error {
			if lease.TaskID != l.TaskID || lease.AttemptID != l.AttemptID {
				return workflow.ErrFence
			}
			_, err := s.workflow.BeginIntentTx(ctx, tx, lease, intent.ID)
			return err
		})
		if err != nil {
			return "", err
		}
		dispatched = true
		return intent.ID, nil
	}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		return s.runners.WithJobTx(ctx, tx, credential, intent.Kind, func(tx pgx.Tx, current workflow.Lease, t workflow.Task) error {
			if current.TaskID != l.TaskID || current.AttemptID != l.AttemptID {
				return workflow.ErrFence
			}
			var active bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_outbox WHERE org_id=$1 AND id=$2 AND task_id=$3 AND state='dispatching')`, l.OrgID, intent.ID, l.TaskID).Scan(&active); err != nil {
				return err
			}
			if !active {
				return workflow.ErrReconciliation
			}
			return nil
		})
	})
	if !dispatched {
		return out, err
	}
	finalctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	outcome, evidence := "unknown", "Native outcome requires authoritative reconciliation"
	if err != nil {
		slog.WarnContext(ctx, "native write failed", "org_id", l.OrgID, "task_id", l.TaskID, "kind", op.Kind, "error", err)
	}
	if err == nil {
		if op.Kind == privateconnector.ForgeUpdateBranch && source.ValidSHA(out.SHA, "sha1") {
			outcome, evidence = "succeeded", out.SHA
		}
		if op.Kind == privateconnector.ForgeCreateChange && out.Change != nil && out.Change.OperationID == op.ID && out.Change.HeadSHA == op.Create.ExpectedHeadSHA && out.Change.HeadBranch == op.Create.HeadBranch && out.Change.TargetBranch == op.Create.TargetBranch {
			outcome, evidence = "succeeded", out.Change.ID
		}
	}
	if _, e := s.workflow.ReconcileIntent(finalctx, l.OrgID, intent.ID, outcome, evidence); e != nil {
		return privateconnector.Result{}, workflow.ErrReconciliation
	}
	if outcome != "succeeded" {
		return privateconnector.Result{}, workflow.ErrReconciliation
	}
	return out, nil
}
func (s *Service) RecordNative(ctx context.Context, credential string, in Publication) (Run, error) {
	var r Run
	err := s.runners.WithJob(ctx, credential, "repair.report", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		var err error
		r, err = loadRun(ctx, tx, l.OrgID, l.TaskID)
		if err != nil {
			return err
		}
		r.Task = t
		if in.HeadSHA != r.CandidateSHA || !source.ValidSHA(in.HeadSHA, "sha1") || in.PlanDigest != r.Context.Plan.Digest || r.Report == nil || len(in.Checks) == 0 || len(in.Checks) > len(r.Context.Plan.Recipe.Commands) || len(in.ArtifactIDs) != len(in.Checks) {
			return ErrValidation
		}
		body, _ := json.Marshal(in.Checks)
		artifactIDs, _ := json.Marshal(in.ArtifactIDs)
		if t.State != domain.TaskValidating || (r.State != "validated" && r.State != "publishing") {
			previous, _ := json.Marshal(r.CandidateChecks)
			previousIDs, _ := json.Marshal(r.CandidateArtifacts)
			if string(body) == string(previous) && string(artifactIDs) == string(previousIDs) {
				return nil
			}
			return auth.ErrConflict
		}
		for i, id := range in.ArtifactIDs {
			if !auth.ValidID(id) {
				return auth.ErrInvalid
			}
			var valid bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE org_id=$1 AND id=$2 AND task_id=$3 AND attempt_id=$4 AND sha256=$5 AND expires_at>clock_timestamp())`, l.OrgID, id, l.TaskID, l.AttemptID, in.Checks[i].OutputSHA256).Scan(&valid); err != nil {
				return err
			}
			if !valid {
				return ErrValidation
			}
		}
		_, err = tx.Exec(ctx, `UPDATE repair_runs SET candidate_checks=$3,candidate_artifacts=$5,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND candidate_sha=$4`, l.OrgID, l.TaskID, body, in.HeadSHA, artifactIDs)
		if err != nil {
			return err
		}
		r, err = loadRun(ctx, tx, l.OrgID, l.TaskID)
		r.Task = t
		return err
	})
	return r, err
}
func (s *Service) Publish(ctx context.Context, credential string, in Publication) (Run, error) {
	r, err := s.RecordNative(ctx, credential, in)
	if err != nil {
		return r, err
	}
	if !Verified(r.Context.Plan, r.Report.Baseline, in.Checks) {
		return r, ErrValidation
	}
	var lease workflow.Lease
	if err = s.checkNative(ctx, credential, r); err != nil {
		return r, err
	}
	var intent workflow.Intent
	err = s.runners.WithJob(ctx, credential, "repair.report", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		lease = l
		if t.State == domain.TaskValidating {
			if _, err := s.workflow.AdvanceTx(ctx, tx, l, domain.TaskPublishing); err != nil {
				return err
			}
		}
		if r.Context.FollowUpBranch != "" {
			return nil
		}
		payload, _ := json.Marshal(map[string]string{"head": r.CandidateSHA, "branch": r.Branch, "target": r.Context.Finding.Evidence.TargetBranch})
		var err error
		intent, err = s.workflow.PrepareIntentTx(ctx, tx, l, "publish", "companion", payload)
		return err
	})
	if err != nil {
		return r, err
	}
	if r.Context.FollowUpBranch != "" {
		return s.adoptChange(ctx, credential, r, lease, in.HeadSHA)
	}
	body := "Compatibility repair validated against pinned upgrade and target source."
	if r.Report != nil && r.Report.Mode == "ci" && r.Report.Reason != "" {
		body = r.Report.Reason + "\n\nFound on: " + r.Context.Finding.Title
	}
	request := forge.CreateChangeRequest{Repository: r.Context.Repository, Title: changeTitle(r), Body: body + "\n\nBaseline: " + r.Context.Plan.BaselineSHA + "\nTarget: " + r.Context.Plan.TargetSHA + "\nValidation plan: " + r.Context.Plan.Digest, HeadBranch: r.Branch, TargetBranch: r.Context.Finding.Evidence.TargetBranch, ExpectedHeadSHA: r.CandidateSHA, OperationID: intent.OperationID, Draft: false}
	if len(request.Title) > 250 {
		request.Title = request.Title[:250]
	}
	result, err := s.dispatch(ctx, credential, r, lease, intent, privateconnector.Operation{ID: intent.OperationID, Kind: privateconnector.ForgeCreateChange, Create: &request})
	if err != nil {
		return r, err
	}
	raw, _ := json.Marshal(result.Change)
	err = s.db.Tenant(ctx, lease.OrgID, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET state='published',native_change=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND candidate_sha=$4`, lease.OrgID, lease.TaskID, raw, in.HeadSHA)
		return err
	})
	if err != nil {
		return r, err
	}
	r.Change = result.Change
	r.State = "published"
	return r, nil
}
func (s *Service) adoptChange(ctx context.Context, credential string, r Run, lease workflow.Lease, head string) (Run, error) {
	c := r.Context
	current, err := s.reader.Read(ctx, c.Finding.OrgID, c.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: c.Repository, ChangeID: c.Finding.Evidence.Change.ID}}, func(ctx context.Context, tx pgx.Tx, connection connections.Connection) error {
		if c.ConnectionID != connection.ID || c.ConnectionVersion != connection.Version {
			return auth.ErrConflict
		}
		return s.runners.WithJobTx(ctx, tx, credential, "repair.report", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
			if l.TaskID != lease.TaskID {
				return workflow.ErrFence
			}
			return nil
		})
	})
	if err != nil {
		return r, err
	}
	if current.Change == nil || current.Change.State != "open" || current.Change.HeadBranch != c.FollowUpBranch || current.Change.HeadSHA != head {
		return r, workflow.ErrReconciliation
	}
	raw, _ := json.Marshal(current.Change)
	err = s.db.Tenant(ctx, lease.OrgID, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET state='published',native_change=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND candidate_sha=$4`, lease.OrgID, lease.TaskID, raw, head)
		return err
	})
	if err != nil {
		return r, err
	}
	r.Change = current.Change
	r.State = "published"
	return r, nil
}
func (s *Service) CheckCompletion(ctx context.Context, tx pgx.Tx, l workflow.Lease, t workflow.Task, in workflow.Completion) error {
	if in.Outcome != "completed" {
		return nil
	}
	r, err := loadRun(ctx, tx, l.OrgID, l.TaskID)
	if err != nil {
		return err
	}
	if r.State != "published" || r.Change == nil || r.Change.HeadSHA != r.CandidateSHA || r.Report == nil || !Verified(r.Context.Plan, r.Report.Baseline, r.CandidateChecks) || !strings.HasPrefix(r.Branch, "reforge/repair/") {
		return ErrValidation
	}
	return nil
}

const MaxFollowUps = 5

var ErrFollowUpsExhausted = errors.New("Reforge fix still failing CI after 5 follow-ups")

func (s *Service) CloseFix(ctx context.Context, session auth.Session, org string, f discovery.Finding) error {
	w, ok := s.reader.(writer)
	change := f.Evidence.Change
	if !ok || change == nil || !strings.HasPrefix(change.HeadBranch, "reforge/repair/") {
		return privateconnector.ErrUnsupported
	}
	failing := []string{}
	for _, check := range f.Evidence.Checks {
		if check.Conclusion == "failure" || check.Conclusion == "failed" || check.Conclusion == "timed_out" {
			failing = append(failing, check.Name)
		}
	}
	comment := fmt.Sprintf("Reforge could not get CI passing after %d follow-ups and is closing this pull request. Still failing: %s.", MaxFollowUps, strings.Join(failing, ", "))
	op := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeCloseChange, Close: &forge.CloseChangeRequest{Repository: change.Repository, ChangeID: change.ID, HeadBranch: change.HeadBranch, Comment: comment}}
	if _, err := w.Write(ctx, org, f.Evidence.ConnectionID, op, func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		if c.ID != f.Evidence.ConnectionID {
			return "", auth.ErrConflict
		}
		return op.ID, nil
	}, func(context.Context, pgx.Tx, connections.Connection) error { return nil }); err != nil {
		return err
	}
	return s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, f.RepositoryID) {
			return auth.ErrForbidden
		}
		if _, err := tx.Exec(ctx, `UPDATE maintenance_repairs m SET active=false FROM repair_runs r WHERE r.org_id=m.org_id AND r.task_id=m.task_id AND r.org_id=$1 AND r.repository_id=$2 AND r.branch=$3`, org, f.RepositoryID, change.HeadBranch); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE repair_runs SET bot_revalidation_state='closed',bot_revalidation_reason=$4,native_change=jsonb_set(native_change,'{state}','"closed"'),version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND repository_id=$2 AND branch=$3 AND state='published' AND native_change IS NOT NULL`, org, f.RepositoryID, change.HeadBranch, comment)
		return err
	})
}
