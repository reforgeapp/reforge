package discovery

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/forge"
	"reforge/internal/maintenance/detectors"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

var ErrStale = errors.New("discovery evidence changed or expired; refresh discovery")
var ErrDuplicate = errors.New("overlapping maintenance work exists; review existing change")
var ErrBlocked = errors.New("finding requires reviewed ownership or complete evidence")

type Reader interface {
	Read(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error)
	SourceReader(string, string, func(context.Context, pgx.Tx, connections.Connection) error) source.Reader
}
type BotIdentity struct {
	Kind    string `json:"kind"`
	ActorID string `json:"actor_id"`
}
type Config struct {
	RepositoryID   string        `json:"repository_id"`
	TrustedBots    []BotIdentity `json:"trusted_bots"`
	MergeAuthority string        `json:"merge_authority"`
	Version        int64         `json:"version"`
}
type Evidence struct {
	Provenance        string                 `json:"provenance"`
	ConnectionID      string                 `json:"connection_id"`
	ConnectionVersion int64                  `json:"connection_version"`
	ConfigVersion     int64                  `json:"config_version"`
	HeadSHA           string                 `json:"head_sha"`
	TargetSHA         string                 `json:"target_sha"`
	TargetBranch      string                 `json:"target_branch"`
	Change            *forge.Change          `json:"change,omitempty"`
	Checks            []forge.Check          `json:"checks"`
	Dependencies      []detectors.Dependency `json:"dependencies"`
	Bot               string                 `json:"bot"`
	Behind            int                    `json:"behind,omitempty"`
	Ownership         string                 `json:"ownership"`
	HeadOwnership     string                 `json:"head_ownership"`
	MergeBlockers     []string               `json:"merge_blockers"`
	BotConfig         detectors.BotConfig    `json:"bot_config"`
	Complete          bool                   `json:"complete"`
	Blockers          []string               `json:"blockers"`
	AdvisoryID        string                 `json:"advisory_id,omitempty"`
	ReferenceURL      string                 `json:"reference_url,omitempty"`
}
type Observation struct {
	RepositoryID string   `json:"repository_id"`
	Source       string   `json:"source"`
	SourceID     string   `json:"source_id"`
	Category     string   `json:"category"`
	Severity     string   `json:"severity"`
	Title        string   `json:"title"`
	Evidence     Evidence `json:"evidence"`
}
type Finding struct {
	ID    string `json:"id"`
	OrgID string `json:"org_id"`
	Observation
	Fingerprint    string     `json:"fingerprint"`
	EvidenceDigest string     `json:"evidence_digest"`
	State          string     `json:"state"`
	Reason         string     `json:"reason"`
	AssignedTo     string     `json:"assigned_to,omitempty"`
	SnoozeUntil    *time.Time `json:"snooze_until,omitempty"`
	SupersededBy   string     `json:"superseded_by,omitempty"`
	Version        int64      `json:"version"`
	FirstSeen      time.Time  `json:"first_seen"`
	LastSeen       time.Time  `json:"last_seen"`
}
type Update struct {
	Action      string     `json:"action"`
	Reason      string     `json:"reason"`
	AssignedTo  string     `json:"assigned_to"`
	SnoozeUntil *time.Time `json:"snooze_until"`
}
type Filter struct{ RepositoryID, State, Category, Severity, Query string }
type Scan struct {
	RepositoryID string     `json:"repository_id"`
	State        string     `json:"state"`
	Reason       string     `json:"reason"`
	Version      int64      `json:"version"`
	ObservedAt   *time.Time `json:"observed_at,omitempty"`
}

type AdvisoryInput struct {
	RepositoryID  string `json:"repository_id"`
	AdvisoryID    string `json:"advisory_id"`
	Title         string `json:"title"`
	Severity      string `json:"severity"`
	ReferenceURL  string `json:"reference_url"`
	Path          string `json:"path"`
	Package       string `json:"package"`
	Ecosystem     string `json:"ecosystem"`
	AffectedRange string `json:"affected_range"`
	CommitSHA     string `json:"commit_sha"`
}
