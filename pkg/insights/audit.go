package insights

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func (s *Service) Audit(ctx context.Context, session auth.Session, org string, f Filter) (domain.Page[Audit], error) {
	out := domain.Page[Audit]{Items: []Audit{}}
	if e := validate(f); e != nil {
		return out, e
	}
	at, id, e := cursor(f.Cursor)
	if e != nil {
		return out, e
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if e := scope(ctx, tx, org, a, f); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id::text,actor_id,action,object_id,request_id,coalesce(repository_id::text,''),occurred_at,data FROM audit_events WHERE org_id=$1 AND ((repository_id IS NULL AND $2) OR (repository_id IS NOT NULL AND ($3 OR repository_id=ANY($4::uuid[])))) AND ($5='' OR repository_id=NULLIF($5,'')::uuid) AND ($6='' OR actor_id=$6) AND ($7='' OR action=$7) AND ($8::timestamptz IS NULL OR occurred_at>=$8) AND ($9::timestamptz IS NULL OR occurred_at<$9) AND ($10::timestamptz IS NULL OR (occurred_at,id)<($10::timestamptz,$11::uuid)) ORDER BY occurred_at DESC,id DESC LIMIT $12`, org, a.Role == domain.Owner || a.Role == domain.Admin, a.AllRepositories, a.RepositoryIDs, f.RepositoryID, f.ActorID, f.Action, f.Since, f.Until, at, id, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var item Audit
			if e = rows.Scan(&item.ID, &item.ActorID, &item.Action, &item.ObjectID, &item.RequestID, &item.RepositoryID, &item.CreatedAt, &item.Data); e != nil {
				return e
			}
			out.Items = append(out.Items, item)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		out.Complete = len(out.Items) <= f.Limit
		if !out.Complete {
			out.Items = out.Items[:f.Limit]
			last := out.Items[len(out.Items)-1]
			out.NextCursor = next(last.CreatedAt, last.ID)
		}
		return nil
	})
	return out, err
}
