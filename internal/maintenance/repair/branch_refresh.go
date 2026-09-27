package repair

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
	"reforge/internal/workflow"
)

var errRefreshBusy = errors.New("repair branch has active workflow work")
var ErrBranchConflict = errors.New("repair branch conflicts with current target")

func (s *Service) RefreshFix(ctx context.Context, session auth.Session, org, taskID string) (bool, error) {
	if !auth.ValidID(org) || !auth.ValidID(taskID) {
		return false, auth.ErrInvalid
	}
	t, err := s.workflow.Get(ctx, session, org, taskID)
	if err != nil {
		return false, err
	}
	r, err := s.Get(ctx, session, org, taskID)
	if err != nil {
		return false, err
	}
	change := r.Change
	if err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, t.RepositoryID) {
			return auth.ErrForbidden
		}
		resolved, err := s.policies.ResolveTx(ctx, tx, org, t.RepositoryID)
		if err != nil {
			return err
		}
		if resolved.Paused || resolved.Hash == "" || len(resolved.Problems) != 0 || !refreshPolicyAllows(resolved, t.Recipe) {
			return workflow.ErrPolicy
		}
		return nil
	}); err != nil {
		return false, err
	}
	if r.State != "published" || change == nil || r.Branch == "" || r.Branch != change.HeadBranch || !strings.HasPrefix(r.Branch, "reforge/repair/") {
		return false, nil
	}
	if change.State != "open" && change.State != "opened" {
		return false, nil
	}
	if change.Repository.NativeID == "" || change.Repository.NativeID != r.Context.Repository.NativeID || change.HeadRepository.NativeID != r.Context.Repository.NativeID || change.TargetRepository.NativeID != r.Context.Repository.NativeID || change.ID == "" || change.HeadSHA == "" || change.TargetBranch == "" {
		return false, auth.ErrConflict
	}

	connectionVersion := int64(0)
	validate := func(ctx context.Context, tx pgx.Tx, c connections.Connection, rejectActive bool) (bool, error) {
		actor, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return false, err
		}
		if !manage(actor, t.RepositoryID) {
			return false, auth.ErrForbidden
		}
		var owned bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repair_runs WHERE org_id=$1 AND task_id=$2 AND repository_id=$3 AND branch=$4 AND state='published' AND native_change->>'id'=$5 AND context->>'connection_id'=$6 AND context->'repository'->>'native_id'=$7 AND context->'repository'->>'full_name'=$8)`, org, taskID, t.RepositoryID, r.Branch, change.ID, r.Context.ConnectionID, r.Context.Repository.NativeID, r.Context.Repository.FullName).Scan(&owned); err != nil {
			return false, err
		}
		if !owned || r.Context.ConnectionID != c.ID || c.State != "healthy" || connectionVersion != 0 && connectionVersion != c.Version {
			return false, auth.ErrConflict
		}
		var repoOK bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE org_id=$1 AND id=$2 AND connection_id=$3 AND accessible AND NOT archived AND NOT paused)`, org, t.RepositoryID, c.ID).Scan(&repoOK); err != nil {
			return false, err
		}
		if !repoOK {
			return false, auth.ErrForbidden
		}
		resolved, err := s.policies.ResolveTx(ctx, tx, org, t.RepositoryID)
		if err != nil {
			return false, err
		}
		if resolved.Paused || resolved.Hash == "" || len(resolved.Problems) != 0 || !refreshPolicyAllows(resolved, t.Recipe) {
			return false, workflow.ErrPolicy
		}
		var active bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_tasks wt LEFT JOIN repair_runs rr ON rr.org_id=wt.org_id AND rr.task_id=wt.id WHERE wt.org_id=$1 AND wt.repository_id=$2 AND (wt.state IN ('queued','reproducing','planning','repairing','validating','publishing','reconciling','cancelling') OR EXISTS(SELECT 1 FROM workflow_jobs j WHERE j.org_id=wt.org_id AND j.task_id=wt.id AND (j.state='reconciling' OR j.state='running' AND j.lease_expires_at>clock_timestamp()))) AND (wt.target_branch=$3 OR rr.branch=$3 OR rr.context->>'follow_up_branch'=$3 OR rr.context->>'replaces_branch'=$3))`, org, t.RepositoryID, r.Branch).Scan(&active)
		if err != nil {
			return false, err
		}
		if active && rejectActive {
			return true, errRefreshBusy
		}
		return active, nil
	}
	var waiting bool
	if err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, t.RepositoryID) {
			return auth.ErrForbidden
		}
		c, err := s.connections.MetadataTx(ctx, tx, org, r.Context.ConnectionID)
		if err != nil {
			return err
		}
		connectionVersion = c.Version
		waiting, err = validate(ctx, tx, c, false)
		return err
	}); err != nil {
		return false, err
	}
	if waiting {
		return true, nil
	}
	if _, ok := s.reader.(writer); !ok {
		return false, privateconnector.ErrUnsupported
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		_, err := validate(ctx, tx, c, true)
		return err
	}
	read := func(op privateconnector.Operation) (privateconnector.Result, error) {
		op.ID = domain.NewID()
		return s.reader.Read(ctx, org, r.Context.ConnectionID, op, check)
	}
	fresh, err := read(privateconnector.Operation{Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: r.Context.Repository, ChangeID: change.ID}})
	if err != nil {
		if errors.Is(err, errRefreshBusy) {
			return true, nil
		}
		return false, err
	}
	if fresh.Change == nil || fresh.Change.State != "open" && fresh.Change.State != "opened" {
		return false, nil
	}
	if fresh.Change.ID != change.ID || fresh.Change.HeadBranch != r.Branch || fresh.Change.TargetBranch != change.TargetBranch || fresh.Change.Repository.NativeID != r.Context.Repository.NativeID || fresh.Change.HeadRepository.NativeID != r.Context.Repository.NativeID || fresh.Change.TargetRepository.NativeID != r.Context.Repository.NativeID || !source.ValidSHA(fresh.Change.HeadSHA, "sha1") {
		return false, auth.ErrConflict
	}
	if discovery.RepairConflict(*fresh.Change) {
		return false, ErrBranchConflict
	}
	target, err := read(privateconnector.Operation{Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: r.Context.Repository, Ref: fresh.Change.TargetBranch}})
	if err != nil {
		if errors.Is(err, errRefreshBusy) {
			return true, nil
		}
		return false, err
	}
	if !source.ValidSHA(target.SHA, "sha1") {
		return false, auth.ErrConflict
	}
	behind, err := read(privateconnector.Operation{Kind: privateconnector.ForgeBehind, Compare: &privateconnector.CompareArgs{Repository: r.Context.Repository, Base: target.SHA, Head: fresh.Change.HeadSHA}})
	if err != nil {
		if errors.Is(err, errRefreshBusy) {
			return true, nil
		}
		return false, err
	}
	if behind.Behind == nil || *behind.Behind < 0 {
		return false, privateconnector.ErrUnsupported
	}
	if *behind.Behind == 0 {
		return false, nil
	}
	opID := domain.NewID()
	request := forge.RefreshBranchRequest{Repository: r.Context.Repository, ChangeID: fresh.Change.ID, HeadBranch: fresh.Change.HeadBranch, TargetBranch: fresh.Change.TargetBranch, ExpectedHeadSHA: fresh.Change.HeadSHA, ExpectedTargetSHA: target.SHA, OperationID: opID}
	if !request.Valid() {
		return false, auth.ErrConflict
	}
	w := s.reader.(writer)
	_, err = w.Write(ctx, org, r.Context.ConnectionID, privateconnector.Operation{ID: opID, Kind: privateconnector.ForgeRefreshBranch, Refresh: &request}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) (string, error) {
		if _, err := validate(ctx, tx, c, true); err != nil {
			return "", err
		}
		return opID, nil
	}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		_, err := validate(ctx, tx, c, true)
		return err
	})
	if errors.Is(err, errRefreshBusy) {
		return true, nil
	}
	return err == nil, err
}

func refreshPolicyAllows(resolved policy.Resolved, recipe string) bool {
	for _, action := range resolved.Policy.Deny {
		if action == policy.Publish || action == policy.Merge {
			return false
		}
	}
	if resolved.Policy.Allow.Recipes != nil && !slices.Contains(resolved.Policy.Allow.Recipes, recipe) {
		return false
	}
	return resolved.Policy.Allow.MergeMethods == nil || len(resolved.Policy.Allow.MergeMethods) > 0
}
