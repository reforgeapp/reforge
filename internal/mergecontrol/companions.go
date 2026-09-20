package mergecontrol

import (
	"context"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

func companionsTx(ctx context.Context, tx pgx.Tx, org, repo, change string) ([]Companion, error) {
	rows, err := tx.Query(ctx, `SELECT r.task_id::text,COALESCE(r.native_change->>'id',''),r.candidate_sha,r.state FROM repair_runs r JOIN workflow_tasks t ON t.org_id=r.org_id AND t.id=r.task_id WHERE r.org_id=$1 AND r.repository_id=$2 AND r.context#>>'{finding,evidence,change,id}'=$3 AND r.state<>'handoff' AND t.state NOT IN ('failed','cancelled') ORDER BY r.task_id LIMIT 101`, org, repo, change)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Companion{}
	for rows.Next() {
		var item Companion
		if err := rows.Scan(&item.TaskID, &item.ChangeID, &item.HeadSHA, &item.State); err != nil {
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
		if result.Change == nil || result.Change.ID != item.ChangeID || result.Change.Repository != change.Repository || result.Change.HeadSHA != item.HeadSHA || result.Change.TargetBranch != change.TargetBranch {
			return nil, auth.ErrConflict
		}
		item.State = "merge_pending"
		if result.Change.State == "merged" && source.ValidSHA(result.Change.MergeSHA, "sha1") {
			item.State, item.MergeSHA = "merged", result.Change.MergeSHA
		}
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
		if item.State != "published" || proof.State != "merged" || !source.ValidSHA(proof.MergeSHA, "sha1") || item.TaskID != proof.TaskID || item.ChangeID != proof.ChangeID || item.HeadSHA != proof.HeadSHA {
			return false
		}
	}
	return true
}
