package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.MaxConns = 20
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, errors.New("database unavailable")
	}
	var unsafe bool
	err = p.QueryRow(ctx, `SELECT has_schema_privilege(current_user,'public','CREATE') OR EXISTS (
		SELECT 1 FROM pg_roles r WHERE pg_has_role(current_user,r.oid,'MEMBER') AND (
			r.rolsuper OR r.rolbypassrls OR r.rolcreatedb OR r.rolcreaterole OR
			EXISTS (SELECT 1 FROM pg_class c WHERE c.relnamespace='public'::regnamespace AND c.relkind IN ('r','p') AND c.relowner=r.oid) OR
			EXISTS (SELECT 1 FROM pg_database d WHERE d.datname=current_database() AND d.datdba=r.oid)
		))`).Scan(&unsafe)
	if err != nil || unsafe {
		p.Close()
		return nil, errors.New("runtime database role must not own tables or bypass row security")
	}
	return &Store{Pool: p}, nil
}

func (s *Store) Close() { s.Pool.Close() }
func (s *Store) Tenant(ctx context.Context, orgID, userID string, fn func(pgx.Tx) error) error {
	if orgID == "" {
		return errors.New("tenant required")
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.org_id',$1,true),set_config('reforge.user_id',$2,true)`, orgID, userID); err != nil {
			return fmt.Errorf("tenant context: %w", err)
		}
		return fn(tx)
	})
}

func (s *Store) Identity(ctx context.Context, userID string, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.user_id',$1,true)`, userID); err != nil {
			return err
		}
		return fn(tx)
	})
}
