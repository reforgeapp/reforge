package repair

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/connections"
	"reforge/internal/customcmd"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/runner"
	"reforge/internal/sandbox"
	"reforge/internal/source"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

var ErrRunNotFound = errors.New("repair context not found for this task")

type Service struct {
	db          *store.Store
	auth        *auth.Service
	findings    *discovery.Service
	workflow    *workflow.Service
	runners     *runner.Service
	policies    *policy.Service
	budgets     *budget.Service
	connections *connections.Service
	profiles    *customcmd.Service
	reader      discovery.Reader
	images      map[string]string
}

func New(db *store.Store, identity *auth.Service, findings *discovery.Service, jobs *workflow.Service, runners *runner.Service, policies *policy.Service, budgets *budget.Service, connections *connections.Service, profiles *customcmd.Service, reader discovery.Reader, images map[string]string) *Service {
	approved := map[string]string{}
	for name, digest := range images {
		if (name == "go" || name == "javascript" || name == "python") && strings.HasPrefix(digest, "sha256:") && source.ValidSHA(strings.TrimPrefix(digest, "sha256:"), "sha256") {
			approved[name] = digest
		}
	}
	return &Service{db, identity, findings, jobs, runners, policies, budgets, connections, profiles, reader, approved}
}
func (s *Service) Recipes() map[string]string {
	out := map[string]string{}
	for k, v := range s.images {
		out[k] = v
	}
	return out
}
func manage(a domain.Actor, repo string) bool {
	return auth.CanReadRepository(a, repo) && (a.Role == domain.Owner || a.Role == domain.Admin || a.Role == domain.Maintainer)
}
func (s *Service) Preview(ctx context.Context, session auth.Session, org string, in Input) (Preview, error) {
	out := Preview{Blockers: []string{}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	if !auth.ValidID(in.FindingID) || in.FindingVersion < 1 || !auth.ValidID(in.ModelConnectionID) || !auth.ValidID(in.RunnerPoolID) || len(in.ModelRoute) == 0 || len(in.ModelRoute) > 100 {
		return out, auth.ErrInvalid
	}
	if in.CustomProfileID != "" && (!auth.ValidID(in.CustomProfileID) || in.CustomProfileVersion < 1) {
		return out, auth.ErrInvalid
	}
	if in.CustomProfileID == "" && in.CustomProfileVersion != 0 {
		return out, auth.ErrInvalid
	}
	f, err := s.findings.Get(ctx, session, org, in.FindingID)
	if err != nil {
		return out, err
	}
	followUp, rounds := "", 0
	if f.Evidence.Change != nil && strings.HasPrefix(f.Evidence.Change.HeadBranch, "reforge/repair/") {
		followUp = f.Evidence.Change.HeadBranch
	}
	image := s.images[in.Recipe]
	if image == "" {
		out.Blockers = append(out.Blockers, "Administrator must register a verified runner image for this recipe")
		return out, nil
	}
	var resolved policy.Resolved
	var modelConnection connections.Connection
	var route budget.Route
	var ref forge.RepoRef
	var spec *customcmd.ProfileSpec
	var poolVersion int64
	attempts := 3
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		actor, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !manage(actor, f.RepositoryID) {
			return auth.ErrForbidden
		}
		if c.ID != f.Evidence.ConnectionID || c.Version != f.Evidence.ConnectionVersion {
			return discovery.ErrStale
		}
		current, err := discovery.PrepareRepairTx(ctx, tx, org, f.ID, in.FindingVersion)
		if err != nil {
			return err
		}
		if current.EvidenceDigest != f.EvidenceDigest {
			return discovery.ErrStale
		}
		return nil
	}
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, f.RepositoryID) {
			return auth.ErrForbidden
		}
		if _, err := discovery.PrepareRepairTx(ctx, tx, org, f.ID, in.FindingVersion); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT native_id,name FROM repositories WHERE org_id=$1 AND id=$2`, org, f.RepositoryID).Scan(&ref.NativeID, &ref.FullName); err != nil {
			return err
		}
		if followUp != "" {
			var owned bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repair_runs WHERE org_id=$1 AND repository_id=$2 AND branch=$3 AND state='published'),(SELECT count(*) FROM repair_runs WHERE org_id=$1 AND repository_id=$2 AND context->>'follow_up_branch'=$3 AND candidate_sha<>'')`, org, f.RepositoryID, followUp).Scan(&owned, &rounds); err != nil {
				return err
			}
			if !owned {
				out.Blockers = append(out.Blockers, "Branch is not a published Reforge fix")
			}
			if rounds >= MaxFollowUps {
				out.Blockers = append(out.Blockers, ErrFollowUpsExhausted.Error())
			}
			var own []byte
			if err := tx.QueryRow(ctx, `SELECT coalesce(report->'patches','[]') FROM repair_runs WHERE org_id=$1 AND repository_id=$2 AND branch=$3 ORDER BY created_at LIMIT 1`, org, f.RepositoryID, followUp).Scan(&own); err != nil {
				return err
			}
			rows, err := tx.Query(ctx, `SELECT coalesce(report->'patches','[]') FROM repair_runs WHERE org_id=$1 AND repository_id=$2 AND state='published' AND bot_revalidation_state NOT IN ('merged','closed') AND branch<>$3 AND created_at<(SELECT min(created_at) FROM repair_runs WHERE org_id=$1 AND branch=$3)`, org, f.RepositoryID, followUp)
			if err != nil {
				return err
			}
			older, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
			if err != nil {
				return err
			}
			var ownPatches []sandbox.Patch
			if json.Unmarshal(own, &ownPatches) == nil && len(ownPatches) > 0 {
				fixes := []map[string]string{}
				for _, raw := range older {
					var patches []sandbox.Patch
					if json.Unmarshal(raw, &patches) == nil {
						fixes = append(fixes, patchHashes(patches))
					}
				}
				if duplicates(patchHashes(ownPatches), fixes) {
					out.Blockers = append([]string{ErrDuplicateFix.Error()}, out.Blockers...)
				}
			}
		}
		var err error
		resolved, err = s.policies.ResolveTx(ctx, tx, org, f.RepositoryID)
		if err != nil {
			return err
		}
		modelConnection, err = s.connections.MetadataTx(ctx, tx, org, in.ModelConnectionID)
		if err != nil {
			return err
		}
		if modelConnection.State != "healthy" {
			return auth.ErrConflict
		}
		if in.CustomProfileID != "" {
			if modelConnection.Kind != "agent" || modelConnection.Provider != "custom_command" {
				return auth.ErrConflict
			}
			profile, err := s.profiles.Bind(ctx, tx, org, in.CustomProfileID, in.CustomProfileVersion)
			if err != nil {
				return err
			}
			bound := customcmd.SpecFromProfile(profile)
			spec = &bound
		} else if modelConnection.Kind != "model" || modelConnection.Settings.BillingRoute != "direct_api" {
			return auth.ErrConflict
		}
		route, err = s.budgets.RouteTx(ctx, tx, org, in.ModelConnectionID, modelConnection.Settings.Model, in.ModelRoute)
		if err != nil {
			return err
		}
		if spec != nil {
			if route.Mode != "quota" || route.Paused {
				return budget.ErrUnknown
			}
		} else if route.Mode != "priced" || route.Paused {
			return budget.ErrUnknown
		}
		if err = tx.QueryRow(ctx, `SELECT p.version FROM runner_pool_repositories rp JOIN runner_pools p ON p.org_id=rp.org_id AND p.id=rp.pool_id WHERE rp.org_id=$1 AND rp.pool_id=$2 AND rp.repository_id=$3 AND p.state='active'`, org, in.RunnerPoolID, f.RepositoryID).Scan(&poolVersion); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return auth.ErrForbidden
			}
			return err
		}
		if resolved.Policy.Limits.Attempts != nil {
			attempts = int(min(3, *resolved.Policy.Limits.Attempts))
		}
		if attempts < 1 {
			return workflow.ErrPolicy
		}
		_, err = s.workflow.CheckProposalTx(ctx, tx, workflow.Task{OrgID: org, RepositoryID: f.RepositoryID, Recipe: in.Recipe, ModelConnectionID: in.ModelConnectionID, ModelRoute: in.ModelRoute, RunnerPoolID: in.RunnerPoolID, MaxAttempts: attempts, State: domain.TaskQueued})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	baselineRef := ref
	if f.Evidence.Change != nil && f.Evidence.Change.HeadRepository.NativeID != "" {
		baselineRef = f.Evidence.Change.HeadRepository
	}
	snapshot, err := source.Fetch(ctx, s.reader.SourceReader(org, f.Evidence.ConnectionID, check), baselineRef, f.Evidence.HeadSHA)
	if err != nil {
		return out, err
	}
	files, err := Files(snapshot)
	if err != nil {
		return out, err
	}
	targetSHA := f.Evidence.TargetSHA
	if followUp != "" {
		targetSHA = f.Evidence.HeadSHA
	}
	plan, err := Freeze(in.Recipe, image, f.Evidence.HeadSHA, targetSHA, files, resolved.Policy.ForbiddenPaths)
	if err != nil {
		return out, err
	}
	if resolved.Policy.Limits.ChangedFiles != nil {
		plan.Recipe.MaxFiles = min(plan.Recipe.MaxFiles, int(min(int64(plan.Recipe.MaxFiles), *resolved.Policy.Limits.ChangedFiles)))
	}
	if resolved.Policy.Limits.ChangedLines != nil {
		plan.MaxChangedLines = int(min(int64(plan.MaxChangedLines), *resolved.Policy.Limits.ChangedLines))
	}
	authority, _ := json.Marshal([]any{resolved.Hash, modelConnection.ID, modelConnection.Version, route, spec, poolVersion, in.RunnerPoolID, f.ID, f.Version, f.EvidenceDigest, attempts})
	plan.AuthorityHash = hashBytes(authority)
	plan.Digest = planDigest(plan)
	context := ExecutionContext{MaxAttempts: attempts, Plan: plan, Repository: ref, BaselineRepository: baselineRef, ConnectionID: f.Evidence.ConnectionID, ConnectionVersion: f.Evidence.ConnectionVersion, Model: modelConnection.Settings.Model, MaxOutputTokens: int(min(route.MaxOutputTokens, 16384)), TurnTimeoutMS: min(route.MaxMilliseconds, (5 * time.Minute).Milliseconds()), CustomProfile: spec, Finding: f, PolicyHash: resolved.Hash}
	if spec != nil {
		context.MaxOutputTokens = 0
		context.TurnTimeoutMS = int64(spec.MaxWallSeconds) * 1000
	}
	context.FollowUpBranch = followUp
	context.CILogs = s.ciLogs(ctx, org, f, check)
	if len(context.CILogs) > 0 {
		context.OpenFixes, context.OpenFixFiles = s.openFixes(ctx, org, f.RepositoryID, followUp)
	}
	out.Context = context
	if !plan.Valid() {
		out.Blockers = append(out.Blockers, "Effective policy permits no source changes")
	}
	return out, nil
}
func (s *Service) Enqueue(ctx context.Context, session auth.Session, org string, in Input, request string) (Run, error) {
	return s.enqueue(ctx, session, org, in, request, "", nil)
}
func (s *Service) EnqueueCampaign(ctx context.Context, session auth.Session, org string, in Input, request, campaign string, admit func(context.Context, pgx.Tx, workflow.Task) error) (Run, error) {
	if !auth.ValidID(campaign) || admit == nil {
		return Run{}, auth.ErrInvalid
	}
	return s.enqueue(ctx, session, org, in, request, campaign, admit)
}
func (s *Service) enqueue(ctx context.Context, session auth.Session, org string, in Input, request, campaign string, admit func(context.Context, pgx.Tx, workflow.Task) error) (Run, error) {
	var out Run
	if len(in.IdempotencyKey) < 1 || len(in.IdempotencyKey) > 150 || len(in.PlanDigest) != 64 {
		return out, auth.ErrInvalid
	}
	var existing string
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var repo string
		err := tx.QueryRow(ctx, `SELECT t.id::text,t.repository_id::text FROM workflow_tasks t JOIN repair_runs r ON r.org_id=t.org_id AND r.task_id=t.id WHERE t.org_id=$1 AND t.idempotency_key=$2`, org, "repair/"+in.IdempotencyKey).Scan(&existing, &repo)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !manage(a, repo) {
			return auth.ErrForbidden
		}
		previous, err := loadRun(ctx, tx, org, existing)
		if err != nil {
			return err
		}
		if previous.Context.Request != in {
			return auth.ErrConflict
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if existing != "" {
		return s.Get(ctx, session, org, existing)
	}
	preview, err := s.Preview(ctx, session, org, in)
	if err != nil {
		return out, err
	}
	if len(preview.Blockers) > 0 || preview.Context.Plan.Digest != in.PlanDigest {
		return out, auth.ErrConflict
	}
	c := preview.Context
	c.Request = in
	task, err := s.workflow.EnqueuePrepared(ctx, session, org, workflow.EnqueueInput{CampaignID: campaign, RepositoryID: c.Finding.RepositoryID, Recipe: c.Plan.Recipe.Name, RecipeVersion: c.Plan.Recipe.Version, TargetBranch: c.Finding.Evidence.TargetBranch, ModelConnectionID: in.ModelConnectionID, ModelRoute: in.ModelRoute, RunnerPoolID: in.RunnerPoolID, PolicyHash: c.PolicyHash, IdempotencyKey: "repair/" + in.IdempotencyKey, MaxAttempts: c.MaxAttempts}, request, func(ctx context.Context, tx pgx.Tx, t workflow.Task) error {
		if admit != nil {
			if err := admit(ctx, tx, t); err != nil {
				return err
			}
		}
		f, err := discovery.PrepareRepairTx(ctx, tx, org, in.FindingID, in.FindingVersion)
		if err != nil {
			return err
		}
		if f.EvidenceDigest != c.Finding.EvidenceDigest {
			return discovery.ErrStale
		}
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO repair_runs(org_id,task_id,repository_id,finding_id,requested_by,finding_version,finding_digest,context) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, org, t.ID, t.RepositoryID, f.ID, session.User.ID, f.Version, f.EvidenceDigest, raw); err != nil {
			return err
		}
		return discovery.BindRepairTx(ctx, tx, f, t.ID)
	})
	if err != nil {
		return out, err
	}
	out, err = s.Get(ctx, session, org, task.ID)
	if err == nil && out.Context.Request != in {
		return Run{}, auth.ErrConflict
	}
	return out, err
}
func (s *Service) ciLogs(ctx context.Context, org string, f discovery.Finding, check func(context.Context, pgx.Tx, connections.Connection) error) []CILog {
	logs := []CILog{}
	for _, c := range f.Evidence.Checks {
		if c.Conclusion == "missing" && len(logs) < 3 {
			logs = append(logs, CILog{Name: c.Name, Log: "This check passes on the target branch but did not run or pass on this pull request. Restore it; the change must keep every existing CI check."})
			continue
		}
		name := strings.ToLower(c.Name)
		if len(logs) == 3 || c.Conclusion != "failure" && c.Conclusion != "failed" && c.Conclusion != "timed_out" || name == "dependabot" || name == "renovate" {
			continue
		}
		repo := f.Evidence.Change
		ref := forge.RepoRef{}
		if repo != nil {
			ref = repo.Repository
		}
		if ref.NativeID == "" {
			if err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT native_id,name FROM repositories WHERE org_id=$1 AND id=$2`, org, f.RepositoryID).Scan(&ref.NativeID, &ref.FullName)
			}); err != nil {
				continue
			}
		}
		result, err := s.reader.Read(ctx, org, f.Evidence.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeCheckLog, CheckLog: &privateconnector.CheckLogArgs{Repository: ref, CheckID: c.ID}}, check)
		if err != nil || result.Log == "" {
			continue
		}
		logs = append(logs, CILog{Name: c.Name, URL: c.URL, Log: result.Log})
	}
	return logs
}

func (s *Service) openFixes(ctx context.Context, org, repository, exclude string) ([]string, []map[string]string) {
	fixes, files := []string{}, []map[string]string{}
	_ = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT branch,coalesce(report->>'reason',''),coalesce(report->'dependencies','[]')::text,coalesce(report->'patches','[]') FROM repair_runs WHERE org_id=$1 AND repository_id=$2 AND state='published' AND bot_revalidation_state NOT IN ('merged','closed') AND branch<>$3 ORDER BY created_at DESC LIMIT 20`, org, repository, exclude)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var branch, reason, deps string
			var raw []byte
			var patches []sandbox.Patch
			if rows.Scan(&branch, &reason, &deps, &raw) != nil || json.Unmarshal(raw, &patches) != nil {
				continue
			}
			fixes = append(fixes, bounded(branch+": "+reason+" "+deps))
			files = append(files, patchHashes(patches))
		}
		return rows.Err()
	})
	return fixes, files
}

func loadRun(ctx context.Context, tx pgx.Tx, org, id string) (Run, error) {
	var out Run
	var body, report, checks, change, artifactIDs []byte
	err := tx.QueryRow(ctx, `SELECT context,report,state,version,updated_at,branch,candidate_sha,candidate_checks,native_change,candidate_artifacts FROM repair_runs WHERE org_id=$1 AND task_id=$2`, org, id).Scan(&body, &report, &out.State, &out.Version, &out.UpdatedAt, &out.Branch, &out.CandidateSHA, &checks, &change, &artifactIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrRunNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(body, &out.Context); err != nil {
		return out, err
	}
	if len(report) > 0 {
		err = json.Unmarshal(report, &out.Report)
	}
	if len(checks) > 0 {
		if e := json.Unmarshal(checks, &out.CandidateChecks); e != nil {
			return out, e
		}
	}
	if len(change) > 0 {
		if e := json.Unmarshal(change, &out.Change); e != nil {
			return out, e
		}
	}
	if e := json.Unmarshal(artifactIDs, &out.CandidateArtifacts); e != nil {
		return out, e
	}
	out.Context.NativeHeadSHA = out.CandidateSHA
	return out, err
}
func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Run, error) {
	var out Run
	t, err := s.workflow.Get(ctx, session, org, id)
	if err != nil {
		return out, err
	}
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, t.RepositoryID) {
			return auth.ErrForbidden
		}
		var err error
		out, err = loadRun(ctx, tx, org, id)
		return err
	})
	out.Task = t
	return out, err
}
func (s *Service) Baseline(ctx context.Context, session auth.Session, org, repo string) (*Run, error) {
	if !auth.ValidID(repo) {
		return nil, auth.ErrInvalid
	}
	var id string
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE org_id=$1 AND id=$2 AND accessible)`, org, repo).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return auth.ErrForbidden
		}
		err := tx.QueryRow(ctx, `SELECT r.task_id::text FROM repair_runs r JOIN workflow_tasks t ON t.org_id=r.org_id AND t.id=r.task_id WHERE r.org_id=$1 AND t.repository_id=$2 ORDER BY t.created_at DESC,t.id DESC LIMIT 1`, org, repo).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil || id == "" {
		return nil, err
	}
	out, err := s.Get(ctx, session, org, id)
	return &out, err
}
func (s *Service) Context(ctx context.Context, credential string) (ExecutionContext, error) {
	var out ExecutionContext
	err := s.runners.WithJob(ctx, credential, "repair.source", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		r, err := loadRun(ctx, tx, l.OrgID, l.TaskID)
		if err != nil {
			return err
		}
		if r.Context.PolicyHash != t.PolicyHash {
			return workflow.ErrPolicy
		}
		out = r.Context
		return nil
	})
	return out, err
}
func (s *Service) Snapshot(ctx context.Context, credential, sha string) (sandbox.Snapshot, error) {
	var out sandbox.Snapshot
	c, err := s.Context(ctx, credential)
	if err != nil {
		return out, err
	}
	if sha != c.Plan.BaselineSHA && sha != c.Plan.TargetSHA && sha != c.NativeHeadSHA {
		return out, auth.ErrForbidden
	}
	ref := c.Repository
	if sha == c.Plan.BaselineSHA {
		ref = c.BaselineRepository
	}
	authorize := func(ctx context.Context, tx pgx.Tx, connection connections.Connection) error {
		if connection.ID != c.ConnectionID || connection.Version != c.ConnectionVersion {
			return auth.ErrConflict
		}
		return s.runners.WithJobTx(ctx, tx, credential, "repair.source", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
			if t.RepositoryID != c.Finding.RepositoryID || t.OrgID != c.Finding.OrgID || t.PolicyHash != c.PolicyHash {
				return auth.ErrForbidden
			}
			return nil
		})
	}
	return source.Fetch(ctx, s.reader.SourceReader(c.Finding.OrgID, c.ConnectionID, authorize), ref, sha)
}
func (s *Service) SaveReport(ctx context.Context, credential string, in Report) (Run, error) {
	var out Run
	in.Diff = ""
	raw, err := json.Marshal(in)
	if err != nil || len(raw) > 1<<20 {
		return out, auth.ErrInvalid
	}
	ci := in.Mode == "ci"
	if in.Mode != "" && !ci || len(in.Dependencies) > 10 {
		return out, auth.ErrInvalid
	}
	for _, u := range in.Dependencies {
		if !u.Valid() {
			return out, auth.ErrInvalid
		}
	}
	var original map[string][]byte
	if in.State == "validated" {
		c, err := s.Context(ctx, credential)
		if err != nil {
			return out, err
		}
		sha := c.Plan.BaselineSHA
		if ci {
			if len(c.CILogs) == 0 {
				return out, ErrValidation
			}
			sha = c.Plan.TargetSHA
		}
		snapshot, err := s.Snapshot(ctx, credential, sha)
		if err != nil {
			return out, err
		}
		original, err = Files(snapshot)
		if err != nil {
			return out, err
		}
		if ci && CheckCIPatch(c.Plan, original, in.Patches, in.Dependencies) != nil || !ci && CheckPatch(c.Plan, original, in.Patches) != nil {
			return out, ErrPatch
		}
		in.Diff = SourceDiff(original, in.Patches)
		in.ChangedLines = PatchLines(original, in.Patches)
		raw, err = json.Marshal(in)
		if err != nil || len(raw) > 1<<20 {
			return out, auth.ErrInvalid
		}
	}
	err = s.runners.WithJob(ctx, credential, "repair.report", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		current, err := loadRun(ctx, tx, l.OrgID, l.TaskID)
		if err != nil {
			return err
		}
		if current.Report != nil {
			previous, _ := json.Marshal(current.Report)
			if string(previous) == string(raw) {
				current.Task = t
				out = current
				return nil
			}
		}
		p := current.Context.Plan
		if in.PlanDigest != p.Digest || in.Turns < 0 || in.Turns > p.Recipe.MaxTurns || len(in.Reason) > 2000 || len(in.Artifacts) > 100 {
			return auth.ErrInvalid
		}
		if in.State == "validated" {
			if t.State != domain.TaskValidating || !ci && !Reproduced(in.Baseline) || !Verified(p, in.Baseline, in.Candidate) || !Verified(p, in.Baseline, in.Target) || len(in.Patches) == 0 {
				return ErrValidation
			}
		} else if in.State != "handoff" {
			return auth.ErrInvalid
		}
		for _, id := range in.Artifacts {
			if !auth.ValidID(id) {
				return auth.ErrInvalid
			}
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE org_id=$1 AND id=$2 AND task_id=$3 AND repository_id=$4 AND attempt_id=$5 AND expires_at>clock_timestamp())`, l.OrgID, id, l.TaskID, l.RepositoryID, l.AttemptID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return auth.ErrForbidden
			}
		}

		for _, group := range [][]CheckResult{in.Baseline, in.Candidate, in.Target} {
			for _, check := range group {
				var bound bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE org_id=$1 AND task_id=$2 AND attempt_id=$3 AND id=ANY($4::uuid[]) AND sha256=$5 AND expires_at>clock_timestamp())`, l.OrgID, l.TaskID, l.AttemptID, in.Artifacts, check.OutputSHA256).Scan(&bound); err != nil {
					return err
				}
				if !bound {
					return ErrValidation
				}
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE repair_runs SET state=$3,report=$4,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND state IN ('queued','handoff')`, l.OrgID, l.TaskID, in.State, raw)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrConflict
		}
		if in.State == "handoff" {
			if _, err = tx.Exec(ctx, `UPDATE maintenance_repairs SET active=false WHERE org_id=$1 AND task_id=$2 AND NOT EXISTS(SELECT 1 FROM workflow_outbox WHERE org_id=$1 AND task_id=$2 AND state IN ('dispatching','unknown')) AND NOT EXISTS(SELECT 1 FROM model_turns WHERE org_id=$1 AND task_id=$2 AND state IN ('dispatched','unknown'))`, l.OrgID, l.TaskID); err != nil {
				return err
			}
		}
		out, err = loadRun(ctx, tx, l.OrgID, l.TaskID)
		out.Task = t
		return err
	})
	return out, err
}

func (s *Service) JobRun(ctx context.Context, credential string) (Run, error) {
	var out Run
	err := s.runners.WithJob(ctx, credential, "repair.source", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		var err error
		out, err = loadRun(ctx, tx, l.OrgID, l.TaskID)
		out.Task = t
		return err
	})
	return out, err
}
