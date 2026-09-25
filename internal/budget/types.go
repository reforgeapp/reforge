package budget

import (
	"errors"
	"time"
)

var defaultConcurrency int64 = 1

var (
	ErrCapacity = errors.New("budget capacity unavailable")
	ErrUnknown  = errors.New("budget or pricing not configured")
	ErrConflict = errors.New("budget operation conflict")
	ErrRevoked  = errors.New("budget authority revoked")
	ErrInvalid  = errors.New("invalid budget input")
)

type Scope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type Amount struct {
	MicroUSD     int64 `json:"micro_usd"`
	Tokens       int64 `json:"tokens"`
	Milliseconds int64 `json:"milliseconds"`
	Requests     int64 `json:"requests"`
	Concurrency  int64 `json:"concurrency"`
}
type Caps struct {
	MicroUSD     *int64 `json:"micro_usd"`
	Tokens       *int64 `json:"tokens"`
	Milliseconds *int64 `json:"milliseconds"`
	Requests     *int64 `json:"requests"`
	Concurrency  *int64 `json:"concurrency"`
}
type Limit struct {
	Scope   Scope     `json:"scope"`
	Period  string    `json:"period"`
	Start   time.Time `json:"start,omitempty"`
	End     time.Time `json:"end,omitempty"`
	Caps    Caps      `json:"caps"`
	Paused  bool      `json:"paused"`
	Version int64     `json:"version"`
	Held    Amount    `json:"held"`
	Spent   Amount    `json:"spent"`
}
type Route struct {
	ConnectionID             string `json:"connection_id"`
	Model                    string `json:"model"`
	Name                     string `json:"name"`
	Mode                     string `json:"mode"`
	PricingVersion           string `json:"pricing_version"`
	InputMicroUSDPerMillion  int64  `json:"input_micro_usd_per_million"`
	OutputMicroUSDPerMillion int64  `json:"output_micro_usd_per_million"`
	RequestMicroUSD          int64  `json:"request_micro_usd"`
	MaxInputTokens           int64  `json:"max_input_tokens"`
	MaxOutputTokens          int64  `json:"max_output_tokens"`
	MaxMilliseconds          int64  `json:"max_milliseconds"`
	MaxRequests              int64  `json:"max_requests"`
	Qualified                bool   `json:"qualified"`
	QualificationRef         string `json:"qualification_ref"`
	Paused                   bool   `json:"paused"`
	Version                  int64  `json:"version"`
}
type Lease struct {
	OrgID        string    `json:"org_id"`
	RepositoryID string    `json:"repository_id"`
	TaskID       string    `json:"task_id"`
	JobID        string    `json:"job_id"`
	AttemptID    string    `json:"attempt_id"`
	OperationID  string    `json:"operation_id"`
	WorkerID     string    `json:"worker_id"`
	Fence        int64     `json:"fence"`
	PolicyHash   string    `json:"policy_hash"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type Quote struct {
	OperationID     string `json:"operation_id"`
	Model           string `json:"model"`
	Route           string `json:"route"`
	RouteVersion    int64  `json:"route_version"`
	InputTokens     int64  `json:"input_tokens"`
	MaxOutputTokens int64  `json:"max_output_tokens"`
	MaxMilliseconds int64  `json:"max_milliseconds"`
	MaxRequests     int64  `json:"max_requests"`
}
type ScopeSnapshot struct {
	Scope       Scope     `json:"scope"`
	Version     int64     `json:"version"`
	PeriodStart time.Time `json:"period_start"`
}
type Reservation struct {
	ID                string          `json:"id"`
	Lease             Lease           `json:"lease"`
	Quote             Quote           `json:"quote"`
	ConnectionID      string          `json:"connection_id"`
	ConnectionVersion int64           `json:"connection_version"`
	CampaignID        string          `json:"campaign_id,omitempty"`
	Route             Route           `json:"route"`
	Scopes            []ScopeSnapshot `json:"scopes"`
	Maximum           Amount          `json:"maximum"`
	Actual            *Amount         `json:"actual,omitempty"`
	Debt              Amount          `json:"debt"`
	State             string          `json:"state"`
	Reference         string          `json:"reference,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	DispatchedAt      *time.Time      `json:"dispatched_at,omitempty"`
}

type Settlement struct {
	Actual    Amount `json:"actual"`
	Reference string `json:"reference"`
	Known     bool   `json:"known"`
}
