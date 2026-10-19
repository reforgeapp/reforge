package mergecontrol

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/source"
)

func companionsTx(ctx context.Context, tx pgx.Tx, org, repo, change string) ([]Companion, error) {
	rows, err := tx.Query(ctx, `SELECT r.task_id::text,r.finding_id::text,COALESCE(r.native_change->>'id',''),r.candidate_sha,r.state,r.branch,COALESCE(r.native_change->>'target_branch',''),COALESCE(r.context#>>'{finding,evidence,change,id}',''),COALESCE(r.context#>>'{finding,evidence,change,head_sha}',''),COALESCE(r.context#>>'{finding,evidence,change,head_branch}',''),COALESCE(r.context#>>'{request,owner}'='true' AND r.context#>>'{plan,owner}'='true',false),COALESCE(r.context->>'replaces_branch','') FROM repair_runs r JOIN workflow_tasks t ON t.org_id=r.org_id AND t.id=r.task_id WHERE r.org_id=$1 AND r.repository_id=$2 AND r.context#>>'{finding,evidence,change,id}'=$3 AND r.state<>'handoff' AND NOT (r.state='published' AND COALESCE(r.native_change->>'id','')=$3) AND t.state NOT IN ('failed','cancelled') ORDER BY r.task_id LIMIT 101`, org, repo, change)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Companion{}
	for rows.Next() {
		var item Companion
		if err := rows.Scan(&item.TaskID, &item.FindingID, &item.ChangeID, &item.HeadSHA, &item.State, &item.Branch, &item.TargetBranch, &item.SourceChangeID, &item.SourceHeadSHA, &item.SourceBranch, &item.OwnerReplacement, &item.ReplacesBranch); err != nil {
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
		if item.State != "merged" {
			chain, terminal, err := s.resolveReplacementChain(ctx, org, repo, connection, *item, result.Change, change.Repository, change.TargetBranch, check)
			if err != nil {
				return nil, err
			}
			if terminal {
				item.Supersession = chain
				item.State = "superseded"
			}
		}
	}
	return out, nil
}

func validateCompanionsTx(ctx context.Context, tx pgx.Tx, org string, gate Gate) (bool, error) {
	current, err := companionsTx(ctx, tx, org, gate.RepositoryID, gate.Snapshot.Change.ID)
	if err != nil {
		return false, err
	}
	valid := companionsCurrent(current, gate.Companions)
	if !valid {
		return false, nil
	}
	for _, proof := range gate.Companions {
		if proof.State != "superseded" {
			continue
		}
		if len(proof.Supersession) == 0 || len(proof.Supersession) > 16 {
			return false, nil
		}
		for i, link := range proof.Supersession {
			if i == 0 && (link.SourceChangeID != proof.ChangeID || link.SourceHeadSHA != proof.ObservedHeadSHA || link.SourceBranch != proof.Branch) {
				return false, nil
			}
			var count int
			var matches bool
			err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(bool_or(r.task_id::text=$7 AND r.finding_id::text=$8 AND r.candidate_sha=$9 AND r.branch=$10 AND r.native_change->>'id'=$11),false) FROM repair_runs r JOIN workflow_tasks t ON t.org_id=r.org_id AND t.id=r.task_id WHERE r.org_id=$1 AND r.repository_id=$2 AND r.state='published' AND t.state NOT IN ('failed','cancelled') AND r.native_change->>'target_branch'=$3 AND r.context#>>'{finding,evidence,change,id}'=$4 AND r.context#>>'{finding,evidence,change,head_sha}'=$5 AND r.context#>>'{finding,evidence,change,head_branch}'=$6 AND r.context#>>'{request,owner}'='true' AND r.context#>>'{plan,owner}'='true' AND r.context->>'replaces_branch'=$6 AND r.branch LIKE 'reforge/repair/%' AND r.branch<>$6`, org, gate.RepositoryID, link.TargetBranch, link.SourceChangeID, link.SourceHeadSHA, link.SourceBranch, link.TaskID, link.FindingID, link.CandidateSHA, link.Branch, link.ChangeID).Scan(&count, &matches)
			if err != nil || count != 1 || !matches || link.SourceChangeID == "" || !source.ValidSHA(link.SourceHeadSHA, "sha1") || !source.ValidSHA(link.ObservedHeadSHA, "sha1") {
				return false, err
			}
			if i > 0 && (link.SourceChangeID != proof.Supersession[i-1].ChangeID || link.SourceHeadSHA != proof.Supersession[i-1].ObservedHeadSHA || link.SourceBranch != proof.Supersession[i-1].Branch) {
				return false, nil
			}
		}
		last := proof.Supersession[len(proof.Supersession)-1]
		matched, err := hasValidatedMergeHeadTx(ctx, tx, org, gate.RepositoryID, Companion{ChangeID: last.ChangeID, Branch: last.Branch, TargetBranch: last.TargetBranch}, forge.Change{Repository: gate.Snapshot.Change.Repository, HeadSHA: last.ObservedHeadSHA, MergeSHA: last.MergeSHA})
		if err != nil || !matched {
			return false, err
		}
		if err := persistCompanionSupersessionTx(ctx, tx, org, gate.RepositoryID, proof); err != nil {
			return false, err
		}
	}
	return true, nil
}

func persistCompanionSupersessionTx(ctx context.Context, tx pgx.Tx, org, repo string, proof Companion) error {
	if proof.State != "superseded" || proof.TaskID == "" || proof.FindingID == "" || len(proof.Supersession) == 0 || len(proof.Supersession) > 16 {
		return auth.ErrConflict
	}
	raw, err := json.Marshal(proof.Supersession)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE maintenance_repairs SET active=false,supersession=$5::jsonb WHERE org_id=$1 AND repository_id=$2 AND task_id=$3 AND finding_id=$4`, org, repo, proof.TaskID, proof.FindingID, raw)
	if err == nil && result.RowsAffected() != 1 {
		err = auth.ErrConflict
	}
	return err
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
		if item.State != "published" || (proof.State != "merged" && proof.State != "superseded") || proof.State == "merged" && !source.ValidSHA(proof.MergeSHA, "sha1") || proof.State == "superseded" && (len(proof.Supersession) == 0 || len(proof.Supersession) > 16) || !source.ValidSHA(observed, "sha1") || item.TaskID != proof.TaskID || item.FindingID != proof.FindingID || item.ChangeID != proof.ChangeID || item.HeadSHA != proof.HeadSHA || item.Branch != proof.Branch || item.TargetBranch != proof.TargetBranch || item.SourceChangeID != proof.SourceChangeID || item.SourceHeadSHA != proof.SourceHeadSHA || item.SourceBranch != proof.SourceBranch || item.OwnerReplacement != proof.OwnerReplacement || item.ReplacesBranch != proof.ReplacesBranch {
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
		var err error
		matched, err = hasValidatedMergeHeadTx(ctx, tx, org, repo, item, fresh)
		return err
	})
	return matched, err
}

func hasValidatedMergeHeadTx(ctx context.Context, tx pgx.Tx, org, repo string, item Companion, fresh forge.Change) (bool, error) {
	var matched bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM merge_operations mo JOIN merge_gates mg ON mg.org_id=mo.org_id AND mg.id=mo.gate_id AND mg.repository_id=mo.repository_id WHERE mo.org_id=$1 AND mo.repository_id=$2 AND mo.change_id=$3 AND mo.target_branch=$4 AND mo.state='merged' AND mo.native_result->>'state'='merged' AND mo.native_result->>'head_sha'=$5 AND mo.native_result->>'merge_sha'=$6 AND mg.change_id=mo.change_id AND mg.document#>>'{decision,outcome}'='allow' AND mg.document#>>'{binding,head}'=$5 AND mg.document#>>'{snapshot,change,head_sha}'=$5 AND mg.document#>>'{snapshot,change,id}'=$3 AND mg.document#>>'{snapshot,change,target_branch}'=$4 AND mg.document#>>'{snapshot,change,head_branch}'=$8 AND mg.document#>>'{snapshot,change,repository,native_id}'=$7 AND mg.document#>>'{snapshot,change,head_repository,native_id}'=$7 AND mg.document#>>'{snapshot,change,target_repository,native_id}'=$7)`, org, repo, item.ChangeID, item.TargetBranch, fresh.HeadSHA, fresh.MergeSHA, fresh.Repository.NativeID, item.Branch).Scan(&matched)
	return matched, err
}

func (s *Service) resolveReplacementChain(ctx context.Context, org, repo, connection string, original Companion, fresh *forge.Change, repository forge.RepoRef, target string, check func(context.Context, pgx.Tx, connections.Connection) error) ([]ReplacementProof, bool, error) {
	if fresh == nil || fresh.ID != original.ChangeID || fresh.HeadBranch != original.Branch || fresh.TargetBranch != target || fresh.Repository != repository || fresh.HeadRepository != repository || fresh.TargetRepository != repository || !source.ValidSHA(fresh.HeadSHA, "sha1") {
		return nil, false, auth.ErrConflict
	}
	chain := []ReplacementProof{}
	seenIDs := map[string]bool{fresh.ID: true}
	seenBranches := map[string]bool{fresh.HeadBranch: true}
	previous := fresh
	for range 16 {
		var links []ReplacementProof
		err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT r.task_id::text,r.finding_id::text,COALESCE(r.native_change->>'id',''),r.candidate_sha,r.branch,COALESCE(r.native_change->>'target_branch',''),COALESCE(r.context#>>'{finding,evidence,change,id}',''),COALESCE(r.context#>>'{finding,evidence,change,head_sha}',''),COALESCE(r.context#>>'{finding,evidence,change,head_branch}','') FROM repair_runs r JOIN workflow_tasks t ON t.org_id=r.org_id AND t.id=r.task_id WHERE r.org_id=$1 AND r.repository_id=$2 AND r.state='published' AND t.state NOT IN ('failed','cancelled') AND r.context#>>'{request,owner}'='true' AND r.context#>>'{plan,owner}'='true' AND r.context->>'replaces_branch'=$3 AND r.context#>>'{finding,evidence,change,id}'=$4 AND r.context#>>'{finding,evidence,change,head_sha}'=$5 AND r.context#>>'{finding,evidence,change,head_branch}'=$3 AND r.branch LIKE 'reforge/repair/%' AND r.branch<>$3 AND r.native_change->>'target_branch'=$6 ORDER BY r.task_id LIMIT 2`, org, repo, previous.HeadBranch, previous.ID, previous.HeadSHA, target)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var link ReplacementProof
				if err := rows.Scan(&link.TaskID, &link.FindingID, &link.ChangeID, &link.CandidateSHA, &link.Branch, &link.TargetBranch, &link.SourceChangeID, &link.SourceHeadSHA, &link.SourceBranch); err != nil {
					return err
				}
				links = append(links, link)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, false, err
		}
		if len(links) == 0 {
			return nil, false, nil
		}
		if len(links) != 1 {
			return nil, false, auth.ErrConflict
		}
		link := links[0]
		if link.ChangeID == "" || !source.ValidSHA(link.CandidateSHA, "sha1") || seenIDs[link.ChangeID] || seenBranches[link.Branch] || link.SourceChangeID != previous.ID || link.SourceHeadSHA != previous.HeadSHA || link.SourceBranch != previous.HeadBranch {
			return nil, false, auth.ErrConflict
		}
		read, err := s.providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: repository, ChangeID: link.ChangeID}}, check)
		if err != nil {
			return nil, false, err
		}
		current := read.Change
		if current == nil || current.ID != link.ChangeID || current.Repository != repository || current.HeadRepository != repository || current.TargetRepository != repository || current.HeadBranch != link.Branch || current.TargetBranch != target || !source.ValidSHA(current.HeadSHA, "sha1") {
			return nil, false, auth.ErrConflict
		}
		link.ObservedHeadSHA = current.HeadSHA
		if current.State == "merged" {
			if !source.ValidSHA(current.MergeSHA, "sha1") {
				return nil, false, nil
			}
			valid, err := s.hasValidatedMergeHead(ctx, org, repo, Companion{ChangeID: link.ChangeID, Branch: link.Branch, TargetBranch: target}, *current)
			if err != nil || !valid {
				return nil, false, err
			}
			link.MergeSHA = current.MergeSHA
			chain = append(chain, link)
			return chain, true, nil
		}
		if current.State != "open" && current.State != "opened" {
			return nil, false, nil
		}
		chain = append(chain, link)
		seenIDs[current.ID], seenBranches[current.HeadBranch] = true, true
		previous = current
	}
	return nil, false, nil
}
