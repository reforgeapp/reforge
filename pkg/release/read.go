package release

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func receiptTx(ctx context.Context, tx pgx.Tx, org, id string) (Receipt, error) {
	var out Receipt
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT id::text,document,received_at FROM release_receipts WHERE org_id=$1 AND id=$2`, org, id).Scan(&out.ID, &raw, &out.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, auth.ErrForbidden
	}
	if err == nil {
		err = json.Unmarshal(raw, &out.Provenance)
	}
	return out, err
}

func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Receipt, error) {
	var out Receipt
	if !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	err := s.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		out, err = receiptTx(ctx, tx, org, id)
		if err == nil && !auth.CanReadRepository(a, out.Provenance.Document.RepositoryID) {
			return auth.ErrForbidden
		}
		return err
	})
	return out, err
}

func (s *Service) List(ctx context.Context, session auth.Session, org, repo, cursor string, limit int) (domain.Page[Receipt], error) {
	out := domain.Page[Receipt]{Items: []Receipt{}}
	if !auth.ValidID(repo) || cursor != "" && !auth.ValidID(cursor) || limit < 1 || limit > 100 {
		return out, auth.ErrInvalid
	}
	err := s.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		var at *time.Time
		if cursor != "" {
			r, err := receiptTx(ctx, tx, org, cursor)
			if err != nil {
				return err
			}
			if r.Provenance.Document.RepositoryID != repo {
				return auth.ErrForbidden
			}
			at = &r.ReceivedAt
		}
		rows, err := tx.Query(ctx, `SELECT id::text,document,received_at FROM release_receipts WHERE org_id=$1 AND repository_id=$2 AND ($3::timestamptz IS NULL OR (received_at,id)<($3,$4::uuid)) ORDER BY received_at DESC,id DESC LIMIT $5`, org, repo, at, nullableID(cursor), limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Receipt
			var raw []byte
			if err = rows.Scan(&r.ID, &raw, &r.ReceivedAt); err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &r.Provenance); err != nil {
				return err
			}
			out.Items = append(out.Items, r)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		rows.Close()
		for i := range out.Items {
			p := out.Items[i].Provenance.Document
			c, err := configurationTx(ctx, tx, org, repo, p.Kind, p.Environment)
			if err != nil && !errors.Is(err, auth.ErrForbidden) {
				return err
			}
			out.Items[i].Mode = c.Mode
		}
		out.Complete = len(out.Items) <= limit
		if !out.Complete {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[limit-1].ID
		}
		return nil
	})
	return out, err
}

func nullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (s *Service) Request(ctx context.Context, session auth.Session, org, id string) (Receipt, deployment.PreviewRequest, error) {
	var r Receipt
	var request deployment.PreviewRequest
	if !auth.ValidID(id) {
		return r, request, auth.ErrInvalid
	}
	err := s.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		r, err = receiptTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		p := r.Provenance.Document
		if !auth.CanReadRepository(a, p.RepositoryID) {
			return auth.ErrForbidden
		}
		c, err := configurationTx(ctx, tx, org, p.RepositoryID, p.Kind, p.Environment)
		if err != nil {
			return err
		}
		if !valid(c, org, p.RepositoryID, r.Provenance, time.Now()) {
			return &domain.ProviderError{Kind: "conflict", Message: "CI release evidence expired or deployment settings changed; run the configured build again"}
		}
		var change string
		r.Mode = c.Mode
		err = tx.QueryRow(ctx, `SELECT c.snapshot->>'id' FROM inventory_changes c JOIN repositories r ON r.org_id=c.org_id AND r.id=c.repository_id WHERE c.org_id=$1 AND c.repository_id=$2 AND c.snapshot->>'state'='merged' AND c.snapshot->>'merge_sha'=$3 AND c.snapshot->>'target_branch'=r.default_branch ORDER BY c.observed_at DESC LIMIT 1`, org, p.RepositoryID, p.SourceSHA).Scan(&change)
		if errors.Is(err, pgx.ErrNoRows) {
			return &domain.ProviderError{Kind: "unknown", Message: "Sync this repository to link the release to its merged change"}
		}
		if err != nil {
			return err
		}
		request = deployment.PreviewRequest{ChangeID: change, SourceSHA: p.SourceSHA, ArtifactDigest: p.ArtifactDigest, Provenance: r.Provenance}
		return nil
	})
	return r, request, err
}
