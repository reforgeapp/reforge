package budget

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

type FenceCheck func(context.Context, pgx.Tx, Lease) error
type CampaignCheck func(context.Context, pgx.Tx, string, string) error

type Service struct {
	db       *store.Store
	auth     *auth.Service
	fence    FenceCheck
	campaign CampaignCheck
}

func New(db *store.Store, identity *auth.Service, fence FenceCheck, campaign CampaignCheck) *Service {
	return &Service{db: db, auth: identity, fence: fence, campaign: campaign}
}
func validScope(scope Scope, org string) bool {
	return auth.ValidID(scope.ID) && (scope.Kind == "organisation" && scope.ID == org || scope.Kind == "team" || scope.Kind == "repository" || scope.Kind == "connection" || scope.Kind == "campaign")
}
func writeAudit(ctx context.Context, tx pgx.Tx, org, actor, action, id, request string, data any) error {
	b, _ := json.Marshal(data)
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), org, actor, action, id, request, b)
	if err != nil {
		return err
	}
	return workflow.EmitTx(ctx, tx, domain.Event{OrgID: org, Type: action, AggregateType: "budget", AggregateID: id, AggregateVersion: 1, DataVersion: 1, Data: b, RequestID: request})
}

func (s *Service) PutLimit(ctx context.Context, session auth.Session, org string, input Limit, expected int64, request string) (Limit, error) {
	if !validScope(input.Scope, org) || expected < 0 || !validCaps(input.Caps) || (input.Period != "daily" && input.Period != "monthly" && input.Period != "custom") {
		return Limit{}, ErrInvalid
	}
	if input.Period == "custom" {
		if input.Start.IsZero() || !input.End.After(input.Start) {
			return Limit{}, ErrInvalid
		}
	} else if !input.Start.IsZero() || !input.End.IsZero() {
		return Limit{}, ErrInvalid
	}
	input.Start = input.Start.UTC()
	input.End = input.End.UTC()
	input.Held = Amount{}
	input.Spent = Amount{}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner || !canReadScope(a, input.Scope) {
			return auth.ErrForbidden
		}
		if err := s.scopeExists(ctx, tx, org, input.Scope); err != nil {
			return err
		}
		old, err := loadLimit(ctx, tx, org, input.Scope)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if old.Version != expected {
			return ErrConflict
		}
		if old.Version > 0 && (old.Period != input.Period || !old.Start.Equal(input.Start) || !old.End.Equal(input.End)) {
			return ErrConflict
		}
		input.Version = expected + 1
		input.Held = old.Held
		caps, _ := json.Marshal(input.Caps)
		_, err = tx.Exec(ctx, `INSERT INTO budget_limits(org_id,scope_kind,scope_id,period,period_start,period_end,caps,paused,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(org_id,scope_kind,scope_id) DO UPDATE SET caps=EXCLUDED.caps,paused=EXCLUDED.paused,version=EXCLUDED.version`, org, input.Scope.Kind, input.Scope.ID, input.Period, input.Start, input.End, caps, input.Paused, input.Version)
		if err != nil {
			return err
		}
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		start, err := periodStart(input, now)
		if input.Period == "custom" {
			start, err = input.Start, nil
		}
		if err != nil {
			return err
		}
		input.Spent, err = loadSpend(ctx, tx, org, input.Scope, start)
		if err != nil {
			return err
		}
		return writeAudit(ctx, tx, org, a.UserID, "budget.configured", input.Scope.ID, request, map[string]any{"scope": input.Scope, "version": input.Version})
	})
	return input, err
}

func (s *Service) GetLimit(ctx context.Context, session auth.Session, org string, scope Scope) (Limit, error) {
	var result Limit
	if !validScope(scope, org) {
		return result, ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !canReadScope(a, scope) || (a.Role != domain.Owner && scope.Kind != "repository") {
			return auth.ErrForbidden
		}
		var err error
		result, err = loadLimit(ctx, tx, org, scope)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknown
		}
		if err != nil {
			return err
		}
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		start, err := periodStart(result, now)
		if result.Period == "custom" {
			start, err = result.Start, nil
		}
		if err != nil {
			return err
		}
		result.Spent, err = loadSpend(ctx, tx, org, scope, start)
		return err
	})
	return result, err
}

func canReadScope(a domain.Actor, scope Scope) bool {
	if scope.Kind == "repository" {
		return auth.CanReadRepository(a, scope.ID)
	}
	if scope.Kind == "team" {
		return a.AllRepositories || slices.Contains(a.TeamIDs, scope.ID)
	}
	return true
}

func (s *Service) scopeExists(ctx context.Context, tx pgx.Tx, org string, scope Scope) error {
	var exists bool
	switch scope.Kind {
	case "organisation":
		return nil
	case "campaign":
		if s.campaign == nil {
			return ErrUnknown
		}
		return s.campaign(ctx, tx, org, scope.ID)
	case "team":
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE org_id=$1 AND id=$2)`, org, scope.ID).Scan(&exists); err != nil {
			return err
		}
	case "repository":
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE org_id=$1 AND id=$2)`, org, scope.ID).Scan(&exists); err != nil {
			return err
		}
	case "connection":
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connections WHERE org_id=$1 AND id=$2 AND kind IN ('model','agent'))`, org, scope.ID).Scan(&exists); err != nil {
			return err
		}
	default:
		return ErrInvalid
	}
	if !exists {
		return auth.ErrForbidden
	}
	return nil
}

func loadLimit(ctx context.Context, tx pgx.Tx, org string, scope Scope) (Limit, error) {
	l := Limit{Scope: scope}
	var caps, held []byte
	err := tx.QueryRow(ctx, `SELECT period,period_start,period_end,caps,held,paused,version FROM budget_limits WHERE org_id=$1 AND scope_kind=$2 AND scope_id=$3`, org, scope.Kind, scope.ID).Scan(&l.Period, &l.Start, &l.End, &caps, &held, &l.Paused, &l.Version)
	if err != nil {
		return l, err
	}
	if err = json.Unmarshal(caps, &l.Caps); err != nil {
		return l, err
	}
	err = json.Unmarshal(held, &l.Held)
	return l, err
}

func loadSpend(ctx context.Context, tx pgx.Tx, org string, scope Scope, start time.Time) (Amount, error) {
	var amount Amount
	var b []byte
	err := tx.QueryRow(ctx, `SELECT amount FROM budget_spend WHERE org_id=$1 AND scope_kind=$2 AND scope_id=$3 AND period_start=$4`, org, scope.Kind, scope.ID, start).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return amount, nil
	}
	if err != nil {
		return amount, err
	}
	err = json.Unmarshal(b, &amount)
	return amount, err
}

func (s *Service) PutRoute(ctx context.Context, session auth.Session, org string, input Route, expected int64, request string) (Route, error) {
	if !validRoute(input) || expected < 0 || input.Qualified || input.QualificationRef != "" {
		return Route{}, ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		if err := s.scopeExists(ctx, tx, org, Scope{"connection", input.ConnectionID}); err != nil {
			return err
		}
		old, err := loadRoute(ctx, tx, org, input.ConnectionID, input.Model, input.Name)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if old.Version != expected {
			return ErrConflict
		}
		input.Version = expected + 1
		b, _ := json.Marshal(input)
		_, err = tx.Exec(ctx, `INSERT INTO budget_routes(org_id,connection_id,model,name,config,version) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(org_id,connection_id,model,name) DO UPDATE SET config=EXCLUDED.config,version=EXCLUDED.version`, org, input.ConnectionID, input.Model, input.Name, b, input.Version)
		if err != nil {
			return err
		}
		return writeAudit(ctx, tx, org, a.UserID, "budget.route_configured", input.ConnectionID, request, map[string]any{"model": input.Model, "route": input.Name, "version": input.Version})
	})
	return input, err
}

func (s *Service) GetRoute(ctx context.Context, session auth.Session, org, connection, model, route string) (Route, error) {
	var result Route
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return auth.ErrForbidden
		}
		var err error
		result, err = loadRoute(ctx, tx, org, connection, model, route)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknown
		}
		return err
	})
	return result, err
}

func loadRoute(ctx context.Context, tx pgx.Tx, org, connection, model, route string) (Route, error) {
	var result Route
	var b []byte
	err := tx.QueryRow(ctx, `SELECT config FROM budget_routes WHERE org_id=$1 AND connection_id=$2 AND model=$3 AND name=$4`, org, connection, model, route).Scan(&b)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(b, &result)
	return result, err
}

func (s *Service) QualifyRouteTx(ctx context.Context, tx pgx.Tx, org, connection, model, route string, expected int64, evidence string) (Route, error) {
	if evidence == "" || len(evidence) > 1000 {
		return Route{}, ErrInvalid
	}
	if err := lockOrg(ctx, tx, org); err != nil {
		return Route{}, err
	}
	r, err := loadRoute(ctx, tx, org, connection, model, route)
	if err != nil {
		return r, err
	}
	if r.Version != expected || r.Mode != "quota" {
		return r, ErrConflict
	}
	r.Qualified = true
	r.QualificationRef = evidence
	r.Version++
	b, _ := json.Marshal(r)
	_, err = tx.Exec(ctx, `UPDATE budget_routes SET config=$5,version=$6 WHERE org_id=$1 AND connection_id=$2 AND model=$3 AND name=$4`, org, connection, model, route, b, r.Version)
	if err == nil {
		err = writeAudit(ctx, tx, org, "agent-controller", "budget.route_qualified", connection, evidence, map[string]any{"model": model, "route": route, "version": r.Version, "evidence": evidence})
	}
	return r, err
}

func lockOrg(ctx context.Context, tx pgx.Tx, org string) error {
	if !auth.ValidID(org) {
		return ErrInvalid
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRevoked
	}
	return err
}

func (s *Service) RouteTx(ctx context.Context, tx pgx.Tx, org, connection, model, route string) (Route, error) {
	return loadRoute(ctx, tx, org, connection, model, route)
}

func (s *Service) HeadroomTx(ctx context.Context, tx pgx.Tx, org string) error {
	return headroomTx(ctx, tx, org, Scope{Kind: "organisation", ID: org}, true)
}

func (s *Service) ConnectionHeadroomTx(ctx context.Context, tx pgx.Tx, org, connection string) error {
	return headroomTx(ctx, tx, org, Scope{Kind: "connection", ID: connection}, false)
}

func headroomTx(ctx context.Context, tx pgx.Tx, org string, scope Scope, required bool) error {
	limit, err := loadLimit(ctx, tx, org, scope)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && limit.Caps.MicroUSD == nil {
		if !required {
			return nil
		}
		return ErrUnknown
	}
	if err != nil {
		return err
	}
	if limit.Paused {
		return ErrRevoked
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	start, err := periodStart(limit, now)
	if err != nil {
		return err
	}
	spent, err := loadSpend(ctx, tx, org, limit.Scope, start)
	if err != nil {
		return err
	}
	if spent.MicroUSD+limit.Held.MicroUSD >= *limit.Caps.MicroUSD {
		return ErrCapacity
	}
	return nil
}
