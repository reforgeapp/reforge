package repair

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/source"
	"github.com/reforgeapp/reforge/pkg/workflow"
)

func RevalidationReferenceTx(ctx context.Context, tx pgx.Tx, org, repo, task, digest, head, target string) (string, error) {
	var id string
	var body, proof, evidence []byte
	err := tx.QueryRow(ctx, `SELECT rr.task_id::text,rr.context,rr.report,rr.candidate_checks FROM repair_runs rr JOIN workflow_tasks t ON t.org_id=rr.org_id AND t.id=rr.task_id WHERE rr.org_id=$1 AND rr.repository_id=$2 AND rr.context->>'validation_task_id'=$3 AND rr.context#>>'{request,plan_digest}'=$4 AND rr.candidate_sha=$5 AND rr.context->>'validation_target_sha'=$6 AND rr.state='validated' AND t.state='completed' ORDER BY rr.created_at DESC LIMIT 1`, org, repo, task, digest, head, target).Scan(&id, &body, &proof, &evidence)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var c ExecutionContext
	var report Report
	var checks []CheckResult
	if json.Unmarshal(body, &c) != nil || json.Unmarshal(proof, &report) != nil || json.Unmarshal(evidence, &checks) != nil {
		return "", ErrValidation
	}
	if c.Plan.BaselineSHA != head || c.Plan.TargetSHA != head || !VerifiedReport(c, report, checks) {
		return "", nil
	}
	return "repair-revalidation:" + id, nil
}

func (s *Service) RevalidateFix(ctx context.Context, session auth.Session, org, task string) (string, error) {
	r, err := s.Get(ctx, session, org, task)
	if err != nil || r.Change == nil || r.Report == nil || r.State != "published" || r.Context.Plan.Recipe.MinProof == "provider" {
		return "", err
	}
	c := r.Context
	if !c.Plan.Valid() || r.Report.PlanDigest != c.Plan.Digest || r.Report.State != "validated" && r.Report.State != "unverified" {
		return "", ErrValidation
	}
	if RequiresReproduction(c.Finding) && !BehaviourReproduced(c.Plan, r.Report.Reproduction) {
		return "", nil
	}
	authorize := func(ctx context.Context, tx pgx.Tx, connection connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !manage(a, r.Task.RepositoryID) || connection.ID != c.ConnectionID || connection.Version != c.ConnectionVersion {
			return auth.ErrConflict
		}
		return nil
	}
	current, err := s.reader.Read(ctx, org, c.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: c.Repository, ChangeID: r.Change.ID}}, authorize)
	if err != nil {
		return "", err
	}
	if current.Change == nil || current.Change.ID != r.Change.ID || current.Change.Repository != c.Repository || current.Change.State != "open" && current.Change.State != "opened" || current.Change.HeadBranch != r.Branch || current.Change.TargetBranch != r.Change.TargetBranch || current.Change.HeadRepository.NativeID != c.Repository.NativeID || current.Change.TargetRepository.NativeID != c.Repository.NativeID || !source.ValidSHA(current.Change.HeadSHA, "sha1") || !source.ValidSHA(current.Change.TargetSHA, "sha1") {
		return "", auth.ErrConflict
	}
	head, target := current.Change.HeadSHA, current.Change.TargetSHA
	if r.Report.State == "validated" && r.CandidateSHA == head && c.Plan.TargetSHA == target && VerifiedReport(c, *r.Report, r.CandidateChecks) {
		return "", nil
	}
	var reference string
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, r.Task.RepositoryID) {
			return auth.ErrForbidden
		}
		resolved, err := s.policies.ResolveTx(ctx, tx, org, r.Task.RepositoryID)
		if err != nil {
			return err
		}
		c.PolicyHash = resolved.Hash
		reference, err = RevalidationReferenceTx(ctx, tx, org, r.Task.RepositoryID, task, c.Plan.Digest, head, target)
		if err != nil || reference == "" || r.Report.State == "validated" {
			return err
		}
		reason := strings.TrimPrefix(strings.TrimSuffix(r.Report.Reason, ". Independent CI behaviour proof required before merge."), "Unverified proposal: ")
		_, err = tx.Exec(ctx, `WITH changed AS (UPDATE repair_runs SET report=jsonb_set(jsonb_set(report,'{state}','"validated"'::jsonb),'{reason}',to_jsonb($6::text)),version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND state='published' AND report->>'state'='unverified' RETURNING repository_id) INSERT INTO audit_events(id,org_id,repository_id,actor_id,action,object_id,request_id,data) SELECT gen_random_uuid(),$1,repository_id,'autopilot','repair.proof_revalidated',$2,'repair-revalidation',jsonb_build_object('reference',$3::text,'head',$4::text,'target',$5::text) FROM changed`, org, task, reference, head, target, reason)
		return err
	})
	if err != nil || reference != "" {
		return "", err
	}
	key := "repair-validation/" + domain.StableID(task, head, target, c.PolicyHash)
	var existing string
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		return tx.QueryRow(ctx, `SELECT state FROM workflow_tasks WHERE org_id=$1 AND idempotency_key=$2 AND repository_id=$3`, org, key, r.Task.RepositoryID).Scan(&existing)
	})
	if err == nil {
		if existing == "failed" || existing == "cancelled" || existing == "blocked" {
			return "Saved repair checks did not verify the current branch; waiting for updated code or validation tools", nil
		}
		return "Rechecking saved repair against current branch", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	snapshot, err := source.Fetch(ctx, s.reader.SourceReader(org, c.ConnectionID, authorize), c.Repository, head)
	if err != nil {
		return "", err
	}
	files, err := Files(snapshot)
	if err != nil {
		return "", err
	}
	c.ValidationTaskID, c.ValidationTargetSHA = task, target
	c.Request.PlanDigest = c.Plan.Digest
	c.Plan.BaselineSHA, c.Plan.TargetSHA = head, head
	c.Plan = retarget(c.Plan, files)
	c.NativeHeadSHA, c.BaselineRepository = head, c.Repository
	report := *r.Report
	report.PlanDigest, report.State, report.Reason = c.Plan.Digest, "validated", "Rechecking saved repair against current branch"
	report.Patches, report.Diff, report.Artifacts = nil, "", nil
	_, err = s.workflow.EnqueuePrepared(ctx, session, org, workflow.EnqueueInput{RepositoryID: r.Task.RepositoryID, Recipe: r.Task.Recipe, RecipeVersion: r.Task.RecipeVersion, TargetBranch: current.Change.TargetBranch, ModelConnectionID: r.Task.ModelConnectionID, ModelRoute: r.Task.ModelRoute, RunnerPoolID: r.Task.RunnerPoolID, PolicyHash: c.PolicyHash, IdempotencyKey: key, MaxAttempts: min(2, r.Task.MaxAttempts)}, "repair-revalidation", func(ctx context.Context, tx pgx.Tx, t workflow.Task) error {
		var parent bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repair_runs WHERE org_id=$1 AND task_id=$2 AND repository_id=$3 AND state='published' AND context#>>'{plan,digest}'=$4 AND native_change->>'id'=$5)`, org, task, t.RepositoryID, r.Context.Plan.Digest, r.Change.ID).Scan(&parent); err != nil {
			return err
		}
		if !parent {
			return auth.ErrConflict
		}
		c.PolicyHash = t.PolicyHash
		body, err := json.Marshal(c)
		if err != nil {
			return err
		}
		proof, err := json.Marshal(report)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context,report,state,candidate_sha) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'validated',$10)`, org, t.ID, t.RepositoryID, c.Finding.ID, session.User.ID, c.Finding.Version, c.Finding.EvidenceDigest, body, proof, head)
		return err
	})
	if err != nil {
		return "", err
	}
	return report.Reason, nil
}
