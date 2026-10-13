package delivery

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/gitops"
	"strings"
)

func (s *Service) Deploy(ctx context.Context, session auth.Session, org, receiptID, requestID string) (Result, error) {
	var out Result
	r, request, err := s.Releases.Request(ctx, session, org, receiptID)
	if err != nil {
		return out, err
	}
	p := r.Provenance.Document
	if p.Kind == "gitops" {
		session, err = s.gitopsSession(ctx, session, org, p.RepositoryID, p.Environment)
		if err != nil {
			return out, err
		}
	}
	if session.AutomationID() != "" {
		if err := s.currentHead(ctx, session, org, p.RepositoryID, p.SourceSHA); err != nil {
			return out, err
		}
	}
	key := domain.StableID("release-delivery", org, p.RepositoryID, p.Kind, p.Environment, p.SourceSHA, p.ArtifactDigest, strconv.FormatInt(p.ConfigurationVersion, 10))
	if previous, found, lookupErr := s.existingRelease(ctx, session, org, p, key); lookupErr != nil || found {
		return previous, lookupErr
	}
	out.Kind = p.Kind
	if p.Kind == "native" {
		if s.Native == nil {
			return out, &domain.ProviderError{Kind: "unsupported", Message: "Native delivery is unavailable"}
		}
		var operation deployment.Operation
		if r.Mode == "observe" {
			operation, err = s.Native.Track(ctx, session, org, p.Environment, deployment.TrackRequest{PreviewRequest: request, RunID: p.BuildID, IdempotencyKey: key}, requestID)
		} else {
			gate, previewErr := s.Native.Preview(ctx, session, org, p.Environment, request, requestID)
			if previewErr != nil {
				return out, previewErr
			}
			if len(gate.Blockers) > 0 {
				return out, &domain.ProviderError{Kind: "unknown", Message: strings.Join(gate.Blockers, "; ")}
			}
			operation, err = s.Native.Request(ctx, session, org, gate.ID, key, requestID)
		}
		out.ID, out.State = operation.ID, operation.State
		if errors.Is(err, auth.ErrConflict) {
			if previous, found, lookupErr := s.existingRelease(ctx, session, org, p, key); lookupErr == nil && found {
				return previous, nil
			}
		}
		return out, err
	}
	if s.GitOps == nil {
		return out, &domain.ProviderError{Kind: "unsupported", Message: "GitOps delivery is unavailable"}
	}
	gate, err := s.GitOps.Preview(ctx, session, org, p.Environment, gitops.PreviewRequest{ChangeID: request.ChangeID, SourceSHA: request.SourceSHA, ArtifactDigest: request.ArtifactDigest, Provenance: request.Provenance}, requestID)
	if err != nil {
		return out, err
	}
	if len(gate.Blockers) > 0 {
		return out, &domain.ProviderError{Kind: "unknown", Message: strings.Join(gate.Blockers, "; ")}
	}
	operation, err := s.GitOps.Request(ctx, session, org, gate.ID, key, requestID)
	out.ID, out.State = operation.ID, operation.State
	if errors.Is(err, auth.ErrConflict) {
		if previous, found, lookupErr := s.existingRelease(ctx, session, org, p, key); lookupErr == nil && found {
			return previous, nil
		}
	}
	return out, err
}

func (s *Service) existingRelease(ctx context.Context, session auth.Session, org string, p deployment.Provenance, key string) (Result, bool, error) {
	out := Result{Kind: p.Kind}
	found := false
	err := s.Releases.Auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner && a.Role != domain.Admin && a.Role != domain.Maintainer || !auth.CanReadRepository(a, p.RepositoryID) {
			return auth.ErrForbidden
		}
		query := `SELECT id::text,state FROM deployments WHERE org_id=$1 AND repository_id=$2 AND environment=$3 AND idempotency_key=$4 AND recovery_of IS NULL`
		if p.Kind == "gitops" {
			query = `SELECT id::text,state FROM gitops_promotions WHERE org_id=$1 AND source_repository_id=$2 AND environment=$3 AND idempotency_key=$4 AND recovery_of IS NULL AND ($5 OR delivery_repository_id=ANY($6::uuid[]))`
		}
		var err error
		if p.Kind == "gitops" {
			err = tx.QueryRow(ctx, query, org, p.RepositoryID, p.Environment, key, a.AllRepositories, a.RepositoryIDs).Scan(&out.ID, &out.State)
		} else {
			err = tx.QueryRow(ctx, query, org, p.RepositoryID, p.Environment, key).Scan(&out.ID, &out.State)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		found = err == nil
		return err
	})
	return out, found, err
}
