package dbgen

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type Artifact struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ID           pgtype.UUID        `json:"id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	TaskID       pgtype.UUID        `json:"task_id"`
	AttemptID    pgtype.UUID        `json:"attempt_id"`
	Name         string             `json:"name"`
	MediaType    string             `json:"media_type"`
	Size         int64              `json:"size"`
	Sha256       string             `json:"sha256"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
	ExpiresAt    pgtype.Timestamptz `json:"expires_at"`
}

type AuditEvent struct {
	ID           pgtype.UUID        `json:"id"`
	OrgID        pgtype.UUID        `json:"org_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	ActorID      string             `json:"actor_id"`
	Action       string             `json:"action"`
	ObjectID     string             `json:"object_id"`
	RequestID    string             `json:"request_id"`
	Data         []byte             `json:"data"`
	OccurredAt   pgtype.Timestamptz `json:"occurred_at"`
}

type Bootstrap struct {
	ID         bool               `json:"id"`
	TokenHash  string             `json:"token_hash"`
	ExpiresAt  pgtype.Timestamptz `json:"expires_at"`
	ConsumedAt pgtype.Timestamptz `json:"consumed_at"`
}

type BudgetLimit struct {
	OrgID       pgtype.UUID        `json:"org_id"`
	ScopeKind   string             `json:"scope_kind"`
	ScopeID     pgtype.UUID        `json:"scope_id"`
	Period      string             `json:"period"`
	PeriodStart pgtype.Timestamptz `json:"period_start"`
	PeriodEnd   pgtype.Timestamptz `json:"period_end"`
	Caps        []byte             `json:"caps"`
	Held        []byte             `json:"held"`
	Paused      bool               `json:"paused"`
	Version     int64              `json:"version"`
}

type BudgetReservation struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ID           pgtype.UUID        `json:"id"`
	OperationID  pgtype.UUID        `json:"operation_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	TaskID       pgtype.UUID        `json:"task_id"`
	JobID        pgtype.UUID        `json:"job_id"`
	AttemptID    pgtype.UUID        `json:"attempt_id"`
	Fingerprint  string             `json:"fingerprint"`
	Record       []byte             `json:"record"`
	State        string             `json:"state"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
}

type BudgetRoute struct {
	OrgID        pgtype.UUID `json:"org_id"`
	ConnectionID pgtype.UUID `json:"connection_id"`
	Model        string      `json:"model"`
	Name         string      `json:"name"`
	Config       []byte      `json:"config"`
	Version      int64       `json:"version"`
}

type BudgetSpend struct {
	OrgID       pgtype.UUID        `json:"org_id"`
	ScopeKind   string             `json:"scope_kind"`
	ScopeID     pgtype.UUID        `json:"scope_id"`
	PeriodStart pgtype.Timestamptz `json:"period_start"`
	Amount      []byte             `json:"amount"`
}

type Connection struct {
	OrgID             pgtype.UUID        `json:"org_id"`
	ID                pgtype.UUID        `json:"id"`
	Kind              string             `json:"kind"`
	Provider          string             `json:"provider"`
	Name              string             `json:"name"`
	Endpoint          string             `json:"endpoint"`
	Settings          []byte             `json:"settings"`
	State             string             `json:"state"`
	Reason            string             `json:"reason"`
	Capabilities      []byte             `json:"capabilities"`
	ServerVersion     string             `json:"server_version"`
	VerifiedAt        pgtype.Timestamptz `json:"verified_at"`
	SecretID          pgtype.UUID        `json:"secret_id"`
	CredentialVersion int64              `json:"credential_version"`
	Version           int64              `json:"version"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	RevokedAt         pgtype.Timestamptz `json:"revoked_at"`
}

type ConnectionRoute struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ConnectionID pgtype.UUID        `json:"connection_id"`
	RunnerID     pgtype.UUID        `json:"runner_id"`
	Hostname     string             `json:"hostname"`
	Cidrs        []string           `json:"cidrs"`
	ApprovedBy   pgtype.UUID        `json:"approved_by"`
	ApprovedAt   pgtype.Timestamptz `json:"approved_at"`
	RevokedAt    pgtype.Timestamptz `json:"revoked_at"`
}

type ConnectionSecret struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ID           pgtype.UUID        `json:"id"`
	ConnectionID pgtype.UUID        `json:"connection_id"`
	Version      int64              `json:"version"`
	Envelope     []byte             `json:"envelope"`
	RotatedAt    pgtype.Timestamptz `json:"rotated_at"`
}

type MemberRepository struct {
	OrgID        pgtype.UUID `json:"org_id"`
	UserID       pgtype.UUID `json:"user_id"`
	RepositoryID pgtype.UUID `json:"repository_id"`
}

type Membership struct {
	OrgID           pgtype.UUID `json:"org_id"`
	UserID          pgtype.UUID `json:"user_id"`
	Role            string      `json:"role"`
	AllRepositories bool        `json:"all_repositories"`
	Version         int64       `json:"version"`
}

type OidcLogin struct {
	StateHash   string             `json:"state_hash"`
	BrowserHash string             `json:"browser_hash"`
	Nonce       string             `json:"nonce"`
	Verifier    string             `json:"verifier"`
	ExpiresAt   pgtype.Timestamptz `json:"expires_at"`
}

type Organisation struct {
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	Version   int64              `json:"version"`
	Paused    bool               `json:"paused"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

type PolicyBinding struct {
	OrgID           pgtype.UUID `json:"org_id"`
	ScopeKind       string      `json:"scope_kind"`
	ScopeID         pgtype.UUID `json:"scope_id"`
	PolicyVersionID pgtype.UUID `json:"policy_version_id"`
	Version         int64       `json:"version"`
	PrimaryTeamID   pgtype.UUID `json:"primary_team_id"`
	SimulationHash  string      `json:"simulation_hash"`
}

type PolicyVersion struct {
	OrgID      pgtype.UUID        `json:"org_id"`
	ID         pgtype.UUID        `json:"id"`
	ScopeKind  string             `json:"scope_kind"`
	ScopeID    pgtype.UUID        `json:"scope_id"`
	Document   []byte             `json:"document"`
	PolicyHash string             `json:"policy_hash"`
	ActorID    pgtype.UUID        `json:"actor_id"`
	Reason     string             `json:"reason"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
}

type Repository struct {
	OrgID         pgtype.UUID        `json:"org_id"`
	ID            pgtype.UUID        `json:"id"`
	ConnectionID  pgtype.UUID        `json:"connection_id"`
	NativeID      string             `json:"native_id"`
	Name          string             `json:"name"`
	Url           string             `json:"url"`
	DefaultBranch string             `json:"default_branch"`
	Provider      string             `json:"provider"`
	Archived      bool               `json:"archived"`
	Paused        bool               `json:"paused"`
	Accessible    bool               `json:"accessible"`
	LastSyncedAt  pgtype.Timestamptz `json:"last_synced_at"`
	Version       int64              `json:"version"`
}

type Runner struct {
	OrgID               pgtype.UUID        `json:"org_id"`
	ID                  pgtype.UUID        `json:"id"`
	PoolID              pgtype.UUID        `json:"pool_id"`
	Name                string             `json:"name"`
	State               string             `json:"state"`
	CredentialHash      string             `json:"credential_hash"`
	CredentialExpiresAt pgtype.Timestamptz `json:"credential_expires_at"`
	CredentialVersion   int64              `json:"credential_version"`
	Version             int64              `json:"version"`
	EnrolledAt          pgtype.Timestamptz `json:"enrolled_at"`
	LastSeenAt          pgtype.Timestamptz `json:"last_seen_at"`
}

type RunnerEnrollment struct {
	OrgID      pgtype.UUID        `json:"org_id"`
	ID         pgtype.UUID        `json:"id"`
	PoolID     pgtype.UUID        `json:"pool_id"`
	TokenHash  string             `json:"token_hash"`
	ExpiresAt  pgtype.Timestamptz `json:"expires_at"`
	ConsumedAt pgtype.Timestamptz `json:"consumed_at"`
	CreatedBy  pgtype.UUID        `json:"created_by"`
}

type RunnerJobCredential struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ID           pgtype.UUID        `json:"id"`
	RunnerID     pgtype.UUID        `json:"runner_id"`
	PoolID       pgtype.UUID        `json:"pool_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	TaskID       pgtype.UUID        `json:"task_id"`
	JobID        pgtype.UUID        `json:"job_id"`
	AttemptID    pgtype.UUID        `json:"attempt_id"`
	OperationID  pgtype.UUID        `json:"operation_id"`
	FencingToken int64              `json:"fencing_token"`
	PolicyHash   string             `json:"policy_hash"`
	TokenHash    string             `json:"token_hash"`
	Methods      []string           `json:"methods"`
	ExpiresAt    pgtype.Timestamptz `json:"expires_at"`
	RevokedAt    pgtype.Timestamptz `json:"revoked_at"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
}

type RunnerPool struct {
	OrgID     pgtype.UUID        `json:"org_id"`
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	State     string             `json:"state"`
	Version   int64              `json:"version"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

type RunnerPoolRepository struct {
	OrgID        pgtype.UUID `json:"org_id"`
	PoolID       pgtype.UUID `json:"pool_id"`
	RepositoryID pgtype.UUID `json:"repository_id"`
}

type Session struct {
	ID        pgtype.UUID        `json:"id"`
	UserID    pgtype.UUID        `json:"user_id"`
	TokenHash string             `json:"token_hash"`
	CsrfToken string             `json:"csrf_token"`
	ExpiresAt pgtype.Timestamptz `json:"expires_at"`
	RevokedAt pgtype.Timestamptz `json:"revoked_at"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

type Team struct {
	OrgID   pgtype.UUID `json:"org_id"`
	ID      pgtype.UUID `json:"id"`
	Name    string      `json:"name"`
	Version int64       `json:"version"`
}

type TeamMembership struct {
	OrgID  pgtype.UUID `json:"org_id"`
	TeamID pgtype.UUID `json:"team_id"`
	UserID pgtype.UUID `json:"user_id"`
}

type TeamRepository struct {
	OrgID        pgtype.UUID `json:"org_id"`
	TeamID       pgtype.UUID `json:"team_id"`
	RepositoryID pgtype.UUID `json:"repository_id"`
}

type User struct {
	ID        pgtype.UUID        `json:"id"`
	Issuer    string             `json:"issuer"`
	Subject   string             `json:"subject"`
	Name      string             `json:"name"`
	Email     string             `json:"email"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

type WorkflowAttempt struct {
	OrgID      pgtype.UUID        `json:"org_id"`
	ID         pgtype.UUID        `json:"id"`
	TaskID     pgtype.UUID        `json:"task_id"`
	JobID      pgtype.UUID        `json:"job_id"`
	Number     int32              `json:"number"`
	Fence      int64              `json:"fence"`
	LeaseOwner string             `json:"lease_owner"`
	State      string             `json:"state"`
	StartedAt  pgtype.Timestamptz `json:"started_at"`
	EndedAt    pgtype.Timestamptz `json:"ended_at"`
}

type WorkflowEvent struct {
	OrgID            pgtype.UUID        `json:"org_id"`
	ID               int64              `json:"id"`
	RepositoryID     pgtype.UUID        `json:"repository_id"`
	Type             string             `json:"type"`
	AggregateType    string             `json:"aggregate_type"`
	AggregateID      pgtype.UUID        `json:"aggregate_id"`
	AggregateVersion int64              `json:"aggregate_version"`
	OccurredAt       pgtype.Timestamptz `json:"occurred_at"`
	RequestID        string             `json:"request_id"`
	DataVersion      int32              `json:"data_version"`
	Data             []byte             `json:"data"`
}

type WorkflowEventHead struct {
	OrgID         pgtype.UUID `json:"org_id"`
	LastID        int64       `json:"last_id"`
	RetainedAfter int64       `json:"retained_after"`
}

type WorkflowJob struct {
	OrgID          pgtype.UUID        `json:"org_id"`
	ID             pgtype.UUID        `json:"id"`
	TaskID         pgtype.UUID        `json:"task_id"`
	OperationID    pgtype.UUID        `json:"operation_id"`
	State          string             `json:"state"`
	Fence          int64              `json:"fence"`
	Attempts       int32              `json:"attempts"`
	LeaseOwner     string             `json:"lease_owner"`
	LeaseExpiresAt pgtype.Timestamptz `json:"lease_expires_at"`
	AvailableAt    pgtype.Timestamptz `json:"available_at"`
	Priority       int32              `json:"priority"`
}

type WorkflowOutbox struct {
	OrgID          pgtype.UUID        `json:"org_id"`
	ID             pgtype.UUID        `json:"id"`
	TaskID         pgtype.UUID        `json:"task_id"`
	RepositoryID   pgtype.UUID        `json:"repository_id"`
	OperationID    pgtype.UUID        `json:"operation_id"`
	Kind           string             `json:"kind"`
	IdempotencyKey string             `json:"idempotency_key"`
	Payload        []byte             `json:"payload"`
	PayloadHash    string             `json:"payload_hash"`
	State          string             `json:"state"`
	DispatchCount  int32              `json:"dispatch_count"`
	Evidence       string             `json:"evidence"`
	UpdatedAt      pgtype.Timestamptz `json:"updated_at"`
}

type WorkflowPause struct {
	OrgID     pgtype.UUID `json:"org_id"`
	ScopeKind string      `json:"scope_kind"`
	ScopeID   string      `json:"scope_id"`
	Paused    bool        `json:"paused"`
	Version   int64       `json:"version"`
}

type WorkflowRepoFairness struct {
	OrgID         pgtype.UUID        `json:"org_id"`
	RepositoryID  pgtype.UUID        `json:"repository_id"`
	LastClaimedAt pgtype.Timestamptz `json:"last_claimed_at"`
}

type WorkflowScheduler struct {
	OrgID         pgtype.UUID        `json:"org_id"`
	LastClaimedAt pgtype.Timestamptz `json:"last_claimed_at"`
}

type WorkflowTask struct {
	OrgID              pgtype.UUID        `json:"org_id"`
	ID                 pgtype.UUID        `json:"id"`
	RepositoryID       pgtype.UUID        `json:"repository_id"`
	OperationID        pgtype.UUID        `json:"operation_id"`
	IdempotencyKey     string             `json:"idempotency_key"`
	RequestHash        string             `json:"request_hash"`
	Recipe             string             `json:"recipe"`
	RecipeVersion      string             `json:"recipe_version"`
	TargetBranch       string             `json:"target_branch"`
	ModelConnectionID  pgtype.UUID        `json:"model_connection_id"`
	CampaignID         pgtype.UUID        `json:"campaign_id"`
	RunnerPoolID       pgtype.UUID        `json:"runner_pool_id"`
	PolicyHash         string             `json:"policy_hash"`
	StartingPolicyHash string             `json:"starting_policy_hash"`
	State              string             `json:"state"`
	Reason             string             `json:"reason"`
	Version            int64              `json:"version"`
	CancelVersion      int64              `json:"cancel_version"`
	MaxAttempts        int32              `json:"max_attempts"`
	CreatedBy          pgtype.UUID        `json:"created_by"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
	ModelRoute         string             `json:"model_route"`
}
