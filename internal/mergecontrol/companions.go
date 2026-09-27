package mergecontrol

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

func companionsTx(ctx context.Context, tx pgx.Tx, org, repo, change string) ([]Companion, error) {
	rows, err := tx.Query(ctx, `SELECT r.task_id::text,COALESCE(r.native_change->>'id',''),r.candidate_sha,r.state,r.branch,COALESCE(r.native_change->>'target_branch','') FROM repair_runs r JOIN workflow_tasks t ON t.org_id=r.org_id AND t.id=r.task_id WHERE r.org_id=$1 AND r.repository_id=$2 AND r.context#>>'{finding,evidence,change,id}'=$3 AND r.state<>'handoff' AND NOT (r.state='published' AND COALESCE(r.native_change->>'id','')=$3) AND t.state NOT IN ('failed','cancelled') ORDER BY r.task_id LIMIT 101`, org, repo, change)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Companion{}
	for rows.Next() {
		var item Companion
		if err := rows.Scan(&item.TaskID, &item.ChangeID, &item.HeadSHA, &item.State, &item.Branch, &item.TargetBranch); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if len(out) > 100 {
		return nil, auth.ErrConflict
	}
	return out, rows.Err()
}

func (s *Service) inspectCompanions(ctx context.Context, org, repo, connection string, change forge.Change, check func(context.Context, pgx.Tx, connections.Connection) error) ([]Companion, error) {
	var out []Companion
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		out, err = companionsTx(ctx, tx, org, repo, change.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		item := &out[i]
		if item.State != "published" || item.ChangeID == "" || !source.ValidSHA(item.HeadSHA, "sha1") {
			item.State = "repair_pending"
			continue
		}
		result, err := s.providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: change.Repository, ChangeID: item.ChangeID}}, check)
		if err != nil {
			return nil, err
		}
		proof, err := companionProof(*item, result.Change, change.Repository, change.TargetBranch, func(base, head string) (int, error) {
			compared, err := s.providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeBehind, Compare: &privateconnector.CompareArgs{Repository: change.Repository, Base: base, Head: head}}, check)
			if err != nil {
				return 0, err
			}
			if compared.Behind == nil || *compared.Behind < 0 {
				return 0, privateconnector.ErrUnsupported
			}
			return *compared.Behind, nil
		}, func() (bool, error) {
			return s.hasValidatedMergeHead(ctx, org, repo, *item, *result.Change)
		})
		if err != nil {
			return nil, err
		}
		*item = proof
	}
	return out, nil
}

func validateCompanionsTx(ctx context.Context, tx pgx.Tx, org string, gate Gate) (bool, error) {
	current, err := companionsTx(ctx, tx, org, gate.RepositoryID, gate.Snapshot.Change.ID)
	if err != nil {
		return false, err
	}
	return companionsCurrent(current, gate.Companions), nil
}

func companionsCurrent(current, proofs []Companion) bool {
	if len(current) != len(proofs) {
		return false
	}
	for i, item := range current {
		proof := proofs[i]
		observed := proof.ObservedHeadSHA
		if observed == "" {
			observed = proof.HeadSHA
		}
		if item.State != "published" || proof.State != "merged" || !source.ValidSHA(proof.MergeSHA, "sha1") || !source.ValidSHA(observed, "sha1") || item.TaskID != proof.TaskID || item.ChangeID != proof.ChangeID || item.HeadSHA != proof.HeadSHA || item.Branch != proof.Branch || item.TargetBranch != proof.TargetBranch {
			return false
		}
	}
	return true
}

func companionProof(item Companion, fresh *forge.Change, repository forge.RepoRef, targetBranch string, compare func(string, string) (int, error), validatedMerge func() (bool, error)) (Companion, error) {
	if fresh == nil || item.ChangeID == "" || !strings.HasPrefix(item.Branch, "reforge/repair/") || item.TargetBranch == "" || item.TargetBranch != targetBranch || fresh.ID != item.ChangeID || fresh.Repository != repository || fresh.HeadRepository != repository || fresh.TargetRepository != repository || fresh.HeadBranch != item.Branch || fresh.TargetBranch != item.TargetBranch || !source.ValidSHA(item.HeadSHA, "sha1") || !source.ValidSHA(fresh.HeadSHA, "sha1") {
		return item, auth.ErrConflict
	}
	item.ObservedHeadSHA = fresh.HeadSHA
	item.State = "merge_pending"
	item.MergeSHA = ""
	if fresh.State != "merged" || !source.ValidSHA(fresh.MergeSHA, "sha1") {
		return item, nil
	}
	descendant := false
	var compareErr error
	if compare != nil {
		descendant, compareErr = companionHeadDescends(item.HeadSHA, fresh.HeadSHA, compare)
	}
	if !descendant && validatedMerge != nil {
		var err error
		descendant, err = validatedMerge()
		if err != nil {
			return item, err
		}
	}
	if !descendant {
		return item, compareErr
	}
	item.State, item.MergeSHA = "merged", fresh.MergeSHA
	return item, nil
}

func companionHeadDescends(candidate, observed string, compare func(string, string) (int, error)) (bool, error) {
	if !source.ValidSHA(candidate, "sha1") || !source.ValidSHA(observed, "sha1") {
		return false, auth.ErrConflict
	}
	if candidate == observed {
		return true, nil
	}
	candidateBehind, err := compare(candidate, observed)
	if err != nil {
		return false, err
	}
	observedBehind, err := compare(observed, candidate)
	if err != nil {
		return false, err
	}
	return candidateBehind == 0 && observedBehind > 0, nil
}

func (s *Service) hasValidatedMergeHead(ctx context.Context, org, repo string, item Companion, fresh forge.Change) (bool, error) {
	var matched bool
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM merge_operations mo JOIN merge_gates mg ON mg.org_id=mo.org_id AND mg.id=mo.gate_id AND mg.repository_id=mo.repository_id WHERE mo.org_id=$1 AND mo.repository_id=$2 AND mo.change_id=$3 AND mo.target_branch=$4 AND mo.state='merged' AND mo.native_result->>'state'='merged' AND mo.native_result->>'head_sha'=$5 AND mo.native_result->>'merge_sha'=$6 AND mg.change_id=mo.change_id AND mg.document#>>'{decision,outcome}'='allow' AND mg.document#>>'{binding,head}'=$5 AND mg.document#>>'{snapshot,change,head_sha}'=$5 AND mg.document#>>'{snapshot,change,id}'=$3 AND mg.document#>>'{snapshot,change,target_branch}'=$4 AND mg.document#>>'{snapshot,change,head_branch}'=$8 AND mg.document#>>'{snapshot,change,repository,native_id}'=$7 AND mg.document#>>'{snapshot,change,head_repository,native_id}'=$7 AND mg.document#>>'{snapshot,change,target_repository,native_id}'=$7)`, org, repo, item.ChangeID, item.TargetBranch, fresh.HeadSHA, fresh.MergeSHA, fresh.Repository.NativeID, item.Branch).Scan(&matched)
	})
	return matched, err
}
