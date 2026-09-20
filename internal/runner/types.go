package runner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/workflow"
)

var ErrUnsupported = errors.New("runner operation is not registered")

type Pool struct {
	ID            string   `json:"id"`
	OrgID         string   `json:"org_id"`
	Name          string   `json:"name"`
	State         string   `json:"state"`
	RepositoryIDs []string `json:"repository_ids"`
	Version       int64    `json:"version"`
}
type PoolInput struct {
	Name          string   `json:"name"`
	State         string   `json:"state"`
	RepositoryIDs []string `json:"repository_ids"`
}
type Runner struct {
	ID                  string    `json:"id"`
	OrgID               string    `json:"org_id"`
	PoolID              string    `json:"pool_id"`
	Name                string    `json:"name"`
	State               string    `json:"state"`
	Version             int64     `json:"version"`
	CredentialExpiresAt time.Time `json:"credential_expires_at"`
}
type Credential struct {
	Token     string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Runner    Runner    `json:"runner"`
}

func (c Credential) String() string       { return "runner credential [redacted]" }
func (c Credential) GoString() string     { return c.String() }
func (c Credential) LogValue() slog.Value { return slog.StringValue(c.String()) }

type Assignment struct {
	Credential Credential     `json:"-"`
	Lease      workflow.Lease `json:"lease"`
	Task       workflow.Task  `json:"task"`
}

func (a Assignment) String() string   { return "runner assignment " + a.Task.ID }
func (a Assignment) GoString() string { return a.String() }

type Heartbeat struct {
	Lease  workflow.Lease `json:"lease"`
	Stop   bool           `json:"stop"`
	Reason string         `json:"reason,omitempty"`
}
type Operation struct {
	Lease        workflow.Lease
	Task         workflow.Task
	RunnerID     string
	PoolID       string
	ConnectionID string
	Input        json.RawMessage
}
type FixedOperation func(context.Context, pgx.Tx, Operation) (json.RawMessage, error)
