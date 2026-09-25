package budget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
)

func fingerprint(l Lease, q Quote) string {
	l.ExpiresAt = time.Time{}
	b, _ := json.Marshal(struct {
		Lease Lease
		Quote Quote
	}{l, q})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Service) validateLease(ctx context.Context, tx pgx.Tx, l Lease) (string, string, string, error) {
	for _, id := range []string{l.OrgID, l.RepositoryID, l.TaskID, l.JobID, l.AttemptID, l.OperationID} {
		if !auth.ValidID(id) {
			return "", "", "", ErrInvalid
		}
	}
	if l.Fence < 1 || l.WorkerID == "" || l.PolicyHash == "" {
		return "", "", "", ErrInvalid
	}
	if s.fence == nil {
		return "", "", "", ErrUnknown
	}
	if err := s.fence(ctx, tx, l); err != nil {
		return "", "", "", err
	}
	if err := lockOrg(ctx, tx, l.OrgID); err != nil {
		return "", "", "", err
	}
	var connection, campaign, modelRoute string
	err := tx.QueryRow(ctx, `SELECT coalesce(t.model_connection_id::text,''),coalesce(t.campaign_id::text,''),t.model_route FROM workflow_tasks t JOIN workflow_jobs j ON j.org_id=t.org_id AND j.task_id=t.id JOIN workflow_attempts a ON a.org_id=j.org_id AND a.job_id=j.id AND a.task_id=t.id JOIN organisations o ON o.id=t.org_id JOIN repositories r ON r.org_id=t.org_id AND r.id=t.repository_id WHERE t.org_id=$1 AND t.id=$2 AND t.repository_id=$3 AND j.id=$4 AND a.id=$5 AND j.operation_id=$6 AND j.fence=$7 AND a.fence=$7 AND j.lease_owner=$8 AND a.lease_owner=$8 AND t.policy_hash=$9 AND j.state='running' AND a.state='running' AND j.lease_expires_at>clock_timestamp() AND t.state IN ('reproducing','planning','repairing','validating','publishing') AND NOT o.paused AND NOT r.paused AND r.accessible AND NOT r.archived FOR UPDATE OF j,a`, l.OrgID, l.TaskID, l.RepositoryID, l.JobID, l.AttemptID, l.OperationID, l.Fence, l.WorkerID, l.PolicyHash).Scan(&connection, &campaign, &modelRoute)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrRevoked
	}
	if err != nil {
		return "", "", "", err
	}
	if connection == "" {
		return "", "", "", ErrUnknown
	}
	if campaign != "" {
		if s.campaign == nil {
			return "", "", "", ErrUnknown
		}
		if err = s.campaign(ctx, tx, l.OrgID, campaign); err != nil {
			return "", "", "", err
		}
	}
	return connection, campaign, modelRoute, nil
}

func currentConnection(ctx context.Context, tx pgx.Tx, org, id string, route Route) (int64, error) {
	var version int64
	var kind, billing, model string
	err := tx.QueryRow(ctx, `SELECT version,kind,coalesce(settings->>'billing_route',''),coalesce(settings->>'model','') FROM connections WHERE org_id=$1 AND id=$2 AND state='healthy' AND revoked_at IS NULL`, org, id).Scan(&version, &kind, &billing, &model)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrRevoked
	}
	if err != nil {
		return 0, err
	}
	if route.Mode == "priced" && (kind != "model" || billing != "direct_api" || model != route.Model) {
		return 0, ErrRevoked
	}
	if route.Mode == "quota" && (kind != "agent" || billing != "subscription" || !route.Qualified) {
		return 0, ErrRevoked
	}
	return version, nil
}

func scopes(ctx context.Context, tx pgx.Tx, org, repo, connection, campaign string) ([]Scope, error) {
	out := []Scope{{"organisation", org}, {"repository", repo}, {"connection", connection}}
	if campaign != "" {
		out = append(out, Scope{"campaign", campaign})
	}
	rows, err := tx.Query(ctx, `SELECT team_id::text FROM team_repositories WHERE org_id=$1 AND repository_id=$2 ORDER BY team_id`, org, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, Scope{"team", id})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind+out[i].ID < out[j].Kind+out[j].ID })
	return out, nil
}

func (s *Service) ReserveTx(ctx context.Context, tx pgx.Tx, l Lease, q Quote) (Reservation, error) {
	var result Reservation
	if !auth.ValidID(q.OperationID) {
		return result, ErrInvalid
	}
	connection, campaign, modelRoute, err := s.validateLease(ctx, tx, l)
	if err != nil {
		return result, err
	}
	if q.Route != modelRoute {
		return result, ErrRevoked
	}
	var old []byte
	var oldHash string
	err = tx.QueryRow(ctx, `SELECT record,fingerprint FROM budget_reservations WHERE org_id=$1 AND operation_id=$2`, l.OrgID, q.OperationID).Scan(&old, &oldHash)
	if err == nil {
		if oldHash != fingerprint(l, q) {
			return result, ErrConflict
		}
		err = json.Unmarshal(old, &result)
		return result, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	route, err := loadRoute(ctx, tx, l.OrgID, connection, q.Model, q.Route)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrUnknown
	}
	if err != nil {
		return result, err
	}
	if route.Version != q.RouteVersion {
		return result, ErrConflict
	}
	maximum, err := estimate(route, q)
	if err != nil {
		return result, err
	}
	connectionVersion, err := currentConnection(ctx, tx, l.OrgID, connection, route)
	if err != nil {
		return result, err
	}
	derived, err := scopes(ctx, tx, l.OrgID, l.RepositoryID, connection, campaign)
	if err != nil {
		return result, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return result, err
	}
	result = Reservation{ID: domain.NewID(), Lease: l, Quote: q, ConnectionID: connection, ConnectionVersion: connectionVersion, CampaignID: campaign, Route: route, Maximum: maximum, State: "reserved", CreatedAt: now, Scopes: []ScopeSnapshot{}}
	for _, scope := range derived {
		limit, err := loadLimit(ctx, tx, l.OrgID, scope)
		if errors.Is(err, pgx.ErrNoRows) {
			if scope.Kind == "organisation" || scope.Kind == "campaign" {
				return Reservation{}, ErrUnknown
			}
			continue
		}
		if err != nil {
			return Reservation{}, err
		}
		if limit.Paused {
			return Reservation{}, ErrRevoked
		}
		if scope.Kind == "organisation" && (route.Mode == "priced" && limit.Caps.MicroUSD == nil || route.Mode == "quota" && !quotaCaps(limit.Caps)) {
			return Reservation{}, ErrUnknown
		}
		if scope.Kind == "organisation" && limit.Caps.Concurrency == nil {
			limit.Caps.Concurrency = &defaultConcurrency
		}
		start, err := periodStart(limit, now)
		if err != nil {
			return Reservation{}, err
		}
		spent, err := loadSpend(ctx, tx, l.OrgID, scope, start)
		if err != nil {
			return Reservation{}, err
		}
		total, err := plus(limit.Held, spent)
		if err != nil {
			return Reservation{}, err
		}
		total, err = plus(total, maximum)
		if err != nil {
			return Reservation{}, err
		}
		if !within(total, limit.Caps) {
			return Reservation{}, ErrCapacity
		}
		result.Scopes = append(result.Scopes, ScopeSnapshot{Scope: scope, Version: limit.Version, PeriodStart: start})
	}
	for _, snapshot := range result.Scopes {
		limit, err := loadLimit(ctx, tx, l.OrgID, snapshot.Scope)
		if err != nil {
			return Reservation{}, err
		}
		held, err := plus(limit.Held, maximum)
		if err != nil {
			return Reservation{}, err
		}
		if err = setHeld(ctx, tx, l.OrgID, snapshot.Scope, held); err != nil {
			return Reservation{}, err
		}
	}
	b, _ := json.Marshal(result)
	_, err = tx.Exec(ctx, `INSERT INTO budget_reservations(org_id,id,operation_id,repository_id,task_id,job_id,attempt_id,fingerprint,record,state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, l.OrgID, result.ID, q.OperationID, l.RepositoryID, l.TaskID, l.JobID, l.AttemptID, fingerprint(l, q), b, result.State, now)
	return result, err
}

func setHeld(ctx context.Context, tx pgx.Tx, org string, scope Scope, held Amount) error {
	b, _ := json.Marshal(held)
	_, err := tx.Exec(ctx, `UPDATE budget_limits SET held=$4 WHERE org_id=$1 AND scope_kind=$2 AND scope_id=$3`, org, scope.Kind, scope.ID, b)
	return err
}
func loadReservation(ctx context.Context, tx pgx.Tx, org, id string) (Reservation, error) {
	var r Reservation
	var b []byte
	if !auth.ValidID(id) {
		return r, ErrInvalid
	}
	if err := lockOrg(ctx, tx, org); err != nil {
		return r, err
	}
	err := tx.QueryRow(ctx, `SELECT record FROM budget_reservations WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrRevoked
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}
func saveReservation(ctx context.Context, tx pgx.Tx, r Reservation) error {
	b, _ := json.Marshal(r)
	_, err := tx.Exec(ctx, `UPDATE budget_reservations SET record=$3,state=$4 WHERE org_id=$1 AND id=$2`, r.Lease.OrgID, r.ID, b, r.State)
	return err
}

func (s *Service) CheckTx(ctx context.Context, tx pgx.Tx, l Lease, id string) (Reservation, error) {
	connection, campaign, modelRoute, err := s.validateLease(ctx, tx, l)
	if err != nil {
		return Reservation{}, err
	}
	r, err := loadReservation(ctx, tx, l.OrgID, id)
	if err != nil {
		return r, err
	}
	if fingerprint(r.Lease, r.Quote) != fingerprint(l, r.Quote) || r.ConnectionID != connection || r.CampaignID != campaign || r.Route.Name != modelRoute || r.State != "reserved" {
		return r, ErrConflict
	}
	route, err := loadRoute(ctx, tx, l.OrgID, connection, r.Route.Model, r.Route.Name)
	if err != nil {
		return r, err
	}
	if route.Version != r.Route.Version || route.Paused {
		return r, ErrRevoked
	}
	version, err := currentConnection(ctx, tx, l.OrgID, connection, route)
	if err != nil {
		return r, err
	}
	if version != r.ConnectionVersion {
		return r, ErrRevoked
	}
	derived, err := scopes(ctx, tx, l.OrgID, l.RepositoryID, connection, campaign)
	if err != nil {
		return r, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return r, err
	}
	index := 0
	for _, scope := range derived {
		limit, err := loadLimit(ctx, tx, l.OrgID, scope)
		if errors.Is(err, pgx.ErrNoRows) {
			if scope.Kind == "organisation" || scope.Kind == "campaign" {
				return r, ErrUnknown
			}
			continue
		}
		if err != nil {
			return r, err
		}
		if index >= len(r.Scopes) || r.Scopes[index].Scope != scope || r.Scopes[index].Version != limit.Version || limit.Paused {
			return r, ErrRevoked
		}
		start, err := periodStart(limit, now)
		if err != nil {
			return r, err
		}
		if !start.Equal(r.Scopes[index].PeriodStart) {
			return r, ErrRevoked
		}
		spent, err := loadSpend(ctx, tx, l.OrgID, scope, start)
		if err != nil {
			return r, err
		}
		total, err := plus(spent, limit.Held)
		if err != nil {
			return r, err
		}
		if !within(total, limit.Caps) {
			return r, ErrCapacity
		}
		index++
	}
	if index != len(r.Scopes) {
		return r, ErrRevoked
	}
	return r, nil
}

func (s *Service) MarkDispatchedTx(ctx context.Context, tx pgx.Tx, l Lease, id string) (Reservation, error) {
	r, err := s.CheckTx(ctx, tx, l, id)
	if err != nil {
		return r, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return r, err
	}
	r.State = "dispatched"
	r.DispatchedAt = &now
	err = saveReservation(ctx, tx, r)
	return r, err
}

func (s *Service) MarkUnknownTx(ctx context.Context, tx pgx.Tx, org, id, reference string) (Reservation, error) {
	r, err := loadReservation(ctx, tx, org, id)
	if err != nil {
		return r, err
	}
	if reference == "" || len(reference) > 1000 {
		return r, ErrInvalid
	}
	if r.State == "unknown" {
		return r, nil
	}
	if r.State != "dispatched" {
		return r, ErrConflict
	}
	r.State = "unknown"
	r.Reference = reference
	err = saveReservation(ctx, tx, r)
	return r, err
}

func (s *Service) SettleTx(ctx context.Context, tx pgx.Tx, org, id string, settlement Settlement) (Reservation, error) {
	if !settlement.Known {
		return s.MarkUnknownTx(ctx, tx, org, id, settlement.Reference)
	}
	r, err := loadReservation(ctx, tx, org, id)
	if err != nil {
		return r, err
	}
	if !validAmount(settlement.Actual) || settlement.Actual.Concurrency != 0 || settlement.Reference == "" || len(settlement.Reference) > 1000 || r.Route.Mode == "quota" && settlement.Actual.MicroUSD != 0 {
		return r, ErrInvalid
	}
	if r.State == "settled" {
		if r.Actual != nil && *r.Actual == settlement.Actual && r.Reference == settlement.Reference {
			return r, nil
		}
		return r, ErrConflict
	}
	if r.State != "dispatched" && r.State != "unknown" && r.State != "cancelled" {
		return r, ErrConflict
	}
	maximum := r.Maximum
	if r.State == "cancelled" {
		maximum = Amount{}
	}
	r.Debt = excess(settlement.Actual, maximum)
	for _, scope := range r.Scopes {
		limit, err := loadLimit(ctx, tx, org, scope.Scope)
		if err != nil {
			return r, err
		}
		held, err := minus(limit.Held, maximum)
		if err != nil {
			return r, err
		}
		spent, err := loadSpend(ctx, tx, org, scope.Scope, scope.PeriodStart)
		if err != nil {
			return r, err
		}
		spent, err = plus(spent, settlement.Actual)
		if err != nil {
			return r, err
		}
		if err = setHeld(ctx, tx, org, scope.Scope, held); err != nil {
			return r, err
		}
		b, _ := json.Marshal(spent)
		if _, err = tx.Exec(ctx, `INSERT INTO budget_spend(org_id,scope_kind,scope_id,period_start,amount) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,scope_kind,scope_id,period_start) DO UPDATE SET amount=EXCLUDED.amount`, org, scope.Scope.Kind, scope.Scope.ID, scope.PeriodStart, b); err != nil {
			return r, err
		}
		if r.Debt != (Amount{}) {
			if _, err = tx.Exec(ctx, `UPDATE budget_limits SET paused=true,version=version+1 WHERE org_id=$1 AND scope_kind=$2 AND scope_id=$3`, org, scope.Scope.Kind, scope.Scope.ID); err != nil {
				return r, err
			}
		}
	}
	r.State = "settled"
	r.Actual = &settlement.Actual
	r.Reference = settlement.Reference
	if r.Debt != (Amount{}) {
		if err = writeAudit(ctx, tx, org, "budget-controller", "budget.paused", r.ID, r.Quote.OperationID, map[string]any{"debt": r.Debt, "reference": settlement.Reference}); err != nil {
			return r, err
		}
	}
	err = saveReservation(ctx, tx, r)
	return r, err
}

func (s *Service) CancelTx(ctx context.Context, tx pgx.Tx, org, id, unusedEvidence string) (Reservation, error) {
	r, err := loadReservation(ctx, tx, org, id)
	if err != nil {
		return r, err
	}
	if r.State == "cancelled" {
		return r, nil
	}
	if r.State == "settled" {
		return r, ErrConflict
	}
	if r.State != "reserved" && unusedEvidence == "" {
		return r, ErrUnknown
	}
	if len(unusedEvidence) > 1000 {
		return r, ErrInvalid
	}
	for _, scope := range r.Scopes {
		limit, err := loadLimit(ctx, tx, org, scope.Scope)
		if err != nil {
			return r, err
		}
		held, err := minus(limit.Held, r.Maximum)
		if err != nil {
			return r, err
		}
		if err = setHeld(ctx, tx, org, scope.Scope, held); err != nil {
			return r, err
		}
	}
	r.State = "cancelled"
	r.Reference = unusedEvidence
	err = saveReservation(ctx, tx, r)
	return r, err
}

func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Reservation, error) {
	var result Reservation
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var b []byte
		var repo string
		if !auth.ValidID(id) {
			return ErrInvalid
		}
		err := tx.QueryRow(ctx, `SELECT repository_id::text,record FROM budget_reservations WHERE org_id=$1 AND id=$2`, org, id).Scan(&repo, &b)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrForbidden
		}
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		return json.Unmarshal(b, &result)
	})
	return result, err
}

func (s *Service) SettleUnknownAtMaximumTx(ctx context.Context, tx pgx.Tx, org, id, reference string) error {
	r, err := loadReservation(ctx, tx, org, id)
	if err != nil || r.State != "unknown" && r.State != "dispatched" {
		return err
	}
	actual := r.Maximum
	actual.Concurrency = 0
	_, err = s.SettleTx(ctx, tx, org, id, Settlement{Known: true, Actual: actual, Reference: reference})
	return err
}
