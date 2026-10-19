package campaign

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/mergecontrol"
	"github.com/reforgeapp/reforge/internal/workflow"
)

func (s *Service) automationSession(ctx context.Context, org, id, purpose, controller string) (auth.Session, error) {
	var user string
	repos := []string{}
	e := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT requested_by::text FROM campaigns WHERE org_id=$1 AND id=$2`, org, id).Scan(&user); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT repository_id::text FROM campaign_repository_scopes WHERE org_id=$1 AND campaign_id=$2 ORDER BY repository_id`, org, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var repo string
			if e = rows.Scan(&repo); e != nil {
				return e
			}
			repos = append(repos, repo)
		}
		return rows.Err()
	})
	if e != nil {
		return auth.Session{}, e
	}
	return auth.NewAutomationSession(org, user, id, repos, func(ctx context.Context, tx pgx.Tx) error {
		var current, state, owner string
		var until *time.Time
		var expiry time.Time
		if e := tx.QueryRow(ctx, `SELECT requested_by::text,state,grant_expires_at,coalesce(controller_id::text,''),controller_until FROM campaigns WHERE org_id=$1 AND id=$2 FOR SHARE`, org, id).Scan(&current, &state, &expiry, &owner, &until); e != nil {
			return e
		}
		if purpose != "observe" && (owner != controller || until == nil || !until.After(time.Now())) {
			return workflow.ErrFence
		}
		if purpose == "cancel" && active(state) && expiry.After(time.Now()) {
			return workflow.ErrFence
		}
		if current != user {
			return auth.ErrForbidden
		}
		if purpose == "dispatch" && (!active(state) || !expiry.After(time.Now())) {
			return workflow.ErrPaused
		}
		if purpose != "dispatch" && purpose != "observe" && purpose != "cancel" {
			return auth.ErrInvalid
		}
		return nil
	})
}

func (s *Service) authorityTx(ctx context.Context, tx pgx.Tx, org, id string) (Campaign, domain.Actor, error) {
	c, e := load(ctx, tx, org, id)
	if e != nil {
		return c, domain.Actor{}, e
	}
	if !active(c.State) {
		return c, domain.Actor{}, workflow.ErrPaused
	}
	var expiry time.Time
	if e = tx.QueryRow(ctx, `SELECT grant_expires_at FROM campaigns WHERE org_id=$1 AND id=$2`, org, id).Scan(&expiry); e != nil {
		return c, domain.Actor{}, e
	}
	if !expiry.After(time.Now()) {
		return c, domain.Actor{}, workflow.ErrPaused
	}
	rows, e := tx.Query(ctx, `SELECT repository_id::text FROM campaign_repository_scopes WHERE org_id=$1 AND campaign_id=$2 ORDER BY repository_id`, org, id)
	if e != nil {
		return c, domain.Actor{}, e
	}
	repos := []string{}
	for rows.Next() {
		var repo string
		if e = rows.Scan(&repo); e != nil {
			rows.Close()
			return c, domain.Actor{}, e
		}
		repos = append(repos, repo)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return c, domain.Actor{}, e
	}
	grant, e := auth.NewAutomationSession(org, c.RequestedBy, id, repos, func(context.Context, pgx.Tx) error { return nil })
	if e != nil {
		return c, domain.Actor{}, e
	}
	a, e := s.auth.ActorTx(ctx, tx, grant, org)
	return c, a, e
}

func (s *Service) GateAuthorityTx(ctx context.Context, tx pgx.Tx, org, gate string) error {
	var id, memberID string
	e := tx.QueryRow(ctx, `SELECT campaign_id::text,id::text FROM campaign_members WHERE org_id=$1 AND gate_id=$2`, org, gate).Scan(&id, &memberID)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	c, a, e := s.authorityTx(ctx, tx, org, id)
	if e != nil {
		return e
	}
	m, e := memberTx(ctx, tx, org, id, memberID)
	if e != nil {
		return e
	}
	if m.Stage != c.Stage || m.State == "excluded" || m.State == "cancelled" || m.State == "failed" {
		return workflow.ErrPaused
	}
	current, e := s.snapshotMember(ctx, tx, org, a, c.Spec, m)
	if e != nil {
		return e
	}
	return changedPins(m, current)
}

func (s *Service) TaskAuthorityTx(ctx context.Context, tx pgx.Tx, t workflow.Task) error {
	if t.CampaignID == "" {
		return nil
	}
	c, a, e := s.authorityTx(ctx, tx, t.OrgID, t.CampaignID)
	if e != nil {
		return e
	}
	var memberID string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM campaign_members WHERE org_id=$1 AND campaign_id=$2 AND action_id=$3`, t.OrgID, c.ID, t.ID).Scan(&memberID); errors.Is(e, pgx.ErrNoRows) {
		return auth.ErrForbidden
	} else if e != nil {
		return e
	}
	m, e := memberTx(ctx, tx, t.OrgID, c.ID, memberID)
	if e != nil {
		return e
	}
	if c.Kind != "repair" || m.RepositoryID != t.RepositoryID || m.Stage != c.Stage || m.State == "excluded" || m.State == "cancelled" {
		return workflow.ErrPaused
	}
	if t.StartingPolicyHash != m.Pins["policy/"+t.RepositoryID] {
		return workflow.ErrPolicy
	}
	current, e := s.snapshotMember(ctx, tx, t.OrgID, a, c.Spec, m)
	if e != nil {
		return e
	}
	return changedPins(m, current)
}

func (s *Service) CheckMergeTx(ctx context.Context, tx pgx.Tx, org, repo string, snapshot forge.MergeEvidence) error {
	var id, member string
	e := tx.QueryRow(ctx, `SELECT m.campaign_id::text,m.id::text FROM campaign_members m JOIN repair_runs r ON r.org_id=m.org_id AND r.task_id=m.action_id WHERE m.org_id=$1 AND m.repository_id=$2 AND r.native_change->>'id'=$3`, org, repo, snapshot.Change.ID).Scan(&id, &member)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	c, e := load(ctx, tx, org, id)
	if e != nil {
		return e
	}
	if c.State == "completed" || c.State == "cancelled" {
		return nil
	}
	_, a, e := s.authorityTx(ctx, tx, org, id)
	if e != nil {
		return &mergecontrol.AuthorityBlocker{Reason: "Campaign is paused or its approval expired; review before protected merging"}
	}
	m, e := memberTx(ctx, tx, org, id, member)
	if e != nil {
		return e
	}
	current, e := s.snapshotMember(ctx, tx, org, a, c.Spec, m)
	if e != nil {
		return e
	}
	if changedPins(m, current) != nil || !withinWindow(c.Spec, time.Now()) {
		return &mergecontrol.AuthorityBlocker{Reason: "Campaign pins or maintenance window prevent protected merging"}
	}
	return nil
}
