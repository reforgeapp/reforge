package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
)

func (s *Service) Maintain(ctx context.Context, org string) error {
	return s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM inventory_deliveries WHERE (org_id,endpoint_id,delivery_key) IN (SELECT org_id,endpoint_id,delivery_key FROM inventory_deliveries WHERE org_id=$1 AND received_at<clock_timestamp()-interval '7 days' ORDER BY received_at LIMIT 500)`, org)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM inventory_jobs WHERE (org_id,id) IN (SELECT j.org_id,j.id FROM inventory_jobs j WHERE j.org_id=$1 AND j.state NOT IN ('queued','running') AND j.updated_at<clock_timestamp()-interval '30 days' AND NOT EXISTS(SELECT 1 FROM inventory_jobs child WHERE child.org_id=j.org_id AND child.parent_id=j.id) AND NOT EXISTS(SELECT 1 FROM inventory_repository_state s WHERE s.org_id=j.org_id AND s.scan_id=j.id) AND NOT EXISTS(SELECT 1 FROM inventory_changes c WHERE c.org_id=j.org_id AND c.job_id=j.id) ORDER BY j.updated_at LIMIT 50)`, org)
		if err != nil {
			return err
		}
		var connectionID string
		err = tx.QueryRow(ctx, `SELECT s.connection_id::text FROM inventory_sources s JOIN connections c ON c.org_id=s.org_id AND c.id=s.connection_id WHERE s.org_id=$1 AND s.poll_due<=clock_timestamp() AND c.state NOT IN ('disabled','revoked') AND NOT EXISTS(SELECT 1 FROM inventory_jobs j WHERE j.org_id=s.org_id AND j.connection_id=s.connection_id AND j.kind='scan' AND j.state IN ('queued','running')) ORDER BY s.poll_due,s.connection_id LIMIT 1`, org).Scan(&connectionID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			c, err := connection(ctx, tx, org, connectionID)
			if err != nil {
				return err
			}
			if _, err = enqueue(ctx, tx, c, "scan", "", "", "", nil); err != nil {
				if errors.Is(err, ErrBusy) {
					return nil
				}
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE inventory_sources SET namespace=$3,poll_due=clock_timestamp()+interval '15 minutes' WHERE org_id=$1 AND connection_id=$2`, org, connectionID, c.Settings.Namespace)
			if err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `SELECT s.repository_id::text,s.connection_id::text FROM inventory_repository_state s JOIN connections c ON c.org_id=s.org_id AND c.id=s.connection_id JOIN repositories r ON r.org_id=s.org_id AND r.id=s.repository_id WHERE s.org_id=$1 AND s.refresh_due<=clock_timestamp() AND c.state NOT IN ('disabled','revoked') AND s.namespace=coalesce(c.settings->>'namespace','') AND r.accessible AND NOT EXISTS(SELECT 1 FROM inventory_jobs j WHERE j.org_id=s.org_id AND j.repository_id=s.repository_id AND j.kind='refresh' AND j.state IN ('queued','running')) ORDER BY s.refresh_due,s.repository_id LIMIT 10`, org)
		if err != nil {
			return err
		}
		pairs := [][2]string{}
		for rows.Next() {
			var p [2]string
			if err = rows.Scan(&p[0], &p[1]); err != nil {
				rows.Close()
				return err
			}
			pairs = append(pairs, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, p := range pairs {
			c, err := connection(ctx, tx, org, p[1])
			if err != nil {
				return err
			}
			if err = queueRefresh(ctx, tx, c, p[0]); err != nil {
				if errors.Is(err, ErrBusy) {
					return nil
				}
				return err
			}
		}
		return nil
	})
}
func queueRefresh(ctx context.Context, tx pgx.Tx, c connections.Connection, repo string) error {
	var inScope bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_repository_state WHERE org_id=$1 AND repository_id=$2 AND connection_id=$3 AND namespace=$4)`, c.OrgID, repo, c.ID, c.Settings.Namespace).Scan(&inScope); err != nil {
		return err
	}
	if !inScope {
		return nil
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running'))`, c.OrgID, repo).Scan(&active); err != nil {
		return err
	}
	if !active {
		if _, err := enqueue(ctx, tx, c, "refresh", repo, "", "", nil); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE inventory_jobs SET input=jsonb_set(input,'{dirty}','true'::jsonb) WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running')`, c.OrgID, repo); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET refresh_due=clock_timestamp()+interval '5 minutes',changes_observed_at=NULL WHERE org_id=$1 AND repository_id=$2`, c.OrgID, repo)
	return err
}
func (s *Service) nextTenant(ctx context.Context) (string, error) {
	const scheduler = "00000000-0000-4000-8000-000000000011"
	var org string
	err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.inventory_scheduler','true',true)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_scheduler_cursors(id,org_id) VALUES($1,'00000000-0000-0000-0000-000000000000') ON CONFLICT DO NOTHING`, scheduler); err != nil {
			return err
		}
		var cursor string
		if err := tx.QueryRow(ctx, `SELECT org_id::text FROM inventory_scheduler_cursors WHERE id=$1 FOR UPDATE`, scheduler).Scan(&cursor); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT org_id::text FROM inventory_tenants WHERE org_id>$1::uuid ORDER BY org_id LIMIT 1`, cursor).Scan(&org)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT org_id::text FROM inventory_tenants ORDER BY org_id LIMIT 1`).Scan(&org)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE inventory_scheduler_cursors SET org_id=$2 WHERE id=$1`, scheduler, org)
		return err
	})
	return org, err
}
func (s *Service) RunOnce(ctx context.Context, worker string) (bool, error) {
	first := ""
	for i := 0; i < 50; i++ {
		org, err := s.nextTenant(ctx)
		if err != nil {
			return false, err
		}
		if org == "" || org == first {
			return false, nil
		}
		if first == "" {
			first = org
		}
		if err = s.Maintain(ctx, org); err != nil {
			return false, err
		}
		j, err := s.Claim(ctx, org, worker)
		if err != nil {
			return false, err
		}
		if j != nil {
			return true, s.Step(ctx, *j)
		}
	}
	return false, nil
}
func (s *Service) Run(ctx context.Context, worker string) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		worked, err := s.RunOnce(ctx, worker)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if worked && err == nil {
			continue
		}
		delay := time.Second
		if err != nil {
			delay = 5 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
