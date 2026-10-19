package deployment

import (
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/policy"
	"time"
)

type Qualification struct {
	Provider                 string    `json:"provider"`
	ServerVersion            string    `json:"server_version"`
	ConnectionVersion        int64     `json:"connection_version"`
	EvidenceReference        string    `json:"evidence_reference"`
	EvidenceSHA256           string    `json:"evidence_sha256"`
	VerifiedAt               time.Time `json:"verified_at"`
	ExpiresAt                time.Time `json:"expires_at"`
	PinnedInputs             bool      `json:"pinned_inputs"`
	NativeEnforcement        bool      `json:"native_enforcement"`
	EnvironmentSerialization bool      `json:"environment_serialization"`
	NoBypass                 bool      `json:"no_bypass"`
}

type Workflow struct {
	ID           string            `json:"id"`
	Path         string            `json:"path"`
	Ref          string            `json:"ref"`
	SHA          string            `json:"sha"`
	ConfigSHA256 string            `json:"config_sha256"`
	Inputs       map[string]string `json:"inputs"`
}

type Configuration struct {
	NativeEnvironment     string        `json:"native_environment"`
	Environment           string        `json:"environment"`
	RepositoryID          string        `json:"repository_id"`
	Version               int64         `json:"version"`
	Enabled               bool          `json:"enabled"`
	Mode                  string        `json:"mode"`
	Workflow              Workflow      `json:"workflow"`
	RecoveryWorkflow      *Workflow     `json:"recovery_workflow,omitempty"`
	ProvenancePublicKey   string        `json:"provenance_public_key"`
	HealthPublicKey       string        `json:"health_public_key"`
	HealthChecks          []string      `json:"health_checks"`
	ObservationSeconds    int64         `json:"observation_seconds"`
	MaxEvidenceAgeSeconds int64         `json:"max_evidence_age_seconds"`
	DeadlineSeconds       int64         `json:"deadline_seconds"`
	Qualification         Qualification `json:"qualification"`
}

type Provenance struct {
	OrgID          string    `json:"org_id"`
	RepositoryID   string    `json:"repository_id"`
	SourceSHA      string    `json:"source_sha"`
	ArtifactDigest string    `json:"artifact_digest"`
	BuildID        string    `json:"build_id"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type SignedProvenance struct {
	Document  Provenance `json:"document"`
	Signature string     `json:"signature"`
}

type PreviewRequest struct {
	ChangeID            string           `json:"change_id"`
	SourceSHA           string           `json:"source_sha"`
	ArtifactDigest      string           `json:"artifact_digest"`
	Provenance          SignedProvenance `json:"provenance"`
	RecoveryOf          string           `json:"recovery_of,omitempty"`
	RestoreDeploymentID string           `json:"restore_deployment_id,omitempty"`
}

type Gate struct {
	ID                   string                `json:"id"`
	Environment          string                `json:"environment"`
	RepositoryID         string                `json:"repository_id"`
	ConnectionID         string                `json:"connection_id"`
	ConnectionVersion    int64                 `json:"connection_version"`
	ConfigurationVersion int64                 `json:"configuration_version"`
	Request              PreviewRequest        `json:"request"`
	Pipeline             forge.PipelineRequest `json:"pipeline"`
	Native               forge.DeploymentGates `json:"native"`
	Binding              policy.Binding        `json:"binding"`
	Decision             policy.Result         `json:"decision"`
	Blockers             []string              `json:"blockers"`
	ExpiresAt            time.Time             `json:"expires_at"`
}

type Operation struct {
	CancelRequested bool                    `json:"cancel_requested"`
	CancelState     string                  `json:"cancel_state"`
	FinishedAt      *time.Time              `json:"finished_at,omitempty"`
	ID              string                  `json:"id"`
	Environment     string                  `json:"environment"`
	RepositoryID    string                  `json:"repository_id"`
	GateID          string                  `json:"gate_id"`
	State           string                  `json:"state"`
	Reason          string                  `json:"reason"`
	Version         int64                   `json:"version"`
	RequestedBy     string                  `json:"requested_by"`
	Native          *forge.DeploymentStatus `json:"native,omitempty"`
	RecoveryOf      string                  `json:"recovery_of,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

type HealthReport struct {
	OrgID                string          `json:"org_id"`
	DeploymentID         string          `json:"deployment_id"`
	Environment          string          `json:"environment"`
	ConfigurationVersion int64           `json:"configuration_version"`
	SourceSHA            string          `json:"source_sha"`
	ArtifactDigest       string          `json:"artifact_digest"`
	RunID                string          `json:"run_id"`
	RunAttempt           int64           `json:"run_attempt"`
	Revision             string          `json:"revision"`
	Healthy              bool            `json:"healthy"`
	Checks               map[string]bool `json:"checks"`
	ObservedAt           time.Time       `json:"observed_at"`
	Nonce                string          `json:"nonce"`
}
