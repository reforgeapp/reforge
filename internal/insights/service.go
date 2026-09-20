package insights

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/domain"
	"strings"
	"time"
)

type Service struct{ auth *auth.Service }

func New(identity *auth.Service) *Service { return &Service{auth: identity} }

type Filter struct {
	RepositoryID, TeamID, Recipe, Provider, ConnectionID, State, ActorID, Action, Cursor string
	Since, Until                                                                         *time.Time
	Limit                                                                                int
}
type Usage struct {
	Reservation    budget.Reservation `json:"reservation"`
	Provider       string             `json:"provider"`
	Recipe         string             `json:"recipe"`
	RepositoryName string             `json:"repository_name"`
}
type Summary struct {
	Records               int64         `json:"records"`
	Settled               int64         `json:"settled"`
	Unknown               int64         `json:"unknown"`
	Reserved              int64         `json:"reserved"`
	Dispatched            int64         `json:"dispatched"`
	Cancelled             int64         `json:"cancelled"`
	EstimatedCostMicroUSD int64         `json:"estimated_cost_micro_usd"`
	KnownTokens           int64         `json:"known_tokens"`
	UnknownMaximum        budget.Amount `json:"unknown_maximum"`
	Held                  budget.Amount `json:"held"`
}
type Audit struct {
	ID           string          `json:"id"`
	ActorID      string          `json:"actor_id"`
	Action       string          `json:"action"`
	ObjectID     string          `json:"object_id"`
	RequestID    string          `json:"request_id"`
	RepositoryID string          `json:"repository_id,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	Data         json.RawMessage `json:"data"`
}

func validate(f Filter) error {
	for _, id := range []string{f.RepositoryID, f.TeamID, f.ConnectionID} {
		if id != "" && !auth.ValidID(id) {
			return auth.ErrInvalid
		}
	}
	if f.Limit < 1 || f.Limit > 100 || len(f.Recipe) > 128 || len(f.Provider) > 64 || len(f.State) > 32 || len(f.Action) > 128 || len(f.ActorID) > 256 || len(f.Cursor) > 256 {
		return auth.ErrInvalid
	}
	if f.Since != nil && f.Until != nil && !f.Since.Before(*f.Until) {
		return auth.ErrInvalid
	}
	return nil
}
func cursor(value string) (any, any, error) {
	if value == "" {
		return nil, nil, nil
	}
	raw, e := base64.RawURLEncoding.DecodeString(value)
	if e != nil {
		return nil, nil, auth.ErrInvalid
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 || !auth.ValidID(parts[1]) {
		return nil, nil, auth.ErrInvalid
	}
	at, e := time.Parse(time.RFC3339Nano, parts[0])
	if e != nil {
		return nil, nil, auth.ErrInvalid
	}
	return at, parts[1], nil
}
func next(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}
func scope(ctx context.Context, tx pgx.Tx, org string, a domain.Actor, f Filter) error {
	if f.RepositoryID != "" && !auth.CanReadRepository(a, f.RepositoryID) {
		return auth.ErrForbidden
	}
	if f.TeamID != "" && a.Role != domain.Owner && a.Role != domain.Admin {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_memberships WHERE org_id=$1 AND user_id=$2 AND team_id=$3)`, org, a.UserID, f.TeamID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return auth.ErrForbidden
		}
	}
	return nil
}
