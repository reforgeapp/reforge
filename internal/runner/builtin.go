package runner

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/auth"
	"reforge/internal/domain"
)

const builtinActor = "system:builtin-runner"

func (s *Service) BuiltinOrgs(ctx context.Context) ([]string, error) {
	orgs := []string{}
	err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT org_id::text FROM inventory_tenants ORDER BY org_id`)
		if err != nil {
			return err
		}
		orgs, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	return orgs, err
}

func (s *Service) EnrollBuiltin(ctx context.Context, org, name string) (Credential, error) {
	var c Credential
	if !auth.ValidID(org) || strings.TrimSpace(name) == "" || len(name) > 160 {
		return c, auth.ErrInvalid
	}
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return err
		}
		var pool, state string
		err := tx.QueryRow(ctx, `SELECT id::text,state FROM runner_pools WHERE org_id=$1 AND builtin`, org).Scan(&pool, &state)
		if errors.Is(err, pgx.ErrNoRows) {
			pool, state = domain.NewID(), "active"
			if _, err = tx.Exec(ctx, `INSERT INTO runner_pools(org_id,id,name,state,builtin) VALUES($1,$2,'Built-in','active',true)`, org, pool); err != nil {
				return err
			}
			if err = audit(ctx, tx, org, builtinActor, "runner.pool_changed", pool, "", map[string]any{"name": "Built-in", "builtin": true}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if state != "active" {
			return auth.ErrForbidden
		}
		if _, err = tx.Exec(ctx, `INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id) SELECT org_id,$2,id FROM repositories WHERE org_id=$1 ON CONFLICT DO NOTHING`, org, pool); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text FROM runners WHERE org_id=$1 AND pool_id=$2 AND state='active'`, org, pool)
		if err != nil {
			return err
		}
		previous, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, id := range previous {
			if err = s.invalidateTx(ctx, tx, org, id, true); err != nil {
				return err
			}
		}
		c.Runner = Runner{ID: domain.NewID(), OrgID: org, PoolID: pool, Name: strings.TrimSpace(name), State: "active", Version: 1, CredentialExpiresAt: time.Now().UTC().Add(24 * time.Hour)}
		c.ExpiresAt = c.Runner.CredentialExpiresAt
		c.Token = token("sup", org, c.Runner.ID)
		if _, err = tx.Exec(ctx, `INSERT INTO runners(org_id,id,pool_id,name,credential_hash,credential_expires_at) VALUES($1,$2,$3,$4,$5,$6)`, org, c.Runner.ID, pool, c.Runner.Name, hash(c.Token), c.ExpiresAt); err != nil {
			return err
		}
		return audit(ctx, tx, org, builtinActor, "runner.enrolled", c.Runner.ID, "", map[string]string{"pool_id": pool})
	})
	if err != nil {
		c.Token = ""
	}
	return c, err
}
