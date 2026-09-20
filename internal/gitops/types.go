package gitops

import (
	"reforge/internal/deployment"
	"reforge/internal/forge"
	"reforge/internal/policy"
	"time"
)

type Configuration struct {
	Environment           string   `json:"environment"`
	SourceRepositoryID    string   `json:"source_repository_id"`
	DeliveryRepositoryID  string   `json:"delivery_repository_id"`
	Version               int64    `json:"version"`
	Enabled               bool     `json:"enabled"`
	TargetBranch          string   `json:"target_branch"`
	ManifestPath          string   `json:"manifest_path"`
	Pointer               string   `json:"pointer"`
	ImageRepository       string   `json:"image_repository"`
	ProvenancePublicKey   string   `json:"provenance_public_key"`
	HealthPublicKey       string   `json:"health_public_key"`
	HealthChecks          []string `json:"health_checks"`
	ObservationSeconds    int64    `json:"observation_seconds"`
	MaxEvidenceAgeSeconds int64    `json:"max_evidence_age_seconds"`
	DeadlineSeconds       int64    `json:"deadline_seconds"`
	RecoveryAllowed       bool     `json:"recovery_allowed"`
}

type PreviewRequest struct {
	ChangeID           string                      `json:"change_id"`
	SourceSHA          string                      `json:"source_sha"`
	ArtifactDigest     string                      `json:"artifact_digest"`
	Provenance         deployment.SignedProvenance `json:"provenance"`
	RecoveryOf         string                      `json:"recovery_of,omitempty"`
	RestorePromotionID string                      `json:"restore_promotion_id,omitempty"`
}

type Gate struct {
	ID                        string         `json:"id"`
	OperationID               string         `json:"operation_id"`
	Configuration             Configuration  `json:"configuration"`
	Request                   PreviewRequest `json:"request"`
	Source                    forge.RepoRef  `json:"source"`
	Delivery                  forge.RepoRef  `json:"delivery"`
	SourceConnectionID        string         `json:"source_connection_id"`
	DeliveryConnectionID      string         `json:"delivery_connection_id"`
	SourceConnectionVersion   int64          `json:"source_connection_version"`
	DeliveryConnectionVersion int64          `json:"delivery_connection_version"`
	SourcePolicyHash          string         `json:"source_policy_hash"`
	DeliveryPolicyHash        string         `json:"delivery_policy_hash"`
	TargetSHA                 string         `json:"target_sha"`
	Before                    string         `json:"before"`
	After                     string         `json:"after"`
	ManifestSHA256            string         `json:"manifest_sha256"`
	PatchedManifest           []byte         `json:"patched_manifest"`
	Decision                  policy.Result  `json:"decision"`
	Blockers                  []string       `json:"blockers"`
	ExpiresAt                 time.Time      `json:"expires_at"`
}

type Promotion struct {
	CancelRequested      bool          `json:"cancel_requested"`
	ID                   string        `json:"id"`
	Environment          string        `json:"environment"`
	SourceRepositoryID   string        `json:"source_repository_id"`
	DeliveryRepositoryID string        `json:"delivery_repository_id"`
	GateID               string        `json:"gate_id"`
	State                string        `json:"state"`
	Reason               string        `json:"reason"`
	Version              int64         `json:"version"`
	RequestedBy          string        `json:"requested_by"`
	Branch               string        `json:"branch"`
	CandidateSHA         string        `json:"candidate_sha"`
	Change               *forge.Change `json:"change,omitempty"`
	MergeSHA             string        `json:"merge_sha"`
	RecoveryOf           string        `json:"recovery_of,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	FinishedAt           *time.Time    `json:"finished_at,omitempty"`
}

type HealthReport struct {
	OrgID                string          `json:"org_id"`
	PromotionID          string          `json:"promotion_id"`
	Environment          string          `json:"environment"`
	ConfigurationVersion int64           `json:"configuration_version"`
	SourceSHA            string          `json:"source_sha"`
	ArtifactDigest       string          `json:"artifact_digest"`
	DeliveryRevision     string          `json:"delivery_revision"`
	Healthy              bool            `json:"healthy"`
	Checks               map[string]bool `json:"checks"`
	ObservedAt           time.Time       `json:"observed_at"`
	Nonce                string          `json:"nonce"`
}

type Detail struct {
	Promotion Promotion     `json:"promotion"`
	Gate      Gate          `json:"gate"`
	Health    *HealthReport `json:"health,omitempty"`
}
