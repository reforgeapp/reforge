package inventory

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
)

var ErrStale = errors.New("inventory observation or lease is stale")
var ErrIncomplete = errors.New("provider inventory is incomplete")
var ErrBusy = errors.New("inventory queue is full")

type Reader interface {
	Read(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error)
}
type Decoder func(string, string, http.Header, []byte) (forge.Event, error)
type SyncInput struct {
	ConnectionID string `json:"connection_id"`
	Namespace    string `json:"namespace"`
}
type ImportInput struct {
	All       bool     `json:"all"`
	NativeIDs []string `json:"native_ids"`
	TeamIDs   []string `json:"team_ids"`
}
type importPlan struct {
	ImportInput
	TeamVersions map[string]int64 `json:"team_versions"`
}
type Job struct {
	ID                string     `json:"id"`
	OrgID             string     `json:"org_id"`
	ConnectionID      string     `json:"connection_id"`
	ConnectionVersion int64      `json:"connection_version"`
	Namespace         string     `json:"namespace"`
	Kind              string     `json:"kind"`
	RepositoryID      string     `json:"repository_id,omitempty"`
	ParentID          string     `json:"parent_id,omitempty"`
	RequestedBy       string     `json:"-"`
	Input             []byte     `json:"-"`
	State             string     `json:"state"`
	Reason            string     `json:"reason,omitempty"`
	Cursor            string     `json:"-"`
	Phase             string     `json:"-"`
	Pages             int        `json:"pages"`
	Processed         int        `json:"processed"`
	Failures          int        `json:"failures"`
	Fence             int64      `json:"-"`
	LeaseOwner        string     `json:"-"`
	LeaseUntil        *time.Time `json:"-"`
	AvailableAt       time.Time  `json:"available_at"`
	Version           int64      `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
type Repository struct {
	domain.Repository
	SyncState         string     `json:"sync_state"`
	SyncReason        string     `json:"sync_reason,omitempty"`
	ConnectionVersion int64      `json:"connection_version"`
	ChangesObservedAt *time.Time `json:"changes_observed_at,omitempty"`
}
type Webhook struct {
	ID           string `json:"id"`
	ConnectionID string `json:"connection_id"`
	Version      int64  `json:"version"`
	Path         string `json:"path"`
	Revoked      bool   `json:"revoked"`
}
type IssuedWebhook struct {
	Webhook
	Secret string `json:"secret"`
}

func (w IssuedWebhook) String() string       { return "webhook credential [redacted]" }
func (w IssuedWebhook) GoString() string     { return w.String() }
func (w IssuedWebhook) LogValue() slog.Value { return slog.StringValue(w.String()) }

type RepositoryFilter struct {
	Query    string
	Provider string
	TeamID   string
	Status   string
}
type ChangePage struct {
	domain.Page[forge.Change]
	SnapshotState     string     `json:"snapshot_state"`
	ObservedAt        *time.Time `json:"observed_at,omitempty"`
	ConnectionVersion int64      `json:"connection_version"`
}
