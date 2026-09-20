package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
)

const promotionColumns = `id::text,environment,source_repository_id::text,delivery_repository_id::text,gate_id::text,state,reason,version,requested_by::text,branch,candidate_sha,native_change,merge_sha,COALESCE(recovery_of::text,''),created_at,updated_at,finished_at,cancel_requested`

func scanPromotion(row pgx.Row) (Promotion, error) {
	var p Promotion
	var raw []byte
	err := row.Scan(&p.ID, &p.Environment, &p.SourceRepositoryID, &p.DeliveryRepositoryID, &p.GateID, &p.State, &p.Reason, &p.Version, &p.RequestedBy, &p.Branch, &p.CandidateSHA, &raw, &p.MergeSHA, &p.RecoveryOf, &p.CreatedAt, &p.UpdatedAt, &p.FinishedAt, &p.CancelRequested)
	if err == nil && len(raw) > 0 {
		err = json.Unmarshal(raw, &p.Change)
	}
	return p, err
}
func promotionTx(ctx context.Context, tx pgx.Tx, org, id string) (Promotion, error) {
	return scanPromotion(tx.QueryRow(ctx, `SELECT `+promotionColumns+` FROM gitops_promotions WHERE org_id=$1 AND id=$2`, org, id))
}
func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Detail, error) {
	var out Detail
	if !auth.ValidID(id) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var e error
		out.Promotion, e = promotionTx(ctx, tx, org, id)
		if e != nil {
			return e
		}
		p := out.Promotion
		if !auth.CanReadRepository(a, p.SourceRepositoryID) || !auth.CanReadRepository(a, p.DeliveryRepositoryID) {
			return auth.ErrForbidden
		}
		out.Gate, e = gateTx(ctx, tx, org, p.GateID)
		if e != nil {
			return e
		}
		var raw []byte
		e = tx.QueryRow(ctx, `SELECT document FROM gitops_health_reports WHERE org_id=$1 AND promotion_id=$2 ORDER BY received_at DESC LIMIT 1`, org, id).Scan(&raw)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		return json.Unmarshal(raw, &out.Health)
	})
	return out, err
}
func (s *Service) List(ctx context.Context, session auth.Session, org, env, cursor string, limit int) (domain.Page[Promotion], error) {
	out := domain.Page[Promotion]{Items: []Promotion{}}
	if env != "" && !forge.ValidEnvironment(env) || cursor != "" && !auth.ValidID(cursor) || limit < 1 || limit > 100 {
		return out, auth.ErrInvalid
	}
	if cursor == "" {
		cursor = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, e := tx.Query(ctx, `SELECT `+promotionColumns+` FROM gitops_promotions WHERE org_id=$1 AND ($2='' OR environment=$2) AND id<$3::uuid AND ($4 OR source_repository_id=ANY($5::uuid[]) AND delivery_repository_id=ANY($5::uuid[])) ORDER BY id DESC LIMIT $6`, org, env, cursor, a.AllRepositories, a.RepositoryIDs, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			p, e := scanPromotion(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, p)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		out.Complete = len(out.Items) <= limit
		if !out.Complete {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[len(out.Items)-1].ID
		}
		return nil
	})
	return out, err
}
