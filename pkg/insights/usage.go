package insights

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"time"
)

const usageFrom = ` FROM budget_reservations r JOIN repositories repo ON repo.org_id=r.org_id AND repo.id=r.repository_id JOIN workflow_tasks task ON task.org_id=r.org_id AND task.id=r.task_id JOIN connections c ON c.org_id=r.org_id AND c.id=(r.record->>'connection_id')::uuid WHERE r.org_id=$1 AND ($2 OR r.repository_id=ANY($3::uuid[])) AND ($4='' OR r.repository_id=NULLIF($4,'')::uuid) AND ($5='' OR EXISTS(SELECT 1 FROM team_repositories t WHERE t.org_id=r.org_id AND t.repository_id=r.repository_id AND t.team_id=NULLIF($5,'')::uuid)) AND ($6='' OR task.recipe=$6) AND ($7='' OR c.provider=$7) AND ($8='' OR c.id=NULLIF($8,'')::uuid) AND ($9='' OR r.state=$9) AND ($10::timestamptz IS NULL OR r.created_at>=$10) AND ($11::timestamptz IS NULL OR r.created_at<$11)`

func usageArgs(org string, a domain.Actor, f Filter) []any {
	return []any{org, a.AllRepositories, a.RepositoryIDs, f.RepositoryID, f.TeamID, f.Recipe, f.Provider, f.ConnectionID, f.State, f.Since, f.Until}
}
func (s *Service) Usage(ctx context.Context, session auth.Session, org string, f Filter) (domain.Page[Usage], error) {
	out := domain.Page[Usage]{Items: []Usage{}}
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
		rows, e := tx.Query(ctx, `SELECT r.record,c.provider,task.recipe,repo.name`+usageFrom+` AND ($12::timestamptz IS NULL OR (r.created_at,r.id)<($12::timestamptz,$13::uuid)) ORDER BY r.created_at DESC,r.id DESC LIMIT $14`, append(usageArgs(org, a, f), at, id, f.Limit+1)...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var item Usage
			var raw []byte
			if e = rows.Scan(&raw, &item.Provider, &item.Recipe, &item.RepositoryName); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &item.Reservation); e != nil {
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
			last := out.Items[len(out.Items)-1].Reservation
			out.NextCursor = next(last.CreatedAt, last.ID)
		}
		return nil
	})
	return out, err
}
func (s *Service) UsageSummary(ctx context.Context, session auth.Session, org string, f Filter) (Summary, error) {
	var out Summary
	if e := validate(f); e != nil {
		return out, e
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if e := scope(ctx, tx, org, a, f); e != nil {
			return e
		}
		query := `SELECT count(*),count(*) FILTER(WHERE r.state='settled'),count(*) FILTER(WHERE r.state='unknown'),count(*) FILTER(WHERE r.state='reserved'),count(*) FILTER(WHERE r.state='dispatched'),count(*) FILTER(WHERE r.state='cancelled'),COALESCE(sum((r.record->'actual'->>'micro_usd')::bigint) FILTER(WHERE r.state='settled'),0),COALESCE(sum((r.record->'actual'->>'tokens')::bigint) FILTER(WHERE r.state='settled'),0)`
		for _, state := range []string{"r.state='unknown'", "r.state IN ('reserved','dispatched','unknown')"} {
			for _, dimension := range []string{"micro_usd", "tokens", "milliseconds", "requests", "concurrency"} {
				query += `,COALESCE(sum((r.record->'maximum'->>'` + dimension + `')::bigint) FILTER(WHERE ` + state + `),0)`
			}
		}
		return tx.QueryRow(ctx, query+usageFrom, usageArgs(org, a, f)...).Scan(&out.Records, &out.Settled, &out.Unknown, &out.Reserved, &out.Dispatched, &out.Cancelled, &out.EstimatedCostMicroUSD, &out.KnownTokens, &out.UnknownMaximum.MicroUSD, &out.UnknownMaximum.Tokens, &out.UnknownMaximum.Milliseconds, &out.UnknownMaximum.Requests, &out.UnknownMaximum.Concurrency, &out.Held.MicroUSD, &out.Held.Tokens, &out.Held.Milliseconds, &out.Held.Requests, &out.Held.Concurrency)
	})
	return out, err
}

type UsageDay struct {
	Day      string `json:"day"`
	MicroUSD int64  `json:"micro_usd"`
	Tokens   int64  `json:"tokens"`
	Records  int64  `json:"records"`
}
type UsageProvider struct {
	Provider string `json:"provider"`
	MicroUSD int64  `json:"micro_usd"`
	Tokens   int64  `json:"tokens"`
	Records  int64  `json:"records"`
}
type UsageSeries struct {
	Days      []UsageDay      `json:"days"`
	Providers []UsageProvider `json:"providers"`
}

func (s *Service) UsageSeries(ctx context.Context, session auth.Session, org string, f Filter) (UsageSeries, error) {
	out := UsageSeries{Days: []UsageDay{}, Providers: []UsageProvider{}}
	if e := validate(f); e != nil {
		return out, e
	}
	until := time.Now().UTC()
	if f.Until != nil {
		until = f.Until.UTC()
	}
	since := until.AddDate(0, 0, -30)
	if f.Since != nil {
		since = f.Since.UTC()
	}
	if until.Sub(since) > 366*24*time.Hour {
		return out, auth.ErrInvalid
	}
	f.Since, f.Until = &since, &until
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if e := scope(ctx, tx, org, a, f); e != nil {
			return e
		}
		settled := `FILTER(WHERE r.state='settled'),0)`
		rows, e := tx.Query(ctx, `WITH u AS (SELECT date_trunc('day',r.created_at AT TIME ZONE 'UTC') AS day,COALESCE(sum((r.record->'actual'->>'micro_usd')::bigint) `+settled+` AS micro_usd,COALESCE(sum((r.record->'actual'->>'tokens')::bigint) `+settled+` AS tokens,count(*) AS records`+usageFrom+` GROUP BY 1)
			SELECT to_char(d,'YYYY-MM-DD'),COALESCE(u.micro_usd,0),COALESCE(u.tokens,0),COALESCE(u.records,0) FROM generate_series(date_trunc('day',$10::timestamptz AT TIME ZONE 'UTC'),date_trunc('day',($11::timestamptz - interval '1 microsecond') AT TIME ZONE 'UTC'),interval '1 day') d LEFT JOIN u ON u.day=d ORDER BY d`, usageArgs(org, a, f)...)
		if e != nil {
			return e
		}
		for rows.Next() {
			var item UsageDay
			if e = rows.Scan(&item.Day, &item.MicroUSD, &item.Tokens, &item.Records); e != nil {
				rows.Close()
				return e
			}
			out.Days = append(out.Days, item)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		rows, e = tx.Query(ctx, `SELECT c.provider,COALESCE(sum((r.record->'actual'->>'micro_usd')::bigint) `+settled+`,COALESCE(sum((r.record->'actual'->>'tokens')::bigint) `+settled+`,count(*)`+usageFrom+` GROUP BY 1 ORDER BY 2 DESC,4 DESC,1`, usageArgs(org, a, f)...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var item UsageProvider
			if e = rows.Scan(&item.Provider, &item.MicroUSD, &item.Tokens, &item.Records); e != nil {
				return e
			}
			out.Providers = append(out.Providers, item)
		}
		return rows.Err()
	})
	return out, err
}
