package repair

import (
	"reforge/internal/customcmd"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/workflow"
	"time"
)

type Input struct {
	FindingID            string `json:"finding_id"`
	FindingVersion       int64  `json:"finding_version"`
	Recipe               string `json:"recipe"`
	ModelConnectionID    string `json:"model_connection_id"`
	ModelRoute           string `json:"model_route"`
	RunnerPoolID         string `json:"runner_pool_id"`
	CustomProfileID      string `json:"custom_profile_id,omitempty"`
	CustomProfileVersion int64  `json:"custom_profile_version,omitempty"`
	PlanDigest           string `json:"plan_digest,omitempty"`
	IdempotencyKey       string `json:"idempotency_key,omitempty"`
}
type ExecutionContext struct {
	MaxAttempts        int                    `json:"max_attempts"`
	NativeHeadSHA      string                 `json:"native_head_sha,omitempty"`
	Request            Input                  `json:"request"`
	Plan               Plan                   `json:"plan"`
	BaselineRepository forge.RepoRef          `json:"baseline_repository"`
	Repository         forge.RepoRef          `json:"repository"`
	ConnectionID       string                 `json:"connection_id"`
	ConnectionVersion  int64                  `json:"connection_version"`
	Model              string                 `json:"model"`
	MaxOutputTokens    int                    `json:"max_output_tokens"`
	TurnTimeoutMS      int64                  `json:"turn_timeout_ms"`
	CustomProfile      *customcmd.ProfileSpec `json:"custom_profile,omitempty"`
	Finding            discovery.Finding      `json:"finding"`
	PolicyHash         string                 `json:"policy_hash"`
	CILogs             []CILog                `json:"ci_logs,omitempty"`
	OpenFixes          []string               `json:"open_fixes,omitempty"`
	FollowUpBranch     string                 `json:"follow_up_branch,omitempty"`
	OpenFixFiles       []map[string]string    `json:"open_fix_files,omitempty"`
}
type CILog struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Log  string `json:"log"`
}
type Preview struct {
	Context   ExecutionContext `json:"context"`
	Blockers  []string         `json:"blockers"`
	ExpiresAt time.Time        `json:"expires_at"`
}
type Run struct {
	CandidateArtifacts []string         `json:"candidate_artifacts"`
	Branch             string           `json:"branch"`
	CandidateSHA       string           `json:"candidate_sha"`
	CandidateChecks    []CheckResult    `json:"candidate_checks"`
	Change             *forge.Change    `json:"change,omitempty"`
	Task               workflow.Task    `json:"task"`
	Context            ExecutionContext `json:"context"`
	Report             *Report          `json:"report,omitempty"`
	State              string           `json:"state"`
	Version            int64            `json:"version"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

type Publication struct {
	HeadSHA     string        `json:"head_sha"`
	PlanDigest  string        `json:"plan_digest"`
	Checks      []CheckResult `json:"checks"`
	ArtifactIDs []string      `json:"artifact_ids"`
}
