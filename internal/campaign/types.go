package campaign

import (
	"context"
	"time"

	"reforge/internal/auth"
	"reforge/internal/deployment"
	"reforge/internal/gitops"
	"reforge/internal/maintenance/repair"
)

type Window struct {
	Weekdays    []int `json:"weekdays"`
	StartMinute int   `json:"start_minute"`
	EndMinute   int   `json:"end_minute"`
}

type MemberInput struct {
	RepositoryID string                     `json:"repository_id"`
	Repair       *repair.Input              `json:"repair,omitempty"`
	Environment  string                     `json:"environment,omitempty"`
	Pipeline     *deployment.PreviewRequest `json:"pipeline,omitempty"`
	GitOps       *gitops.PreviewRequest     `json:"gitops,omitempty"`
}

type Input struct {
	Name               string        `json:"name"`
	Kind               string        `json:"kind"`
	Selection          string        `json:"selection"`
	Members            []MemberInput `json:"members"`
	CanaryIDs          []string      `json:"canary_ids"`
	CanarySize         int           `json:"canary_size"`
	BatchSize          int           `json:"batch_size"`
	Concurrency        int           `json:"concurrency"`
	Success            string        `json:"success"`
	ObservationSeconds int64         `json:"observation_seconds"`
	FailureLimit       int           `json:"failure_limit"`
	FailurePercent     int           `json:"failure_percent"`
	Windows            []Window      `json:"windows"`
	NotBefore          *time.Time    `json:"not_before,omitempty"`
}

type Member struct {
	ObservedAt      time.Time         `json:"-"`
	ID              string            `json:"id"`
	RepositoryID    string            `json:"repository_id"`
	RepositoryName  string            `json:"repository_name"`
	Repositories    []string          `json:"repositories"`
	Group           string            `json:"group"`
	Canary          bool              `json:"canary"`
	Input           MemberInput       `json:"input"`
	Pins            map[string]string `json:"pins"`
	State           string            `json:"state"`
	Reason          string            `json:"reason"`
	Stage           int               `json:"stage"`
	Halt            bool              `json:"halt"`
	ResumeRequested bool              `json:"-"`
	ActionID        string            `json:"action_id,omitempty"`
	SucceededAt     *time.Time        `json:"succeeded_at,omitempty"`
}

type Preview struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	Input     Input     `json:"input"`
	Members   []Member  `json:"members"`
	Blockers  []string  `json:"blockers"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Counts struct {
	Total     int `json:"total"`
	Excluded  int `json:"excluded"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Unknown   int `json:"unknown"`
}

type Campaign struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	State          string     `json:"state"`
	Reason         string     `json:"reason"`
	Version        int64      `json:"version"`
	Stage          int        `json:"stage"`
	RequestedBy    string     `json:"requested_by"`
	Spec           Input      `json:"spec"`
	Counts         Counts     `json:"counts"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	GrantExpiresAt time.Time  `json:"grant_expires_at"`
	ObservingSince *time.Time `json:"observing_since,omitempty"`
}

type Execution struct {
	Halt                  bool
	ActionID              string
	State                 string
	Reason                string
	VerifiedWindowSeconds int64
}

type Executor interface {
	Dispatch(context.Context, auth.Session, string, Campaign, Member) (Execution, error)
	Observe(context.Context, auth.Session, string, Campaign, Member) (Execution, error)
	Cancel(context.Context, auth.Session, string, Campaign, Member) (Execution, error)
}
