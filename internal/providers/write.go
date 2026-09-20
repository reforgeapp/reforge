package providers

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/privateconnector"
	"time"
)

func (s *Service) Write(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) (string, error), validate func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	var result privateconnector.Result
	if !auth.ValidID(org) || !auth.ValidID(id) || op.Validate() != nil || !op.Mutation() || authorize == nil || validate == nil {
		return result, auth.ErrInvalid
	}
	var initial connections.Connection
	if err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var err error
		initial, err = s.connections.MetadataTx(ctx, tx, org, id)
		return err
	}); err != nil {
		return result, err
	}
	if initial.State != "healthy" || (initial.Kind != "forge" && initial.Kind != "delivery") {
		return result, auth.ErrConflict
	}
	dispatch := func(ctx context.Context, ready *privateconnector.Ready, deliver privateconnector.Deliver) error {
		var resolved connections.Resolved
		var authority string
		err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			var locked string
			if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&locked); err != nil {
				return err
			}
			current, err := s.connections.MetadataTx(ctx, tx, org, id)
			if err != nil {
				return err
			}
			if current.Version != initial.Version || current.State != "healthy" || (current.Kind != "forge" && current.Kind != "delivery") {
				return auth.ErrConflict
			}
			runnerID := ""
			if ready != nil {
				if current.Route == nil || current.Route.RevokedAt != nil || current.Route.RunnerID != ready.ID || ready.OrgID != org {
					return auth.ErrForbidden
				}
				if err = s.runners.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash); err != nil {
					return err
				}
				runnerID = ready.ID
			} else if current.Route != nil {
				return auth.ErrConflict
			}
			authority, err = authorize(ctx, tx, current)
			if err != nil {
				return err
			}
			if !auth.ValidID(authority) {
				return auth.ErrInvalid
			}
			resolved, err = s.connections.ResolveTx(ctx, tx, org, id, runnerID)
			if err != nil {
				return err
			}
			return s.resolveProtectionTx(ctx, tx, &resolved, runnerID)
		})
		defer func() {
			closeProtection(&resolved)
			resolved.Secret = ""
			if resolved.Client != nil {
				resolved.Client.CloseIdleConnections()
			}
		}()
		if err != nil {
			return err
		}
		check := func(ctx context.Context) error {
			return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
				var locked string
				if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&locked); err != nil {
					return err
				}
				current, err := s.connections.MetadataTx(ctx, tx, org, id)
				if err != nil {
					return err
				}
				if current.Version != initial.Version || current.State != "healthy" {
					return auth.ErrConflict
				}
				if err = s.checkProtectionTx(ctx, tx, resolved); err != nil {
					return err
				}
				if ready != nil {
					if err = s.runners.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash); err != nil {
						return err
					}
				}
				return validate(ctx, tx, current)
			})
		}
		if ready != nil {
			result, err = deliver(privateconnector.GrantSpec{Check: check, OperationID: op.ID, AuthorityID: authority, RunnerVersion: ready.Version, CredentialHash: ready.CredentialHash, Connection: PrivateConnection(resolved)})
		} else {
			provider, e := s.factory.Forge(ctx, resolved, check)
			err = e
			if err == nil {
				result, err = privateconnector.ReadForge(ctx, provider, op)
			}
		}
		if err != nil {
			return err
		}
		raw, err := json.Marshal(result)
		if err != nil || len(raw) > privateconnector.MaxResponse || exposesCredential(raw, resolved) {
			return privateconnector.ErrUncertain
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var err error
	if initial.Route == nil {
		err = dispatch(ctx, nil, nil)
	} else if s.private == nil {
		err = privateconnector.ErrUnavailable
	} else {
		_, err = s.private.Dispatch(ctx, privateconnector.Target{OrgID: org, RunnerID: initial.Route.RunnerID}, op, func(ctx context.Context, ready privateconnector.Ready, deliver privateconnector.Deliver) error {
			return dispatch(ctx, &ready, deliver)
		})
	}
	if err != nil {
		return privateconnector.Result{}, err
	}
	return result, nil
}
