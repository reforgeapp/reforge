package mergecontrol

import (
	"reforge/internal/forge"
	"reforge/internal/policy"
	"time"
)

type Snapshot = forge.MergeEvidence

type Gate struct {
	ReforgeEnforced      bool           `json:"reforge_enforced,omitempty"`
	Phase                string         `json:"phase"`
	Companions           []Companion    `json:"companions"`
	Paths                []string       `json:"paths"`
	ChangedLines         int64          `json:"changed_lines"`
	ConfigurationVersion int64          `json:"configuration_version"`
	ID                   string         `json:"id"`
	RepositoryID         string         `json:"repository_id"`
	ConnectionID         string         `json:"connection_id"`
	ConnectionVersion    int64          `json:"connection_version"`
	Method               string         `json:"method"`
	Snapshot             Snapshot       `json:"snapshot"`
	Decision             policy.Result  `json:"decision"`
	Binding              policy.Binding `json:"binding"`
	ExpiresAt            time.Time      `json:"expires_at"`
}

type Companion struct {
	TaskID   string `json:"task_id"`
	ChangeID string `json:"change_id"`
	HeadSHA  string `json:"head_sha"`
	MergeSHA string `json:"merge_sha"`
	State    string `json:"state"`
}

type Qualification struct {
	CIConfigSHA256     string    `json:"ci_config_sha256,omitempty"`
	Provider           string    `json:"provider"`
	ServerVersion      string    `json:"server_version"`
	ConnectionVersion  int64     `json:"connection_version"`
	InspectorVersion   int64     `json:"inspector_version"`
	EvidenceReference  string    `json:"evidence_reference"`
	EvidenceSHA256     string    `json:"evidence_sha256"`
	VerifiedAt         time.Time `json:"verified_at"`
	ExpiresAt          time.Time `json:"expires_at"`
	ExactHead          bool      `json:"exact_head"`
	StrictTarget       bool      `json:"strict_target"`
	QueueExecutionGate bool      `json:"queue_execution_gate"`
}

type Configuration struct {
	RepositoryID          string            `json:"repository_id"`
	Version               int64             `json:"version"`
	Enabled               bool              `json:"enabled"`
	InspectorConnectionID string            `json:"inspector_connection_id,omitempty"`
	CheckPublishers       map[string]string `json:"check_publishers"`
	CooperationReference  string            `json:"cooperation_reference"`
	Qualification         Qualification     `json:"qualification"`
}

type Operation struct {
	ID              string             `json:"id"`
	RepositoryID    string             `json:"repository_id"`
	GateID          string             `json:"gate_id"`
	RequestedGateID string             `json:"requested_gate_id"`
	NativeQueueID   string             `json:"native_queue_id,omitempty"`
	ChangeID        string             `json:"change_id"`
	State           string             `json:"state"`
	Reason          string             `json:"reason"`
	CancelRequested bool               `json:"cancel_requested"`
	NativeResult    *forge.MergeResult `json:"native_result,omitempty"`
	Version         int64              `json:"version"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type AuthorityBlocker struct{ Reason string }

func (b *AuthorityBlocker) Error() string { return b.Reason }

type Authority struct {
	Blockers               []string
	CIConfigSHA256         string
	ExecutionPublisher     string
	CompanionsBlocked      bool
	PathsVerified          bool
	ValidationHead         string
	ValidationTarget       string
	ValidationReference    string
	MergeControlled        bool
	CooperationVerified    bool
	Qualified              bool
	ExactHeadEnforced      bool
	QualificationReference string
	ReforgeEnforced        bool
	Paths                  []string
	Usage                  policy.Limits
}
