package workflow

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func (s *Service) SetCampaignPauseTx(ctx context.Context, tx pgx.Tx, org, id string, paused bool, actor, request string) error {
	if !auth.ValidID(org) || !auth.ValidID(id) || !auth.ValidID(actor) {
		return auth.ErrInvalid
	}
	if e := s.scopeExists(ctx, tx, org, "campaign", id); e != nil {
		return e
	}
	var version int64
	e := tx.QueryRow(ctx, `INSERT INTO workflow_pauses(org_id,scope_kind,scope_id,paused,version) VALUES($1,'campaign',$2,$3,1) ON CONFLICT(org_id,scope_kind,scope_id) DO UPDATE SET paused=EXCLUDED.paused,version=workflow_pauses.version+1 RETURNING version`, org, id, paused).Scan(&version)
	if e != nil {
		return e
	}
	p := Pause{Kind: "campaign", ID: id, Paused: paused, Version: version}
	if paused {
		if e = s.revokePausedTx(ctx, tx, org, p, request); e != nil {
			return e
		}
	}
	if e = audit(ctx, tx, org, actor, "automation.pause_changed", "campaign:"+id, request, version); e != nil {
		return e
	}
	return EmitTx(ctx, tx, domain.Event{OrgID: org, Type: "automation.paused", AggregateType: "campaign", AggregateID: id, AggregateVersion: version, DataVersion: 1, Data: mustJSON(p), RequestID: request})
}

func (s *Service) RegisterCampaignAuthority(check func(context.Context, pgx.Tx, Task) error) {
	s.scopeMu.Lock()
	defer s.scopeMu.Unlock()
	s.campaignCheck = check
}
