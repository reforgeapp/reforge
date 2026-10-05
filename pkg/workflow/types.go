package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
)

var ErrNoWork = errors.New("no eligible work")
var ErrFence = errors.New("lease or fencing token is no longer valid")
var ErrPaused = errors.New("workflow scope is paused")
var ErrPolicy = errors.New("policy changed or does not permit this action")
var ErrReconciliation = errors.New("external outcome must be reconciled before retry")
var ErrCursor = errors.New("event cursor expired; refetch current state")

type PolicyCheck func(context.Context, pgx.Tx, Task, string) (string, error)

type EnqueueInput struct {
	RepositoryID      string `json:"repository_id"`
	Recipe            string `json:"recipe"`
	RecipeVersion     string `json:"recipe_version"`
	TargetBranch      string `json:"target_branch"`
	ModelConnectionID string `json:"model_connection_id,omitempty"`
	ModelRoute        string `json:"model_route"`
	CampaignID        string `json:"campaign_id,omitempty"`
	RunnerPoolID      string `json:"runner_pool_id,omitempty"`
	PolicyHash        string `json:"policy_hash"`
	IdempotencyKey    string `json:"idempotency_key"`
	MaxAttempts       int    `json:"max_attempts"`
	Priority          int    `json:"priority"`
}
type Task struct {
	ID                    string           `json:"id"`
	OrgID                 string           `json:"org_id"`
	RepositoryID          string           `json:"repository_id"`
	OperationID           string           `json:"operation_id"`
	Recipe                string           `json:"recipe"`
	RecipeVersion         string           `json:"recipe_version"`
	TargetBranch          string           `json:"target_branch"`
	ModelConnectionID     string           `json:"model_connection_id,omitempty"`
	ModelRoute            string           `json:"model_route"`
	CampaignID            string           `json:"campaign_id,omitempty"`
	RunnerPoolID          string           `json:"runner_pool_id,omitempty"`
	PolicyHash            string           `json:"policy_hash"`
	StartingPolicyHash    string           `json:"starting_policy_hash"`
	State                 domain.TaskState `json:"state"`
	Reason                string           `json:"reason"`
	Version               int64            `json:"version"`
	CancelVersion         int64            `json:"cancel_version"`
	CancellationRequested bool             `json:"cancellation_requested"`
	MaxAttempts           int              `json:"max_attempts"`
	CreatedAt             time.Time        `json:"created_at"`
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
type Completion struct {
	Outcome      string `json:"outcome"`
	Retryable    bool   `json:"retryable"`
	Reason       string `json:"reason,omitempty"`
	RetryAfterMS int64  `json:"retry_after_ms,omitempty"`
}
type Pause struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Paused  bool   `json:"paused"`
	Version int64  `json:"version"`
}
type EventPage struct {
	ScanAfter int64          `json:"-"`
	Items     []domain.Event `json:"items"`
	Cursor    int64          `json:"cursor"`
	Complete  bool           `json:"complete"`
}
type Intent struct {
	ID            string          `json:"id"`
	OrgID         string          `json:"org_id"`
	TaskID        string          `json:"task_id"`
	RepositoryID  string          `json:"repository_id"`
	OperationID   string          `json:"operation_id"`
	Kind          string          `json:"kind"`
	State         string          `json:"state"`
	Payload       json.RawMessage `json:"payload"`
	DispatchCount int             `json:"dispatch_count"`
	Evidence      string          `json:"evidence"`
}
