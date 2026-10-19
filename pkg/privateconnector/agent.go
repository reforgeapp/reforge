package privateconnector

import (
	"context"
	"errors"

	"github.com/reforgeapp/reforge/pkg/domain"
)

func (e Executor) executeAgent(ctx context.Context, grant Grant) Result {
	fail := func(code string, uncertain bool) Result {
		return Result{OperationID: grant.Operation.ID, Failure: &Failure{Code: code, Uncertain: uncertain}}
	}
	c := grant.Connection
	if !e.Agents.Configured() || c.Native == nil || !c.Native.Valid() || c.OrgID != e.Target.OrgID || c.ID != c.Native.ConnectionID || c.Native.OrgID != e.Target.OrgID || c.Native.RunnerID != e.Target.RunnerID || c.Provider != c.Native.Provider || c.AuthKind != "official_runtime" || c.Secret != "" || c.Protection != nil || c.Version < 1 || c.CredentialVersion < 1 {
		return fail("configuration", false)
	}
	if grant.Operation.Mutation() && (e.Authorize == nil || e.Authorize(ctx) != nil) {
		return fail("forbidden", false)
	}
	result := Result{OperationID: grant.Operation.ID}
	var err error
	switch grant.Operation.Kind {
	case AgentAction:
		state, failure := e.Agents.Action(ctx, *c.Native, *grant.Operation.Agent)
		result.Native, err = &state, failure
	case ModelTurn:
		if grant.Operation.Turn.Model != c.Model {
			return fail("invalid", false)
		}
		turn, failure := e.Agents.Turn(ctx, *c.Native, *grant.Operation.Turn)
		result.Turn, err = &turn, failure
	default:
		return fail("unsupported", false)
	}
	if err != nil {
		var provider *domain.ProviderError
		if errors.As(err, &provider) {
			return Result{OperationID: grant.Operation.ID, Failure: &Failure{Code: provider.Kind, Uncertain: provider.Uncertain, RetryAfterMS: min(max(provider.RetryAfter.Milliseconds(), 0), 3600000)}}
		}
		return fail("configuration", grant.Operation.Kind == ModelTurn)
	}
	return result
}
