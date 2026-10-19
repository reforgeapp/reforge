package repair

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/sandbox"
	"github.com/reforgeapp/reforge/internal/sandbox/guest"
	"github.com/reforgeapp/reforge/internal/source"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type recoveryIntent struct {
	ID          string
	OperationID string
	Kind        string
	State       string
	Evidence    string
	UpdatedAt   time.Time
}

func recoveryReady(ctx context.Context, tx pgx.Tx, org, task string) error {
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_jobs WHERE org_id=$1 AND task_id=$2 AND state='running' AND lease_expires_at>clock_timestamp())`, org, task).Scan(&active); err != nil {
		return err
	}
	if active {
		return workflow.ErrReconciliation
	}
	return nil
}
func stageMatches(r Run, proof forge.CommitProof, operation string, baseline, candidate sandbox.Snapshot) bool {
	if r.Report == nil || proof.SHA != candidate.CommitSHA || len(proof.Parents) != 1 || proof.Parents[0] != r.Context.Plan.TargetSHA || baseline.CommitSHA != r.Context.Plan.TargetSHA {
		return false
	}
	if !strings.HasSuffix(strings.TrimSpace(proof.Message), "[reforge-operation:"+operation+"]") && !strings.HasSuffix(strings.TrimSpace(proof.Message), "<!-- reforge-operation-id:"+operation+" -->") {
		return false
	}
	if _, err := Files(baseline); err != nil {
		return false
	}
	if _, err := Files(candidate); err != nil {
		return false
	}
	expected := map[string]guest.File{}
	for _, f := range baseline.Files {
		expected[f.Path] = f
	}
	for _, patch := range r.Report.Patches {
		if patch.Delete {
			return false
		}
		f := expected[patch.Path]
		f.Path = patch.Path
		f.Content = patch.Content
		expected[patch.Path] = f
	}
	if len(expected) != len(candidate.Files) {
		return false
	}
	for _, f := range candidate.Files {
		want, ok := expected[f.Path]
		if !ok || want.Executable != f.Executable || want.Delete != f.Delete || !bytes.Equal(want.Content, f.Content) {
			return false
		}
	}
	return true
}
func (s *Service) Reconcile(ctx context.Context, session auth.Session, org, id string, expected int64, request string) (Run, error) {
	var out Run
	if !auth.ValidID(id) || expected < 1 {
		return out, auth.ErrInvalid
	}
	r, err := s.Get(ctx, session, org, id)
	if err != nil {
		return out, err
	}
	if r.Version != expected {
		return out, auth.ErrConflict
	}
	if err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, r.Task.RepositoryID) {
			return auth.ErrForbidden
		}
		if err := recoveryReady(ctx, tx, org, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text,reservation_id::text FROM model_turns m WHERE org_id=$1 AND task_id=$2 AND (state='unknown' OR state='dispatched' AND EXISTS(SELECT 1 FROM workflow_tasks t WHERE t.org_id=m.org_id AND t.id=m.task_id AND t.state IN ('failed','cancelled')))`, org, id)
		if err != nil {
			return err
		}
		turns, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ ID, Reservation string }])
		if err != nil {
			return err
		}
		for _, turn := range turns {
			if err = s.budgets.SettleUnknownAtMaximumTx(ctx, tx, org, turn.Reservation, "reconciled-at-maximum:"+turn.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE model_turns SET state='failed',completed_at=coalesce(completed_at,clock_timestamp()) WHERE org_id=$1 AND id=$2`, org, turn.ID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE maintenance_repairs SET active=false WHERE org_id=$1 AND task_id=$2 AND EXISTS(SELECT 1 FROM workflow_tasks WHERE org_id=$1 AND id=$2 AND state IN ('failed','cancelled')) AND NOT EXISTS(SELECT 1 FROM workflow_outbox WHERE org_id=$1 AND task_id=$2 AND state IN ('dispatching','unknown')) AND NOT EXISTS(SELECT 1 FROM model_turns WHERE org_id=$1 AND task_id=$2 AND state IN ('dispatched','unknown'))`, org, id)
		return err
	}); err != nil {
		return out, err
	}
	var intents []recoveryIntent
	authorize := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		actor, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !manage(actor, r.Task.RepositoryID) {
			return auth.ErrForbidden
		}
		if c.ID != r.Context.ConnectionID || c.Version != r.Context.ConnectionVersion {
			return auth.ErrConflict
		}
		return recoveryReady(ctx, tx, org, id)
	}
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, r.Task.RepositoryID) {
			return auth.ErrForbidden
		}
		if err := recoveryReady(ctx, tx, org, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text,operation_id::text,kind,state,evidence,updated_at FROM workflow_outbox WHERE org_id=$1 AND task_id=$2 AND kind IN ('stage','publish') ORDER BY CASE kind WHEN 'stage' THEN 0 ELSE 1 END`, org, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var i recoveryIntent
			if err = rows.Scan(&i.ID, &i.OperationID, &i.Kind, &i.State, &i.Evidence, &i.UpdatedAt); err != nil {
				return err
			}
			intents = append(intents, i)
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for _, intent := range intents {
		if intent.State == "pending" || intent.State == "absent" {
			continue
		}
		if intent.State != "succeeded" && time.Since(intent.UpdatedAt) < 2*time.Minute {
			return out, workflow.ErrReconciliation
		}
		outcome, evidence := "unknown", "Native outcome remains unproven"
		var candidate string
		var change *forge.Change
		if intent.Kind == "stage" {
			branch := stageBranch(r, id)
			ref, readErr := s.reader.Read(ctx, org, r.Context.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: r.Context.Repository, Ref: branch}}, authorize)
			var providerError *domain.ProviderError
			if errors.As(readErr, &providerError) && providerError.Kind == "not_found" && intent.State != "succeeded" {
				outcome, evidence = "absent", "App-owned branch absent after write expiry"
			} else if readErr != nil {
				return out, readErr
			} else if r.Context.FollowUpBranch != "" && ref.SHA == r.Context.Plan.TargetSHA && intent.State != "succeeded" {
				outcome, evidence = "absent", "Follow-up branch unchanged after write expiry"
			} else {
				proof, err := s.reader.Read(ctx, org, r.Context.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeCommitProof, Commit: &privateconnector.ChecksArgs{Repository: r.Context.Repository, CommitSHA: ref.SHA}}, authorize)
				if err != nil {
					return out, err
				}
				reader := s.reader.SourceReader(org, r.Context.ConnectionID, authorize)
				baseline, err := source.Fetch(ctx, reader, r.Context.Repository, r.Context.Plan.TargetSHA)
				if err != nil {
					return out, err
				}
				native, err := source.Fetch(ctx, reader, r.Context.Repository, ref.SHA)
				if err != nil {
					return out, err
				}
				if proof.Commit == nil || !stageMatches(r, *proof.Commit, intent.OperationID, baseline, native) {
					return out, workflow.ErrReconciliation
				}
				candidate = ref.SHA
				outcome, evidence = "succeeded", candidate
			}
		} else {
			result, err := s.reader.Read(ctx, org, r.Context.ConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeFindChange, Find: &privateconnector.FindChangeArgs{Repository: r.Context.Repository, OperationID: intent.OperationID, HeadBranch: r.Branch, TargetBranch: r.Context.Finding.Evidence.TargetBranch}}, authorize)
			if err != nil {
				return out, err
			}
			change = result.Change
			if change == nil && intent.State != "succeeded" {
				outcome, evidence = "absent", "Complete native change search found no operation after write expiry"
			} else if change != nil && r.Report != nil && change.OperationID == intent.OperationID && change.HeadSHA == r.CandidateSHA && change.HeadBranch == r.Branch && change.TargetBranch == r.Context.Finding.Evidence.TargetBranch && change.HeadRepository.NativeID == r.Context.Repository.NativeID && change.TargetRepository.NativeID == r.Context.Repository.NativeID && Verified(r.Context.Plan, r.Report.Baseline, r.CandidateChecks) {
				outcome, evidence = "succeeded", change.ID
			} else {
				return out, workflow.ErrReconciliation
			}
		}
		err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			if !manage(a, r.Task.RepositoryID) {
				return auth.ErrForbidden
			}
			if err := recoveryReady(ctx, tx, org, id); err != nil {
				return err
			}
			current, err := loadRun(ctx, tx, org, id)
			if err != nil {
				return err
			}
			if current.Version != r.Version {
				return auth.ErrConflict
			}
			connection, err := s.connections.MetadataTx(ctx, tx, org, r.Context.ConnectionID)
			if err != nil {
				return err
			}
			if connection.Version != r.Context.ConnectionVersion {
				return auth.ErrConflict
			}
			if _, err = s.workflow.ReconcileIntentTx(ctx, tx, org, id, intent.ID, outcome, evidence); err != nil {
				return err
			}
			if candidate != "" {
				if _, err = tx.Exec(ctx, `UPDATE repair_runs SET candidate_sha=$3,branch=$4,state='publishing',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2 AND (candidate_sha='' OR candidate_sha=$3)`, org, id, candidate, stageBranch(r, id)); err != nil {
					return err
				}
			}
			if change != nil && outcome == "succeeded" {
				raw, _ := json.Marshal(change)
				if _, err = tx.Exec(ctx, `UPDATE repair_runs SET native_change=$3,state='published',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND task_id=$2`, org, id, raw); err != nil {
					return err
				}
				if err = s.workflow.ConcludeReconciledTx(ctx, tx, org, id, a.UserID, request); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return out, err
		}
		r, err = s.Get(ctx, session, org, id)
		if err != nil {
			return out, err
		}
	}
	return r, nil
}
