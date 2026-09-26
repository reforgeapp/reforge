package modelbroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/model"
	"reforge/internal/network"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
	"reforge/internal/runner"
	"reforge/internal/secrets"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

var ErrUncertain = errors.New("model turn outcome requires reconciliation; allowance remains held")
var ErrUnavailable = errors.New("configured model route is not available for this runner")

type Service struct {
	db                   *store.Store
	runners              *runner.Service
	connections          *connections.Service
	budgets              *budget.Service
	private              *privateconnector.Connector
	vault                *secrets.Vault
	factory              providers.Factory
	AuthorizeReservation func(context.Context, pgx.Tx, workflow.Task, budget.Reservation) error
}

func New(db *store.Store, runners *runner.Service, connections *connections.Service, budgets *budget.Service, private *privateconnector.Connector, vault *secrets.Vault, development bool) *Service {
	return &Service{db: db, runners: runners, connections: connections, budgets: budgets, private: private, vault: vault, factory: providers.Factory{Development: development}}
}
func (s *Service) settleOrphansTx(ctx context.Context, tx pgx.Tx, l workflow.Lease) error {
	rows, err := tx.Query(ctx, `SELECT id::text,reservation_id::text FROM model_turns WHERE org_id=$1 AND task_id=$2 AND attempt_id<>$3 AND state IN ('dispatched','unknown')`, l.OrgID, l.TaskID, l.AttemptID)
	if err != nil {
		return err
	}
	turns, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ ID, Reservation string }])
	if err != nil {
		return err
	}
	for _, turn := range turns {
		if err = s.budgets.SettleUnknownAtMaximumTx(ctx, tx, l.OrgID, turn.Reservation, "orphaned-attempt:"+turn.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE model_turns SET state='failed',completed_at=coalesce(completed_at,clock_timestamp()) WHERE org_id=$1 AND id=$2`, l.OrgID, turn.ID); err != nil {
			return err
		}
	}
	return nil
}

func binding(org, id string) secrets.Binding {
	return secrets.Binding{OrgID: org, ConnectionID: id, Version: 1}
}
func (s *Service) existing(ctx context.Context, tx pgx.Tx, l workflow.Lease, id, hash string) (*model.TurnResult, error) {
	var state, oldHash, task, attempt string
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT state,request_hash,task_id::text,attempt_id::text,result_envelope FROM model_turns WHERE org_id=$1 AND id=$2`, l.OrgID, id).Scan(&state, &oldHash, &task, &attempt, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if oldHash != hash || task != l.TaskID || attempt != l.AttemptID {
		return nil, auth.ErrConflict
	}
	if state != "complete" {
		return nil, ErrUncertain
	}
	var envelope secrets.Envelope
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	data, err := s.vault.OpenRecord(ctx, binding(l.OrgID, id), envelope)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	var result model.TurnResult
	err = json.Unmarshal(data, &result)
	return &result, err
}
func (s *Service) Turn(ctx context.Context, credential string, in model.Turn) (model.TurnResult, error) {
	var result model.TurnResult
	if s.AuthorizeReservation == nil {
		return result, ErrUnavailable
	}
	if !auth.ValidID(in.OperationID) || !in.Valid() {
		return result, auth.ErrInvalid
	}
	in, err := in.WithSkills()
	if err != nil {
		return result, auth.ErrInvalid
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	var initial connections.Connection
	var lease workflow.Lease
	var cached *model.TurnResult
	err = s.runners.WithJob(ctx, credential, "model.turn", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
		lease = l
		var err error
		initial, err = s.connections.MetadataTx(ctx, tx, l.OrgID, t.ModelConnectionID)
		if err != nil {
			return err
		}
		if initial.Kind != "model" || initial.State != "healthy" || initial.Settings.Model != in.Model || initial.Settings.BillingRoute != "direct_api" {
			return ErrUnavailable
		}
		cached, err = s.existing(ctx, tx, l, in.OperationID, hash)
		if err == nil && cached == nil {
			err = s.settleOrphansTx(ctx, tx, l)
		}
		if err == nil && cached == nil {
			var unresolved bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM model_turns WHERE org_id=$1 AND task_id=$2 AND state IN ('dispatched','unknown'))`, l.OrgID, l.TaskID).Scan(&unresolved)
			if err == nil && unresolved {
				return ErrUncertain
			}
		}
		return err
	})
	if err != nil {
		return result, err
	}
	if cached != nil {
		return *cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutMS)*time.Millisecond)
	defer cancel()
	var reservation budget.Reservation
	var resolved connections.Resolved
	dispatched := false
	prepare := func(ctx context.Context, ready *privateconnector.Ready) error {
		return s.runners.WithJob(ctx, credential, "model.turn", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
			current, err := s.connections.MetadataTx(ctx, tx, l.OrgID, t.ModelConnectionID)
			if err != nil {
				return err
			}
			if current.ID != initial.ID || current.Version != initial.Version || current.State != "healthy" {
				return auth.ErrConflict
			}
			old, err := s.existing(ctx, tx, l, in.OperationID, hash)
			if err != nil {
				return err
			}
			if old != nil {
				return auth.ErrConflict
			}
			var unresolved bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM model_turns WHERE org_id=$1 AND task_id=$2 AND state IN ('dispatched','unknown'))`, l.OrgID, l.TaskID).Scan(&unresolved); err != nil {
				return err
			}
			if unresolved {
				return ErrUncertain
			}
			runnerID := ""
			if ready != nil {
				if current.Route == nil || current.Route.RevokedAt != nil || current.Route.RunnerID != ready.ID || ready.ID != l.WorkerID || ready.OrgID != l.OrgID {
					return auth.ErrForbidden
				}
				if err = s.runners.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash); err != nil {
					return err
				}
				runnerID = ready.ID
			} else if current.Route != nil {
				return ErrUnavailable
			}
			route, err := s.budgets.RouteTx(ctx, tx, l.OrgID, current.ID, in.Model, t.ModelRoute)
			if errors.Is(err, pgx.ErrNoRows) {
				return budget.ErrUnknown
			}
			if err != nil {
				return err
			}
			if route.Mode != "priced" {
				return ErrUnavailable
			}
			quote := budget.Quote{OperationID: in.OperationID, Model: in.Model, Route: t.ModelRoute, RouteVersion: route.Version, InputTokens: int64(len(raw)) + 4096, MaxOutputTokens: int64(in.MaxOutputTokens), MaxMilliseconds: in.TimeoutMS, MaxRequests: 1}
			reservation, err = s.budgets.ReserveTx(ctx, tx, budget.Lease(l), quote)
			if err != nil {
				return err
			}
			if s.AuthorizeReservation != nil {
				if err = s.AuthorizeReservation(ctx, tx, t, reservation); err != nil {
					return err
				}
			}
			resolved, err = s.connections.ResolveTx(ctx, tx, l.OrgID, current.ID, runnerID)
			if err != nil {
				return err
			}
			reservation, err = s.budgets.MarkDispatchedTx(ctx, tx, budget.Lease(l), reservation.ID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO model_turns(org_id,id,task_id,attempt_id,repository_id,reservation_id,request_hash,state) VALUES($1,$2,$3,$4,$5,$6,$7,'dispatched')`, l.OrgID, in.OperationID, l.TaskID, l.AttemptID, l.RepositoryID, reservation.ID, hash)
			return err
		})
	}
	defer func() {
		resolved.Secret = ""
		if resolved.Client != nil {
			resolved.Client.CloseIdleConnections()
		}
	}()
	started := time.Now()
	execute := func(callctx context.Context, fn func(context.Context) (model.TurnResult, error)) error {
		dispatched = true
		guarded, stop := context.WithCancel(callctx)
		defer stop()
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-guarded.Done():
					return
				case <-ticker.C:
					checkCtx, cancelCheck := context.WithTimeout(guarded, 2*time.Second)
					err := s.runners.WithJob(checkCtx, credential, "model.turn", func(tx pgx.Tx, l workflow.Lease, t workflow.Task) error {
						current, err := s.connections.MetadataTx(checkCtx, tx, l.OrgID, t.ModelConnectionID)
						if err == nil && (current.Version != initial.Version || current.State != "healthy") {
							err = auth.ErrConflict
						}
						return err
					})
					cancelCheck()
					if err != nil && guarded.Err() == nil && !errors.Is(err, context.DeadlineExceeded) {
						slog.WarnContext(ctx, "model turn authority lost", "org_id", lease.OrgID, "operation_id", in.OperationID, "error", err)
						stop()
						cancel()
						return
					}
				}
			}
		}()
		var err error
		result, err = fn(guarded)
		stop()
		<-done
		return err
	}
	if initial.Route != nil {
		if s.private == nil || initial.Route.RunnerID != lease.WorkerID {
			return result, ErrUnavailable
		}
		_, err = s.private.Dispatch(ctx, privateconnector.Target{OrgID: lease.OrgID, RunnerID: lease.WorkerID}, privateconnector.Operation{ID: in.OperationID, Kind: privateconnector.ModelTurn, Turn: &in}, func(callctx context.Context, ready privateconnector.Ready, deliver privateconnector.Deliver) error {
			if err := prepare(callctx, &ready); err != nil {
				return err
			}
			return execute(callctx, func(context.Context) (model.TurnResult, error) {
				received, err := deliver(privateconnector.GrantSpec{OperationID: in.OperationID, AuthorityID: reservation.ID, RunnerVersion: ready.Version, CredentialHash: ready.CredentialHash, Connection: providers.PrivateConnection(resolved)})
				if err != nil {
					return model.TurnResult{}, err
				}
				if received.Turn == nil {
					return model.TurnResult{}, ErrUncertain
				}
				return *received.Turn, nil
			})
		})
	} else {
		if err = prepare(ctx, nil); err == nil {
			err = execute(ctx, func(callctx context.Context) (model.TurnResult, error) {
				scoped, err := network.WithTimeout(resolved.Client, time.Duration(in.TimeoutMS)*time.Millisecond)
				if err != nil {
					return model.TurnResult{}, err
				}
				defer scoped.CloseIdleConnections()
				resolved.Client = scoped
				provider, err := s.factory.Model(resolved)
				if err != nil {
					return model.TurnResult{}, err
				}
				in.Session = lease.AttemptID
				return model.CollectTurn(callctx, provider, in)
			})
		}
	}
	if !dispatched {
		return model.TurnResult{}, err
	}
	finalctx, stopFinal := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stopFinal()
	body, marshalErr := json.Marshal(result)
	var providerError *domain.ProviderError
	rejected := errors.As(err, &providerError) && !providerError.Uncertain
	known := err == nil && marshalErr == nil && len(body) <= 3<<20 && result.Usage.Known && result.Usage.CacheTokens >= 0 && result.Usage.CacheCreationTokens >= 0 && !privateconnector.ContainsSecret(body, resolved.Secret)
	amount, amountErr := budget.ObservedAmount(reservation.Route, result.Usage.InputTokens, result.Usage.OutputTokens, time.Since(started).Milliseconds())
	known = known && amountErr == nil
	if !known && !rejected {
		reason := "usage unknown or response rejected"
		if err != nil && !privateconnector.ContainsSecret([]byte(err.Error()), resolved.Secret) {
			reason = err.Error()
		}
		slog.WarnContext(ctx, "model turn outcome unknown", "org_id", lease.OrgID, "operation_id", in.OperationID, "model", in.Model, "reason", reason)
	}
	persistErr := s.db.Tenant(finalctx, lease.OrgID, "", func(tx pgx.Tx) error {
		if rejected {
			if _, e := s.budgets.SettleTx(finalctx, tx, lease.OrgID, reservation.ID, budget.Settlement{Known: true, Reference: "model-turn:" + in.OperationID}); e != nil {
				return e
			}
			_, e := tx.Exec(finalctx, `UPDATE model_turns SET state='failed',completed_at=clock_timestamp() WHERE org_id=$1 AND id=$2 AND state='dispatched'`, lease.OrgID, in.OperationID)
			return e
		}
		if !known {
			if _, e := s.budgets.MarkUnknownTx(finalctx, tx, lease.OrgID, reservation.ID, "model-turn:"+in.OperationID); e != nil {
				return e
			}
			changed, e := tx.Exec(finalctx, `UPDATE model_turns SET state='unknown',completed_at=clock_timestamp() WHERE org_id=$1 AND id=$2 AND state='dispatched'`, lease.OrgID, in.OperationID)
			if e == nil && changed.RowsAffected() != 1 {
				return auth.ErrConflict
			}
			return e
		}
		envelope, e := s.vault.SealRecord(finalctx, binding(lease.OrgID, in.OperationID), body)
		if e != nil {
			return e
		}
		encrypted, e := json.Marshal(envelope)
		if e != nil {
			return e
		}
		usage, _ := json.Marshal(result.Usage)
		if _, e = s.budgets.SettleTx(finalctx, tx, lease.OrgID, reservation.ID, budget.Settlement{Known: true, Actual: amount, Reference: "model-turn:" + in.OperationID}); e != nil {
			return e
		}
		changed, e := tx.Exec(finalctx, `UPDATE model_turns SET state='complete',result_envelope=$3,usage=$4,completed_at=clock_timestamp() WHERE org_id=$1 AND id=$2 AND state='dispatched'`, lease.OrgID, in.OperationID, encrypted, usage)
		if e == nil && changed.RowsAffected() != 1 {
			return auth.ErrConflict
		}
		return e
	})
	if rejected && persistErr == nil {
		message := providerError.Message
		if privateconnector.ContainsSecret([]byte(message), resolved.Secret) {
			message = ""
		}
		slog.WarnContext(ctx, "model turn rejected by provider", "org_id", lease.OrgID, "operation_id", in.OperationID, "model", in.Model, "kind", providerError.Kind, "reason", message)
		return model.TurnResult{}, err
	}
	if persistErr == nil && known {
		if err := s.db.Tenant(finalctx, lease.OrgID, "", func(tx pgx.Tx) error {
			_, e := tx.Exec(finalctx, `UPDATE connections SET key_confirmed_at=clock_timestamp() WHERE org_id=$1 AND id=$2 AND (key_confirmed_at IS NULL OR key_confirmed_at<clock_timestamp()-interval '1 hour')`, lease.OrgID, reservation.ConnectionID)
			return e
		}); err != nil {
			slog.WarnContext(ctx, "model key confirmation not recorded", "org_id", lease.OrgID, "error", err)
		}
	}
	if persistErr != nil {
		slog.ErrorContext(ctx, "model turn result could not be recorded", "org_id", lease.OrgID, "operation_id", in.OperationID, "error", persistErr)
	}
	if persistErr != nil || !known {
		return model.TurnResult{}, ErrUncertain
	}
	if err = s.runners.WithJob(finalctx, credential, "model.turn", func(pgx.Tx, workflow.Lease, workflow.Task) error { return nil }); err != nil {
		return model.TurnResult{}, err
	}
	return result, nil
}
