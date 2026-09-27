package mergecontrol

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

type Revalidation struct {
	TaskID      string     `json:"task_id"`
	ChangeID    string     `json:"change_id"`
	CompanionID string     `json:"companion_id"`
	State       string     `json:"state"`
	Reason      string     `json:"reason"`
	ObservedAt  *time.Time `json:"observed_at"`
	Gate        *Gate      `json:"gate,omitempty"`
}

func (s *Service) Revalidations(ctx context.Context, session auth.Session, org, repo, change string) ([]Revalidation, error) {
	n, err := strconv.ParseInt(change, 10, 64)
	if !auth.ValidID(repo) || err != nil || n <= 0 || strconv.FormatInt(n, 10) != change {
		return nil, auth.ErrInvalid
	}
	out := []Revalidation{}
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT task_id::text,COALESCE(native_change->>'id',''),bot_revalidation_state,bot_revalidation_reason,bot_observed_at,COALESCE(bot_gate_id::text,'') FROM repair_runs WHERE org_id=$1 AND repository_id=$2 AND context#>>'{finding,evidence,change,id}'=$3 AND state='published' ORDER BY created_at DESC LIMIT 100`, org, repo, change)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			v := Revalidation{ChangeID: change}
			var id string
			if err = rows.Scan(&v.TaskID, &v.CompanionID, &v.State, &v.Reason, &v.ObservedAt, &id); err != nil {
				rows.Close()
				return err
			}
			out = append(out, v)
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for i, id := range ids {
			if id != "" {
				gate, err := gateTx(ctx, tx, org, id)
				if err != nil {
					return err
				}
				out[i].Gate = &gate
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) ObserveBotRepair(ctx context.Context, org, task string) error {
	if !auth.ValidID(org) || !auth.ValidID(task) || s.providers == nil {
		return auth.ErrInvalid
	}
	var repo, connection, changeID, companion, head, headBranch, targetBranch string
	var ref forge.RepoRef
	methods := []string{}
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT repository_id::text,COALESCE(context#>>'{finding,evidence,change,id}',''),COALESCE(native_change->>'id',''),candidate_sha,branch,COALESCE(native_change->>'target_branch','') FROM repair_runs WHERE org_id=$1 AND task_id=$2 AND state='published'`, org, task).Scan(&repo, &changeID, &companion, &head, &headBranch, &targetBranch); err != nil {
			return err
		}
		if changeID == "" || companion == "" || changeID == companion || !source.ValidSHA(head, "sha1") || !strings.HasPrefix(headBranch, "reforge/repair/") || targetBranch == "" {
			return auth.ErrInvalid
		}
		actor, err := s.inspectionActor(ctx, tx, nil, org, repo, task)
		if err != nil {
			return err
		}
		if !manage(actor, repo) {
			return auth.ErrForbidden
		}
		ref, connection, err = repository(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		resolved, err := s.policies.ResolveTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		methods = resolved.Policy.Allow.MergeMethods
		return nil
	})
	if err != nil {
		if repo != "" && errors.Is(err, auth.ErrForbidden) {
			if saveErr := s.saveBotRevalidation(ctx, org, repo, task, changeID, companion, "blocked", "Revalidation author no longer has repository authority", ""); saveErr != nil {
				return saveErr
			}
		}
		return err
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		actor, err := s.inspectionActor(ctx, tx, nil, org, repo, task)
		if err != nil {
			return err
		}
		_, current, err := repository(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if !manage(actor, repo) || current != connection || c.ID != connection {
			return auth.ErrForbidden
		}
		return nil
	}
	state, reason, gateID := "blocked", "Native evidence unavailable; check the connection and revalidation authority", ""
	var result privateconnector.Result
	result, err = s.providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: ref, ChangeID: companion}}, check)
	if err == nil {
		proof, proofErr := companionProof(Companion{TaskID: task, ChangeID: companion, HeadSHA: head, Branch: headBranch, TargetBranch: targetBranch, State: "published"}, result.Change, ref, targetBranch, func(base, head string) (int, error) {
			compared, err := s.providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeBehind, Compare: &privateconnector.CompareArgs{Repository: ref, Base: base, Head: head}}, check)
			if err != nil {
				return 0, err
			}
			if compared.Behind == nil || *compared.Behind < 0 {
				return 0, privateconnector.ErrUnsupported
			}
			return *compared.Behind, nil
		}, func() (bool, error) {
			return s.hasValidatedMergeHead(ctx, org, repo, Companion{ChangeID: companion, Branch: headBranch, TargetBranch: targetBranch}, *result.Change)
		})
		if proofErr != nil {
			err = proofErr
			reason = "Companion changed; refreshed head has no valid Reforge merge proof"
		} else {
			companionSatisfied := proof.State == "merged"
			if !companionSatisfied {
				_, companionSatisfied, err = s.resolveReplacementChain(ctx, org, repo, connection, proof, result.Change, ref, targetBranch, check)
				if err != nil {
					reason = "Replacement lineage changed or is ambiguous"
				}
			}
			if !companionSatisfied && err == nil {
				state, reason = "waiting_companion", "Companion merge is not confirmed or its refreshed head is not validated"
			} else if err == nil {
				var original privateconnector.Result
				original, err = s.providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: ref, ChangeID: changeID}}, check)
				if err == nil && (original.Change == nil || original.Change.ID != changeID || original.Change.Repository != ref || original.Change.TargetBranch != targetBranch) {
					err = auth.ErrConflict
				}
				if err == nil {
					switch original.Change.State {
					case "merged":
						if source.ValidSHA(original.Change.MergeSHA, "sha1") {
							state, reason = "merged", "Original update merged; canonical native outcome observed"
						}
					case "closed":
						state, reason = "closed", "Original update closed"
					default:
						method := "merge"
						if len(methods) > 0 {
							method = methods[0]
						}
						var gate Gate
						gate, err = s.inspect(ctx, nil, org, repo, changeID, method, "", task)
						if err == nil && !slices.Contains(gate.Snapshot.Rules.AllowedMergeMethods, method) {
							for _, candidate := range gate.Snapshot.Rules.AllowedMergeMethods {
								if methods == nil || slices.Contains(methods, candidate) {
									gate, err = s.inspect(ctx, nil, org, repo, changeID, candidate, "", task)
									break
								}
							}
						}
						if err == nil {
							gateID = gate.ID
							reason = "Fresh native checks, reviews or policy still block the original update"
							if gate.Decision.Outcome == "allow" {
								state, reason = "ready", "Original update passed a fresh native merge evaluation; no merge was requested"
							}
						}
					}
				}
			}
		}
	}
	if saveErr := s.saveBotRevalidation(ctx, org, repo, task, changeID, companion, state, reason, gateID); saveErr != nil {
		return saveErr
	}
	return err
}

func (s *Service) saveBotRevalidation(ctx context.Context, org, repo, task, changeID, companion, state, reason, gateID string) error {
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return s.db.Tenant(persist, org, "", func(tx pgx.Tx) error {
		var version int64
		if saveErr := tx.QueryRow(persist, `UPDATE repair_runs SET bot_gate_id=NULLIF($3,'')::uuid,bot_revalidation_state=$4,bot_revalidation_reason=$5,bot_observed_at=now(),bot_observe_after=now()+interval '60 seconds',bot_revalidation_version=bot_revalidation_version+1 WHERE org_id=$1 AND task_id=$2 AND state='published' RETURNING bot_revalidation_version`, org, task, gateID, state, reason).Scan(&version); saveErr != nil {
			return saveErr
		}
		return emit(persist, tx, org, repo, "", "merge.bot_revalidated", task, version, "", map[string]any{"change_id": changeID, "companion_id": companion, "state": state, "gate_id": gateID})
	})
}

func (s *Service) observeNextBotRepair(ctx context.Context, org string) {
	var task string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE repair_runs SET bot_observe_after=now()+interval '3 minutes' WHERE org_id=$1 AND task_id=(SELECT task_id FROM repair_runs WHERE org_id=$1 AND state='published' AND bot_revalidation_state NOT IN ('merged','closed') AND COALESCE(context#>>'{finding,evidence,change,id}','')<>'' AND COALESCE(native_change->>'id','')<>COALESCE(context#>>'{finding,evidence,change,id}','') AND bot_observe_after<=now() ORDER BY bot_observe_after,task_id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING task_id::text`, org).Scan(&task)
	})
	if err != nil {
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_ = s.ObserveBotRepair(bounded, org, task)
}
