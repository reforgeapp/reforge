package control

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/policy"
	"reforge/internal/workflow"
)

type ActionCheck func(context.Context, pgx.Tx, workflow.Task, policy.Resolved) error

type Authority struct {
	policies *policy.Service
	mu       sync.RWMutex
	actions  map[string]ActionCheck
}

func NewAuthority(policies *policy.Service) *Authority {
	return &Authority{policies: policies, actions: map[string]ActionCheck{}}
}

func (a *Authority) Register(action string, check ActionCheck) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions[action] = check
}

func (a *Authority) Check(ctx context.Context, tx pgx.Tx, task workflow.Task, action string) (string, error) {
	r, err := a.policies.ResolveTx(ctx, tx, task.OrgID, task.RepositoryID)
	if err != nil {
		return "", err
	}
	if r.Hash == "" || len(r.Problems) != 0 || r.Paused || slices.Contains(r.Policy.Deny, policy.Repair) {
		return "", workflow.ErrPolicy
	}
	if task.ModelConnectionID == "" || task.ModelRoute == "" {
		return "", workflow.ErrPolicy
	}
	var settings connections.Settings
	var raw []byte
	var kind, state string
	if err = tx.QueryRow(ctx, `SELECT kind,state,settings FROM connections WHERE org_id=$1 AND id=$2`, task.OrgID, task.ModelConnectionID).Scan(&kind, &state, &raw); err != nil {
		return "", workflow.ErrPolicy
	}
	if json.Unmarshal(raw, &settings) != nil || (kind != "model" && kind != "agent") || state != "healthy" || settings.Model == "" {
		return "", workflow.ErrPolicy
	}
	for _, item := range []struct {
		values []string
		value  string
	}{
		{r.Policy.Allow.Recipes, task.Recipe},
		{r.Policy.Allow.Models, settings.Model},
		{r.Policy.Allow.Routes, task.ModelConnectionID + "/" + task.ModelRoute},
	} {
		if item.value == "" || (item.values != nil && !slices.Contains(item.values, item.value)) {
			return "", workflow.ErrPolicy
		}
	}
	if r.Policy.Limits.Attempts != nil && int64(task.MaxAttempts) > *r.Policy.Limits.Attempts {
		return "", workflow.ErrPolicy
	}
	switch action {
	case "enqueue", "resume", "dispatch", "heartbeat", "advance", "complete", "budget", "artifact":
		return r.Hash, nil
	}
	a.mu.RLock()
	check := a.actions[action]
	a.mu.RUnlock()
	if check == nil {
		return "", workflow.ErrPolicy
	}
	if err = check(ctx, tx, task, r); err != nil {
		return "", err
	}
	return r.Hash, nil
}
