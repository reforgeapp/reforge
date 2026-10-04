package privateconnector

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/store"
)

const routeChannel = "reforge_private"

type Routes interface {
	Claim(ctx context.Context, org, route string, until time.Time) error
	Release(org, route string)
	Elsewhere(ctx context.Context, org, route string) (string, error)
}

type Elsewhere struct{ Address string }

func (e Elsewhere) Error() string { return "private operation is held by another control instance" }

type forwardedKey struct{}

func WithForwarded(ctx context.Context) context.Context {
	return context.WithValue(ctx, forwardedKey{}, true)
}

func forwarded(ctx context.Context) bool { return ctx.Value(forwardedKey{}) != nil }

type PostgresRoutes struct {
	db   *store.Store
	self string
}

func NewPostgresRoutes(db *store.Store, self string) PostgresRoutes {
	return PostgresRoutes{db: db, self: self}
}

func (r PostgresRoutes) Claim(ctx context.Context, org, route string, until time.Time) error {
	return r.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM private_routes WHERE org_id=$1 AND expires_at<now()`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO private_routes(org_id,route,pod,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT (org_id,route,pod) DO UPDATE SET expires_at=excluded.expires_at`, org, route, r.self, until); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `SELECT pg_notify($1,$2)`, routeChannel, route)
		return err
	})
}

func (r PostgresRoutes) Release(org, route string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM private_routes WHERE org_id=$1 AND route=$2 AND pod=$3`, org, route, r.self)
		return err
	})
}

func (r PostgresRoutes) Elsewhere(ctx context.Context, org, route string) (string, error) {
	var pod string
	err := r.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT pod FROM private_routes WHERE org_id=$1 AND route=$2 AND pod<>$3 AND expires_at>now() ORDER BY expires_at LIMIT 1`, org, route, r.self).Scan(&pod)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return pod, err
}

func (r PostgresRoutes) Listen(ctx context.Context, notify func(string)) {
	for ctx.Err() == nil {
		err := r.listen(ctx, notify)
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "private route listener restarting", "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (r PostgresRoutes) listen(ctx context.Context, notify func(string)) error {
	conn, err := r.db.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, "LISTEN "+routeChannel); err != nil {
		return err
	}
	notify("")
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		notify(n.Payload)
	}
}
