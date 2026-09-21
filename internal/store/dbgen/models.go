package dbgen

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type AgentQualification struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ConnectionID pgtype.UUID        `json:"connection_id"`
	EvidenceID   pgtype.UUID        `json:"evidence_id"`
	Document     []byte             `json:"document"`
	CheckedAt    pgtype.Timestamptz `json:"checked_at"`
	ExpiresAt    pgtype.Timestamptz `json:"expires_at"`
	CreatedBy    pgtype.UUID        `json:"created_by"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
	UpdatedAt    pgtype.Timestamptz `json:"updated_at"`
}

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

type Campaign struct {
	OrgID              pgtype.UUID        `json:"org_id"`
	ID                 pgtype.UUID        `json:"id"`
	PreviewID          pgtype.UUID        `json:"preview_id"`
	IdempotencyKey     string             `json:"idempotency_key"`
	RequestedBy        pgtype.UUID        `json:"requested_by"`
	RequestedSessionID pgtype.UUID        `json:"requested_session_id"`
	GrantExpiresAt     pgtype.Timestamptz `json:"grant_expires_at"`
	Spec               []byte             `json:"spec"`
	Name               string             `json:"name"`
	Kind               string             `json:"kind"`
	State              string             `json:"state"`
	Reason             string             `json:"reason"`
	Version            int64              `json:"version"`
	Stage              int32              `json:"stage"`
	ObservingSince     pgtype.Timestamptz `json:"observing_since"`
	ObserveDue         pgtype.Timestamptz `json:"observe_due"`
	ControllerID       pgtype.UUID        `json:"controller_id"`
	ControllerUntil    pgtype.Timestamptz `json:"controller_until"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
	UpdatedAt          pgtype.Timestamptz `json:"updated_at"`
}

type CampaignMember struct {
	OrgID           pgtype.UUID        `json:"org_id"`
	CampaignID      pgtype.UUID        `json:"campaign_id"`
	ID              pgtype.UUID        `json:"id"`
	RepositoryID    pgtype.UUID        `json:"repository_id"`
	Document        []byte             `json:"document"`
	State           string             `json:"state"`
	Reason          string             `json:"reason"`
	Stage           int32              `json:"stage"`
	ActionID        pgtype.UUID        `json:"action_id"`
	GateID          pgtype.UUID        `json:"gate_id"`
	SucceededAt     pgtype.Timestamptz `json:"succeeded_at"`
	Halt            bool               `json:"halt"`
	StopComplete    bool               `json:"stop_complete"`
	ResumeRequested bool               `json:"resume_requested"`
	ObserveDue      pgtype.Timestamptz `json:"observe_due"`
	UpdatedAt       pgtype.Timestamptz `json:"updated_at"`
}

type CampaignPreview struct {
	OrgID       pgtype.UUID        `json:"org_id"`
	ID          pgtype.UUID        `json:"id"`
	RequestedBy pgtype.UUID        `json:"requested_by"`
	Document    []byte             `json:"document"`
	ExpiresAt   pgtype.Timestamptz `json:"expires_at"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
}

type CampaignRepositoryScope struct {
	OrgID        pgtype.UUID `json:"org_id"`
	CampaignID   pgtype.UUID `json:"campaign_id"`
	RepositoryID pgtype.UUID `json:"repository_id"`
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

type CustomProfile struct {
	OrgID            pgtype.UUID        `json:"org_id"`
	ID               pgtype.UUID        `json:"id"`
	Name             string             `json:"name"`
	Version          int64              `json:"version"`
	ImageDigest      string             `json:"image_digest"`
	Executable       string             `json:"executable"`
	Argv             []byte             `json:"argv"`
	ProtocolVersion  int32              `json:"protocol_version"`
	MaxWallSeconds   int32              `json:"max_wall_seconds"`
	MaxOutputBytes   int64              `json:"max_output_bytes"`
	MaxTurns         int32              `json:"max_turns"`
	Concurrency      int32              `json:"concurrency"`
	ApprovalEvidence string             `json:"approval_evidence"`
	ApprovedBy       pgtype.UUID        `json:"approved_by"`
	ApprovedAt       pgtype.Timestamptz `json:"approved_at"`
	RevokedAt        pgtype.Timestamptz `json:"revoked_at"`
	CreatedBy        pgtype.UUID        `json:"created_by"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
	UpdatedAt        pgtype.Timestamptz `json:"updated_at"`
}

type CustomProfileRun struct {
	OrgID          pgtype.UUID        `json:"org_id"`
	ID             pgtype.UUID        `json:"id"`
	ProfileID      pgtype.UUID        `json:"profile_id"`
	ProfileVersion int64              `json:"profile_version"`
	ImageDigest    string             `json:"image_digest"`
	TaskID         pgtype.UUID        `json:"task_id"`
	RequestedBy    pgtype.UUID        `json:"requested_by"`
	State          string             `json:"state"`
	Usage          []byte             `json:"usage"`
	Reason         string             `json:"reason"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
	UpdatedAt      pgtype.Timestamptz `json:"updated_at"`
	AttemptID      pgtype.UUID        `json:"attempt_id"`
	RepositoryID   pgtype.UUID        `json:"repository_id"`
	Events         []byte             `json:"events"`
	CompletedAt    pgtype.Timestamptz `json:"completed_at"`
	ReservationID  pgtype.UUID        `json:"reservation_id"`
}

type Deployment struct {
	OrgID             pgtype.UUID        `json:"org_id"`
	ID                pgtype.UUID        `json:"id"`
	RepositoryID      pgtype.UUID        `json:"repository_id"`
	Environment       string             `json:"environment"`
	GateID            pgtype.UUID        `json:"gate_id"`
	RequestedBy       pgtype.UUID        `json:"requested_by"`
	IdempotencyKey    string             `json:"idempotency_key"`
	State             string             `json:"state"`
	Reason            string             `json:"reason"`
	DispatchID        pgtype.UUID        `json:"dispatch_id"`
	NativeResult      []byte             `json:"native_result"`
	RecoveryOf        pgtype.UUID        `json:"recovery_of"`
	Version           int64              `json:"version"`
	FinishedAt        pgtype.Timestamptz `json:"finished_at"`
	FirstHealthyAt    pgtype.Timestamptz `json:"first_healthy_at"`
	LastHealthAt      pgtype.Timestamptz `json:"last_health_at"`
	ObserveAfter      pgtype.Timestamptz `json:"observe_after"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	UpdatedAt         pgtype.Timestamptz `json:"updated_at"`
	CancelRequested   bool               `json:"cancel_requested"`
	CancelDispatchID  pgtype.UUID        `json:"cancel_dispatch_id"`
	CancelState       string             `json:"cancel_state"`
	NativeEnvironment string             `json:"native_environment"`
}

type DeploymentConfiguration struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	Environment  string             `json:"environment"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	Version      int64              `json:"version"`
	Document     []byte             `json:"document"`
	UpdatedAt    pgtype.Timestamptz `json:"updated_at"`
}

type DeploymentGate struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ID           pgtype.UUID        `json:"id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	Environment  string             `json:"environment"`
	Document     []byte             `json:"document"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
}

type DeploymentHealthReport struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	DeploymentID pgtype.UUID        `json:"deployment_id"`
	Nonce        pgtype.UUID        `json:"nonce"`
	Document     []byte             `json:"document"`
	ReceivedAt   pgtype.Timestamptz `json:"received_at"`
}

type GitopsConfiguration struct {
	OrgID                pgtype.UUID        `json:"org_id"`
	Environment          string             `json:"environment"`
	SourceRepositoryID   pgtype.UUID        `json:"source_repository_id"`
	DeliveryRepositoryID pgtype.UUID        `json:"delivery_repository_id"`
	Version              int64              `json:"version"`
	Document             []byte             `json:"document"`
	UpdatedAt            pgtype.Timestamptz `json:"updated_at"`
}

type GitopsGate struct {
	OrgID                pgtype.UUID        `json:"org_id"`
	ID                   pgtype.UUID        `json:"id"`
	Environment          string             `json:"environment"`
	SourceRepositoryID   pgtype.UUID        `json:"source_repository_id"`
	DeliveryRepositoryID pgtype.UUID        `json:"delivery_repository_id"`
	Document             []byte             `json:"document"`
	CreatedAt            pgtype.Timestamptz `json:"created_at"`
}

type GitopsHealthReport struct {
	OrgID       pgtype.UUID        `json:"org_id"`
	PromotionID pgtype.UUID        `json:"promotion_id"`
	Nonce       pgtype.UUID        `json:"nonce"`
	Document    []byte             `json:"document"`
	ReceivedAt  pgtype.Timestamptz `json:"received_at"`
}

type GitopsPromotion struct {
	OrgID                pgtype.UUID        `json:"org_id"`
	ID                   pgtype.UUID        `json:"id"`
	Environment          string             `json:"environment"`
	SourceRepositoryID   pgtype.UUID        `json:"source_repository_id"`
	DeliveryRepositoryID pgtype.UUID        `json:"delivery_repository_id"`
	GateID               pgtype.UUID        `json:"gate_id"`
	RequestedBy          pgtype.UUID        `json:"requested_by"`
	IdempotencyKey       string             `json:"idempotency_key"`
	State                string             `json:"state"`
	Reason               string             `json:"reason"`
	Branch               string             `json:"branch"`
	TargetBranch         string             `json:"target_branch"`
	ManifestPath         string             `json:"manifest_path"`
	Pointer              string             `json:"pointer"`
	CancelRequested      bool               `json:"cancel_requested"`
	CandidateSha         string             `json:"candidate_sha"`
	NativeChange         []byte             `json:"native_change"`
	MergeSha             string             `json:"merge_sha"`
	StageDispatchID      pgtype.UUID        `json:"stage_dispatch_id"`
	PublishDispatchID    pgtype.UUID        `json:"publish_dispatch_id"`
	RecoveryOf           pgtype.UUID        `json:"recovery_of"`
	Version              int64              `json:"version"`
	FirstHealthyAt       pgtype.Timestamptz `json:"first_healthy_at"`
	LastHealthAt         pgtype.Timestamptz `json:"last_health_at"`
	FinishedAt           pgtype.Timestamptz `json:"finished_at"`
	ObserveAfter         pgtype.Timestamptz `json:"observe_after"`
	CreatedAt            pgtype.Timestamptz `json:"created_at"`
	UpdatedAt            pgtype.Timestamptz `json:"updated_at"`
}

type InventoryCandidate struct {
	OrgID      pgtype.UUID `json:"org_id"`
	JobID      pgtype.UUID `json:"job_id"`
	NativeID   string      `json:"native_id"`
	Repository []byte      `json:"repository"`
}

type InventoryChange struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	NativeID     string             `json:"native_id"`
	Snapshot     []byte             `json:"snapshot"`
	JobID        pgtype.UUID        `json:"job_id"`
	ObservedAt   pgtype.Timestamptz `json:"observed_at"`
}

type InventoryDelivery struct {
	OrgID       pgtype.UUID        `json:"org_id"`
	EndpointID  pgtype.UUID        `json:"endpoint_id"`
	DeliveryKey string             `json:"delivery_key"`
	Digest      string             `json:"digest"`
	ReceivedAt  pgtype.Timestamptz `json:"received_at"`
}

type InventoryJob struct {
	OrgID             pgtype.UUID        `json:"org_id"`
	ID                pgtype.UUID        `json:"id"`
	ConnectionID      pgtype.UUID        `json:"connection_id"`
	ConnectionVersion int64              `json:"connection_version"`
	Namespace         string             `json:"namespace"`
	Kind              string             `json:"kind"`
	RepositoryID      pgtype.UUID        `json:"repository_id"`
	ParentID          pgtype.UUID        `json:"parent_id"`
	RequestedBy       pgtype.UUID        `json:"requested_by"`
	Input             []byte             `json:"input"`
	State             string             `json:"state"`
	Reason            string             `json:"reason"`
	Cursor            string             `json:"cursor"`
	Phase             string             `json:"phase"`
	Pages             int32              `json:"pages"`
	Processed         int32              `json:"processed"`
	Failures          int32              `json:"failures"`
	FencingToken      int64              `json:"fencing_token"`
	LeaseOwner        string             `json:"lease_owner"`
	LeaseUntil        pgtype.Timestamptz `json:"lease_until"`
	AvailableAt       pgtype.Timestamptz `json:"available_at"`
	LastClaimed       pgtype.Timestamptz `json:"last_claimed"`
	Version           int64              `json:"version"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	UpdatedAt         pgtype.Timestamptz `json:"updated_at"`
}

type InventoryRepositoryState struct {
	OrgID             pgtype.UUID        `json:"org_id"`
	RepositoryID      pgtype.UUID        `json:"repository_id"`
	ConnectionID      pgtype.UUID        `json:"connection_id"`
	Namespace         string             `json:"namespace"`
	ConnectionVersion int64              `json:"connection_version"`
	ScanID            pgtype.UUID        `json:"scan_id"`
	State             string             `json:"state"`
	Reason            string             `json:"reason"`
	RefreshDue        pgtype.Timestamptz `json:"refresh_due"`
	ChangesObservedAt pgtype.Timestamptz `json:"changes_observed_at"`
}

type InventorySchedulerCursor struct {
	ID    pgtype.UUID `json:"id"`
	OrgID pgtype.UUID `json:"org_id"`
}

type InventorySource struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ConnectionID pgtype.UUID        `json:"connection_id"`
	Namespace    string             `json:"namespace"`
	PollDue      pgtype.Timestamptz `json:"poll_due"`
	LastServed   pgtype.Timestamptz `json:"last_served"`
}

type InventoryTenant struct {
	OrgID pgtype.UUID `json:"org_id"`
}

type InventoryWebhook struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	ID           pgtype.UUID        `json:"id"`
	ConnectionID pgtype.UUID        `json:"connection_id"`
	Envelope     []byte             `json:"envelope"`
	Version      int64              `json:"version"`
	RevokedAt    pgtype.Timestamptz `json:"revoked_at"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
}

type MaintenanceConfig struct {
	OrgID          pgtype.UUID `json:"org_id"`
	RepositoryID   pgtype.UUID `json:"repository_id"`
	TrustedBots    []byte      `json:"trusted_bots"`
	MergeAuthority string      `json:"merge_authority"`
	Version        int64       `json:"version"`
}

type MaintenanceFinding struct {
	OrgID              pgtype.UUID        `json:"org_id"`
	ID                 pgtype.UUID        `json:"id"`
	RepositoryID       pgtype.UUID        `json:"repository_id"`
	Fingerprint        string             `json:"fingerprint"`
	FingerprintVersion int32              `json:"fingerprint_version"`
	Source             string             `json:"source"`
	SourceID           string             `json:"source_id"`
	Category           string             `json:"category"`
	Severity           string             `json:"severity"`
	Title              string             `json:"title"`
	Evidence           []byte             `json:"evidence"`
	EvidenceDigest     string             `json:"evidence_digest"`
	State              string             `json:"state"`
	Reason             string             `json:"reason"`
	AssignedTo         pgtype.UUID        `json:"assigned_to"`
	SnoozeUntil        pgtype.Timestamptz `json:"snooze_until"`
	SupersededBy       pgtype.UUID        `json:"superseded_by"`
	Version            int64              `json:"version"`
	FirstSeen          pgtype.Timestamptz `json:"first_seen"`
	LastSeen           pgtype.Timestamptz `json:"last_seen"`
}

type MaintenanceObservation struct {
	OrgID          pgtype.UUID        `json:"org_id"`
	ID             pgtype.UUID        `json:"id"`
	FindingID      pgtype.UUID        `json:"finding_id"`
	EvidenceDigest string             `json:"evidence_digest"`
	Evidence       []byte             `json:"evidence"`
	ObservedAt     pgtype.Timestamptz `json:"observed_at"`
}

type MaintenanceRepair struct {
	OrgID          pgtype.UUID        `json:"org_id"`
	FindingID      pgtype.UUID        `json:"finding_id"`
	RepositoryID   pgtype.UUID        `json:"repository_id"`
	TaskID         pgtype.UUID        `json:"task_id"`
	EvidenceDigest string             `json:"evidence_digest"`
	Active         bool               `json:"active"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
}

type MaintenanceScan struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	RequestedBy  pgtype.UUID        `json:"requested_by"`
	State        string             `json:"state"`
	Reason       string             `json:"reason"`
	Fence        int64              `json:"fence"`
	LeaseUntil   pgtype.Timestamptz `json:"lease_until"`
	AvailableAt  pgtype.Timestamptz `json:"available_at"`
	ObservedAt   pgtype.Timestamptz `json:"observed_at"`
	Version      int64              `json:"version"`
}

type MaintenanceSchedulerCursor struct {
	ID    bool        `json:"id"`
	OrgID pgtype.UUID `json:"org_id"`
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

type MergeConfiguration struct {
	OrgID        pgtype.UUID        `json:"org_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	Version      int64              `json:"version"`
	Document     []byte             `json:"document"`
	UpdatedAt    pgtype.Timestamptz `json:"updated_at"`
}

type MergeExecutionCheck struct {
	OrgID       pgtype.UUID        `json:"org_id"`
	ID          pgtype.UUID        `json:"id"`
	OperationID pgtype.UUID        `json:"operation_id"`
	GateID      pgtype.UUID        `json:"gate_id"`
	Sha         string             `json:"sha"`
	Phase       string             `json:"phase"`
	State       string             `json:"state"`
	NativeID    string             `json:"native_id"`
	DispatchID  pgtype.UUID        `json:"dispatch_id"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

type MergeGate struct {
	OrgID                pgtype.UUID        `json:"org_id"`
	ID                   pgtype.UUID        `json:"id"`
	RepositoryID         pgtype.UUID        `json:"repository_id"`
	ChangeID             string             `json:"change_id"`
	ConfigurationVersion int64              `json:"configuration_version"`
	Document             []byte             `json:"document"`
	CreatedAt            pgtype.Timestamptz `json:"created_at"`
}

type MergeOperation struct {
	OrgID           pgtype.UUID        `json:"org_id"`
	ID              pgtype.UUID        `json:"id"`
	RepositoryID    pgtype.UUID        `json:"repository_id"`
	GateID          pgtype.UUID        `json:"gate_id"`
	RequestedGateID pgtype.UUID        `json:"requested_gate_id"`
	ChangeID        string             `json:"change_id"`
	TargetBranch    string             `json:"target_branch"`
	IdempotencyKey  string             `json:"idempotency_key"`
	RequestedBy     pgtype.UUID        `json:"requested_by"`
	State           string             `json:"state"`
	Reason          string             `json:"reason"`
	NativeResult    []byte             `json:"native_result"`
	CancelRequested bool               `json:"cancel_requested"`
	Version         int64              `json:"version"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
	UpdatedAt       pgtype.Timestamptz `json:"updated_at"`
	ObserveAfter    pgtype.Timestamptz `json:"observe_after"`
	NativeQueueID   string             `json:"native_queue_id"`
}

type ModelTurn struct {
	OrgID          pgtype.UUID        `json:"org_id"`
	ID             pgtype.UUID        `json:"id"`
	TaskID         pgtype.UUID        `json:"task_id"`
	AttemptID      pgtype.UUID        `json:"attempt_id"`
	RepositoryID   pgtype.UUID        `json:"repository_id"`
	ReservationID  pgtype.UUID        `json:"reservation_id"`
	RequestHash    string             `json:"request_hash"`
	State          string             `json:"state"`
	ResultEnvelope []byte             `json:"result_envelope"`
	Usage          []byte             `json:"usage"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
	CompletedAt    pgtype.Timestamptz `json:"completed_at"`
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

type RepairRun struct {
	OrgID                  pgtype.UUID        `json:"org_id"`
	TaskID                 pgtype.UUID        `json:"task_id"`
	RepositoryID           pgtype.UUID        `json:"repository_id"`
	FindingID              pgtype.UUID        `json:"finding_id"`
	RequestedBy            pgtype.UUID        `json:"requested_by"`
	FindingVersion         int64              `json:"finding_version"`
	FindingDigest          string             `json:"finding_digest"`
	Context                []byte             `json:"context"`
	Report                 []byte             `json:"report"`
	State                  string             `json:"state"`
	Version                int64              `json:"version"`
	CreatedAt              pgtype.Timestamptz `json:"created_at"`
	UpdatedAt              pgtype.Timestamptz `json:"updated_at"`
	Branch                 string             `json:"branch"`
	CandidateSha           string             `json:"candidate_sha"`
	CandidateChecks        []byte             `json:"candidate_checks"`
	NativeChange           []byte             `json:"native_change"`
	CandidateArtifacts     []byte             `json:"candidate_artifacts"`
	BotGateID              pgtype.UUID        `json:"bot_gate_id"`
	BotRevalidationVersion int64              `json:"bot_revalidation_version"`
	BotRevalidationState   string             `json:"bot_revalidation_state"`
	BotRevalidationReason  string             `json:"bot_revalidation_reason"`
	BotObserveAfter        pgtype.Timestamptz `json:"bot_observe_after"`
	BotObservedAt          pgtype.Timestamptz `json:"bot_observed_at"`
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
	OrgID                 pgtype.UUID        `json:"org_id"`
	ID                    pgtype.UUID        `json:"id"`
	RepositoryID          pgtype.UUID        `json:"repository_id"`
	OperationID           pgtype.UUID        `json:"operation_id"`
	IdempotencyKey        string             `json:"idempotency_key"`
	RequestHash           string             `json:"request_hash"`
	Recipe                string             `json:"recipe"`
	RecipeVersion         string             `json:"recipe_version"`
	TargetBranch          string             `json:"target_branch"`
	ModelConnectionID     pgtype.UUID        `json:"model_connection_id"`
	CampaignID            pgtype.UUID        `json:"campaign_id"`
	RunnerPoolID          pgtype.UUID        `json:"runner_pool_id"`
	PolicyHash            string             `json:"policy_hash"`
	StartingPolicyHash    string             `json:"starting_policy_hash"`
	State                 string             `json:"state"`
	Reason                string             `json:"reason"`
	Version               int64              `json:"version"`
	CancelVersion         int64              `json:"cancel_version"`
	MaxAttempts           int32              `json:"max_attempts"`
	CreatedBy             pgtype.UUID        `json:"created_by"`
	CreatedAt             pgtype.Timestamptz `json:"created_at"`
	ModelRoute            string             `json:"model_route"`
	CancellationRequested bool               `json:"cancellation_requested"`
}
