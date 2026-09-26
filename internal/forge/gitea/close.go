package gitea

import (
	"context"
	"strconv"

	"reforge/internal/forge"
)

func (p *Provider) CloseChange(ctx context.Context, in forge.CloseChangeRequest) (forge.Change, error) {
	if !in.Valid() || !positive(in.ChangeID) {
		return forge.Change{}, failure("invalid", "Reforge pull request required")
	}
	actor, e := p.actor(ctx)
	if e != nil {
		return forge.Change{}, e
	}
	change, e := p.ReadChange(ctx, in.Repository, in.ChangeID)
	if e != nil {
		return forge.Change{}, e
	}
	if !forge.Closable(change, in, strconv.FormatInt(actor.ID, 10)) {
		return forge.Change{}, failure("conflict", "Pull request is not an open Reforge pull request")
	}
	route, e := repoPath(in.Repository)
	if e != nil {
		return forge.Change{}, e
	}
	if e = p.request(ctx, "POST", route+"/issues/"+in.ChangeID+"/comments", map[string]string{"body": in.Comment}, nil); e != nil {
		return forge.Change{}, e
	}
	if e = p.request(ctx, "PATCH", route+"/pulls/"+in.ChangeID, map[string]string{"state": "closed"}, nil); e != nil {
		return forge.Change{}, e
	}
	return p.ReadChange(ctx, in.Repository, in.ChangeID)
}
