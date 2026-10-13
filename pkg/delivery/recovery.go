package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/gitops"
)

type RestorePlan struct {
	Kind        string    `json:"kind"`
	Environment string    `json:"environment"`
	PreviousAt  time.Time `json:"previous_at"`
	GateID      string    `json:"gate_id"`
	ExpiresAt   time.Time `json:"expires_at"`
	Limits      []string  `json:"limits"`
	Blockers    []string  `json:"blockers"`
}

func (s *Service) PreviewRestore(ctx context.Context, session auth.Session, org, kind, id string, expected int64, request string) (RestorePlan, error) {
	out := RestorePlan{Kind: kind, Limits: []string{"Restores application release only; database contents remain unchanged.", "Native approvals and protected merge still apply."}, Blockers: []string{}}
	if !auth.ValidID(id) || expected < 1 {
		return out, auth.ErrInvalid
	}
	if kind == "native" {
		if s.Native == nil {
			return out, &domain.ProviderError{Kind: "unsupported", Message: "Native recovery is unavailable"}
		}
		failed, err := s.Native.Get(ctx, session, org, id)
		if err != nil {
			return out, err
		}
		if failed.Operation.Version != expected {
			return out, auth.ErrConflict
		}
		var known string
		err = s.Releases.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			if !auth.CanReadRepository(a, failed.Operation.RepositoryID) {
				return auth.ErrForbidden
			}
			return tx.QueryRow(ctx, `SELECT id::text FROM deployments WHERE org_id=$1 AND repository_id=$2 AND environment=$3 AND state IN ('healthy','recovered') AND created_at<$4 ORDER BY created_at DESC,id DESC LIMIT 1`, org, failed.Operation.RepositoryID, failed.Operation.Environment, failed.Operation.CreatedAt).Scan(&known)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &domain.ProviderError{Kind: "unknown", Message: "No previous working release in this environment"}
		}
		if err != nil {
			return out, err
		}
		previous, err := s.Native.Get(ctx, session, org, known)
		if err != nil {
			return out, err
		}
		restore := previous.Gate.Request
		restore.RecoveryOf, restore.RestoreDeploymentID = id, known
		gate, err := s.Native.Preview(ctx, session, org, failed.Operation.Environment, restore, request)
		if err != nil {
			return out, err
		}
		out.Environment, out.PreviousAt = failed.Operation.Environment, previous.Operation.CreatedAt
		out.GateID, out.ExpiresAt, out.Blockers = gate.ID, gate.ExpiresAt, gate.Blockers
		return out, nil
	}
	if kind != "gitops" {
		return out, auth.ErrInvalid
	}
	if s.GitOps == nil {
		return out, &domain.ProviderError{Kind: "unsupported", Message: "GitOps recovery is unavailable"}
	}
	failed, err := s.GitOps.Get(ctx, session, org, id)
	if err != nil {
		return out, err
	}
	if failed.Promotion.Version != expected {
		return out, auth.ErrConflict
	}
	var known string
	err = s.Releases.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, failed.Promotion.SourceRepositoryID) || !auth.CanReadRepository(a, failed.Promotion.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		return tx.QueryRow(ctx, `SELECT id::text FROM gitops_promotions WHERE org_id=$1 AND source_repository_id=$2 AND delivery_repository_id=$3 AND environment=$4 AND state IN ('healthy','recovered') AND created_at<$5 ORDER BY created_at DESC,id DESC LIMIT 1`, org, failed.Promotion.SourceRepositoryID, failed.Promotion.DeliveryRepositoryID, failed.Promotion.Environment, failed.Promotion.CreatedAt).Scan(&known)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, &domain.ProviderError{Kind: "unknown", Message: "No previous working release in this environment"}
	}
	if err != nil {
		return out, err
	}
	previous, err := s.GitOps.Get(ctx, session, org, known)
	if err != nil {
		return out, err
	}
	restore := previous.Gate.Request
	restore.RecoveryOf, restore.RestorePromotionID = id, known
	gate, err := s.GitOps.Preview(ctx, session, org, failed.Promotion.Environment, restore, request)
	if err != nil {
		return out, err
	}
	out.Environment, out.PreviousAt = failed.Promotion.Environment, previous.Promotion.CreatedAt
	out.GateID, out.ExpiresAt, out.Blockers = gate.ID, gate.ExpiresAt, gate.Blockers
	return out, nil
}

func (s *Service) Restore(ctx context.Context, session auth.Session, org, kind, id string, expected int64, request string) (Result, error) {
	plan, err := s.PreviewRestore(ctx, session, org, kind, id, expected, request)
	if err != nil {
		return Result{Kind: kind}, err
	}
	if len(plan.Blockers) > 0 {
		return Result{Kind: kind}, &domain.ProviderError{Kind: "unknown", Message: strings.Join(plan.Blockers, "; ")}
	}
	return s.ConfirmRestore(ctx, session, org, kind, id, plan.GateID, expected, request)
}

func (s *Service) ConfirmRestore(ctx context.Context, session auth.Session, org, kind, id, gateID string, expected int64, request string) (Result, error) {
	out := Result{Kind: kind}
	if !auth.ValidID(id) || !auth.ValidID(gateID) || expected < 1 {
		return out, auth.ErrInvalid
	}
	var known string
	if kind == "native" && s.Native != nil {
		failed, err := s.Native.Get(ctx, session, org, id)
		if err != nil {
			return out, err
		}
		if failed.Operation.Version != expected {
			return out, auth.ErrConflict
		}
		err = s.Releases.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
			var raw []byte
			if err := tx.QueryRow(ctx, `SELECT document FROM deployment_gates WHERE org_id=$1 AND id=$2`, org, gateID).Scan(&raw); err != nil {
				return err
			}
			var gate deployment.Gate
			if err := json.Unmarshal(raw, &gate); err != nil {
				return err
			}
			if gate.Request.RecoveryOf != id || gate.RepositoryID != failed.Operation.RepositoryID || !auth.CanReadRepository(a, gate.RepositoryID) {
				return auth.ErrForbidden
			}
			known = gate.Request.RestoreDeploymentID
			return nil
		})
		if err != nil {
			return out, err
		}
		operation, err := s.Native.Request(ctx, session, org, gateID, domain.StableID("restore-working-release", org, kind, id, known), request)
		out.ID, out.State = operation.ID, operation.State
		return out, err
	}
	if kind != "gitops" || s.GitOps == nil {
		return out, auth.ErrInvalid
	}
	failed, err := s.GitOps.Get(ctx, session, org, id)
	if err != nil {
		return out, err
	}
	if failed.Promotion.Version != expected {
		return out, auth.ErrConflict
	}
	err = s.Releases.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT document FROM gitops_gates WHERE org_id=$1 AND id=$2`, org, gateID).Scan(&raw); err != nil {
			return err
		}
		var gate gitops.Gate
		if err := json.Unmarshal(raw, &gate); err != nil {
			return err
		}
		if gate.Request.RecoveryOf != id || gate.Configuration.SourceRepositoryID != failed.Promotion.SourceRepositoryID || gate.Configuration.DeliveryRepositoryID != failed.Promotion.DeliveryRepositoryID || !auth.CanReadRepository(a, gate.Configuration.SourceRepositoryID) || !auth.CanReadRepository(a, gate.Configuration.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		known = gate.Request.RestorePromotionID
		return nil
	})
	if err != nil {
		return out, err
	}
	operation, err := s.GitOps.Request(ctx, session, org, gateID, domain.StableID("restore-working-release", org, kind, id, known), request)
	out.ID, out.State = operation.ID, operation.State
	return out, err
}
