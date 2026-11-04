package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ID = string

func StableID(parts ...string) ID {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func NewID() ID {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

type CapabilityState string

const (
	Supported   CapabilityState = "supported"
	Unsupported CapabilityState = "unsupported"
	Unknown     CapabilityState = "unknown"
)

type Capability struct {
	State       CapabilityState `json:"state"`
	Scope       string          `json:"scope"`
	Reason      string          `json:"reason"`
	Source      string          `json:"source"`
	Version     string          `json:"version"`
	LastChecked time.Time       `json:"last_checked"`
}

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Retryable bool   `json:"retryable"`
	Details   any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

type ProviderError struct {
	Kind       string
	Message    string
	RetryAfter time.Duration
	Uncertain  bool
}

func (e *ProviderError) Error() string { return fmt.Sprintf("%s: %s", e.Kind, e.Message) }

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	Complete   bool   `json:"complete"`
}

type Role string

const (
	Owner      Role = "owner"
	Admin      Role = "admin"
	Maintainer Role = "maintainer"
	Reviewer   Role = "reviewer"
	Viewer     Role = "viewer"
)

type Actor struct {
	UserID          ID   `json:"user_id"`
	OrgID           ID   `json:"org_id"`
	Role            Role `json:"role"`
	TeamIDs         []ID `json:"team_ids"`
	RepositoryIDs   []ID `json:"repository_ids"`
	AllRepositories bool `json:"all_repositories"`
	SessionID       ID   `json:"-"`
}

type Organisation struct {
	ID      ID     `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug,omitempty"`
	Version int64  `json:"version"`
	Paused  bool   `json:"paused"`
}

type Repository struct {
	ID            ID         `json:"id"`
	OrgID         ID         `json:"org_id"`
	ConnectionID  ID         `json:"connection_id"`
	NativeID      string     `json:"native_id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	DefaultBranch string     `json:"default_branch"`
	Provider      string     `json:"provider"`
	Archived      bool       `json:"archived"`
	Paused        bool       `json:"paused"`
	Accessible    bool       `json:"accessible"`
	TeamIDs       []ID       `json:"team_ids"`
	LastSyncedAt  *time.Time `json:"last_synced_at"`
	Version       int64      `json:"version"`
}

type TaskState string

const (
	TaskQueued      TaskState = "queued"
	TaskReproducing TaskState = "reproducing"
	TaskPlanning    TaskState = "planning"
	TaskRepairing   TaskState = "repairing"
	TaskValidating  TaskState = "validating"
	TaskPublishing  TaskState = "publishing"
	TaskCompleted   TaskState = "completed"
	TaskBlocked     TaskState = "blocked"
	TaskFailed      TaskState = "failed"
	TaskCancelling  TaskState = "cancelling"
	TaskCancelled   TaskState = "cancelled"
	TaskReconciling TaskState = "reconciling"
)

type ChangeState string

const (
	ChangeDraft          ChangeState = "draft"
	ChangeOpen           ChangeState = "open"
	ChangeEvaluating     ChangeState = "evaluating"
	ChangeEligible       ChangeState = "eligible"
	ChangeMergeRequested ChangeState = "merge_requested"
	ChangeQueued         ChangeState = "queued"
	ChangeMerged         ChangeState = "merged"
	ChangeBlocked        ChangeState = "blocked"
	ChangeReconciling    ChangeState = "reconciling"
	ChangeClosed         ChangeState = "closed"
)

type DeploymentState string

const (
	DeploymentPlanned           DeploymentState = "planned"
	DeploymentAwaitingGates     DeploymentState = "awaiting_gates"
	DeploymentEligible          DeploymentState = "eligible"
	DeploymentRequested         DeploymentState = "requested"
	DeploymentRunning           DeploymentState = "running"
	DeploymentVerifying         DeploymentState = "verifying"
	DeploymentHealthy           DeploymentState = "healthy"
	DeploymentBlocked           DeploymentState = "blocked"
	DeploymentReconciling       DeploymentState = "reconciling"
	DeploymentFailed            DeploymentState = "failed"
	DeploymentUnverified        DeploymentState = "completed_unverified"
	DeploymentRecoveryRequested DeploymentState = "recovery_requested"
	DeploymentRecovering        DeploymentState = "recovering"
	DeploymentRecovered         DeploymentState = "recovered"
	DeploymentRecoveryFailed    DeploymentState = "recovery_failed"
	DeploymentCancelled         DeploymentState = "cancelled"
)

type Decision struct {
	Outcome         string   `json:"outcome"`
	PolicyHash      string   `json:"policy_hash"`
	Blockers        []string `json:"blockers"`
	RequiredActions []string `json:"required_actions"`
	Rules           []string `json:"rules"`
}

type Event struct {
	ID               int64           `json:"id"`
	OrgID            ID              `json:"org_id"`
	RepositoryID     ID              `json:"repository_id,omitempty"`
	Type             string          `json:"type"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      ID              `json:"aggregate_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	OccurredAt       time.Time       `json:"occurred_at"`
	RequestID        ID              `json:"request_id"`
	DataVersion      int             `json:"data_version"`
	Data             json.RawMessage `json:"data"`
}
