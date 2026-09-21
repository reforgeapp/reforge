package generated

import (
	"time"
)

const (
	BotRevalidationStateBlocked          BotRevalidationState = "blocked"
	BotRevalidationStateClosed           BotRevalidationState = "closed"
	BotRevalidationStateMerged           BotRevalidationState = "merged"
	BotRevalidationStatePending          BotRevalidationState = "pending"
	BotRevalidationStateReady            BotRevalidationState = "ready"
	BotRevalidationStateWaitingCompanion BotRevalidationState = "waiting_companion"
)

func (e BotRevalidationState) Valid() bool {
	switch e {
	case BotRevalidationStateBlocked:
		return true
	case BotRevalidationStateClosed:
		return true
	case BotRevalidationStateMerged:
		return true
	case BotRevalidationStatePending:
		return true
	case BotRevalidationStateReady:
		return true
	case BotRevalidationStateWaitingCompanion:
		return true
	default:
		return false
	}
}

const (
	BudgetLimitPeriodCustom  BudgetLimitPeriod = "custom"
	BudgetLimitPeriodDaily   BudgetLimitPeriod = "daily"
	BudgetLimitPeriodMonthly BudgetLimitPeriod = "monthly"
)

func (e BudgetLimitPeriod) Valid() bool {
	switch e {
	case BudgetLimitPeriodCustom:
		return true
	case BudgetLimitPeriodDaily:
		return true
	case BudgetLimitPeriodMonthly:
		return true
	default:
		return false
	}
}

const (
	BudgetLimitInputPeriodCustom  BudgetLimitInputPeriod = "custom"
	BudgetLimitInputPeriodDaily   BudgetLimitInputPeriod = "daily"
	BudgetLimitInputPeriodMonthly BudgetLimitInputPeriod = "monthly"
)

func (e BudgetLimitInputPeriod) Valid() bool {
	switch e {
	case BudgetLimitInputPeriodCustom:
		return true
	case BudgetLimitInputPeriodDaily:
		return true
	case BudgetLimitInputPeriodMonthly:
		return true
	default:
		return false
	}
}

const (
	ContinueCurrentStage CampaignControlInputStageDecision = "continue_current_stage"
)

func (e CampaignControlInputStageDecision) Valid() bool {
	switch e {
	case ContinueCurrentStage:
		return true
	default:
		return false
	}
}

const (
	CampaignInputKindGitops   CampaignInputKind = "gitops"
	CampaignInputKindPipeline CampaignInputKind = "pipeline"
	CampaignInputKindRepair   CampaignInputKind = "repair"
)

func (e CampaignInputKind) Valid() bool {
	switch e {
	case CampaignInputKindGitops:
		return true
	case CampaignInputKindPipeline:
		return true
	case CampaignInputKindRepair:
		return true
	default:
		return false
	}
}

const (
	CampaignInputSuccessHealthy   CampaignInputSuccess = "healthy"
	CampaignInputSuccessMerged    CampaignInputSuccess = "merged"
	CampaignInputSuccessPublished CampaignInputSuccess = "published"
)

func (e CampaignInputSuccess) Valid() bool {
	switch e {
	case CampaignInputSuccessHealthy:
		return true
	case CampaignInputSuccessMerged:
		return true
	case CampaignInputSuccessPublished:
		return true
	default:
		return false
	}
}

const (
	CapabilityStateSupported   CapabilityState = "supported"
	CapabilityStateUnknown     CapabilityState = "unknown"
	CapabilityStateUnsupported CapabilityState = "unsupported"
)

func (e CapabilityState) Valid() bool {
	switch e {
	case CapabilityStateSupported:
		return true
	case CapabilityStateUnknown:
		return true
	case CapabilityStateUnsupported:
		return true
	default:
		return false
	}
}

const (
	ChangeStateBlocked        ChangeState = "blocked"
	ChangeStateClosed         ChangeState = "closed"
	ChangeStateDraft          ChangeState = "draft"
	ChangeStateEligible       ChangeState = "eligible"
	ChangeStateEvaluating     ChangeState = "evaluating"
	ChangeStateMergeRequested ChangeState = "merge_requested"
	ChangeStateMerged         ChangeState = "merged"
	ChangeStateOpen           ChangeState = "open"
	ChangeStateQueued         ChangeState = "queued"
	ChangeStateReconciling    ChangeState = "reconciling"
)

func (e ChangeState) Valid() bool {
	switch e {
	case ChangeStateBlocked:
		return true
	case ChangeStateClosed:
		return true
	case ChangeStateDraft:
		return true
	case ChangeStateEligible:
		return true
	case ChangeStateEvaluating:
		return true
	case ChangeStateMergeRequested:
		return true
	case ChangeStateMerged:
		return true
	case ChangeStateOpen:
		return true
	case ChangeStateQueued:
		return true
	case ChangeStateReconciling:
		return true
	default:
		return false
	}
}

const (
	DecisionOutcomeAllow   DecisionOutcome = "allow"
	DecisionOutcomeDeny    DecisionOutcome = "deny"
	DecisionOutcomeUnknown DecisionOutcome = "unknown"
)

func (e DecisionOutcome) Valid() bool {
	switch e {
	case DecisionOutcomeAllow:
		return true
	case DecisionOutcomeDeny:
		return true
	case DecisionOutcomeUnknown:
		return true
	default:
		return false
	}
}

const (
	DeploymentConfigurationModeObserve  DeploymentConfigurationMode = "observe"
	DeploymentConfigurationModePipeline DeploymentConfigurationMode = "pipeline"
)

func (e DeploymentConfigurationMode) Valid() bool {
	switch e {
	case DeploymentConfigurationModeObserve:
		return true
	case DeploymentConfigurationModePipeline:
		return true
	default:
		return false
	}
}

const (
	DeploymentOperationCancelStateConfirmed   DeploymentOperationCancelState = "confirmed"
	DeploymentOperationCancelStateDispatching DeploymentOperationCancelState = "dispatching"
	DeploymentOperationCancelStateEmpty       DeploymentOperationCancelState = ""
	DeploymentOperationCancelStatePending     DeploymentOperationCancelState = "pending"
	DeploymentOperationCancelStateUncertain   DeploymentOperationCancelState = "uncertain"
)

func (e DeploymentOperationCancelState) Valid() bool {
	switch e {
	case DeploymentOperationCancelStateConfirmed:
		return true
	case DeploymentOperationCancelStateDispatching:
		return true
	case DeploymentOperationCancelStateEmpty:
		return true
	case DeploymentOperationCancelStatePending:
		return true
	case DeploymentOperationCancelStateUncertain:
		return true
	default:
		return false
	}
}

const (
	DeploymentStateAwaitingGates       DeploymentState = "awaiting_gates"
	DeploymentStateBlocked             DeploymentState = "blocked"
	DeploymentStateCancelled           DeploymentState = "cancelled"
	DeploymentStateCompletedUnverified DeploymentState = "completed_unverified"
	DeploymentStateEligible            DeploymentState = "eligible"
	DeploymentStateFailed              DeploymentState = "failed"
	DeploymentStateHealthy             DeploymentState = "healthy"
	DeploymentStatePlanned             DeploymentState = "planned"
	DeploymentStateReconciling         DeploymentState = "reconciling"
	DeploymentStateRecovered           DeploymentState = "recovered"
	DeploymentStateRecovering          DeploymentState = "recovering"
	DeploymentStateRecoveryFailed      DeploymentState = "recovery_failed"
	DeploymentStateRecoveryRequested   DeploymentState = "recovery_requested"
	DeploymentStateRequested           DeploymentState = "requested"
	DeploymentStateRunning             DeploymentState = "running"
	DeploymentStateVerifying           DeploymentState = "verifying"
)

func (e DeploymentState) Valid() bool {
	switch e {
	case DeploymentStateAwaitingGates:
		return true
	case DeploymentStateBlocked:
		return true
	case DeploymentStateCancelled:
		return true
	case DeploymentStateCompletedUnverified:
		return true
	case DeploymentStateEligible:
		return true
	case DeploymentStateFailed:
		return true
	case DeploymentStateHealthy:
		return true
	case DeploymentStatePlanned:
		return true
	case DeploymentStateReconciling:
		return true
	case DeploymentStateRecovered:
		return true
	case DeploymentStateRecovering:
		return true
	case DeploymentStateRecoveryFailed:
		return true
	case DeploymentStateRecoveryRequested:
		return true
	case DeploymentStateRequested:
		return true
	case DeploymentStateRunning:
		return true
	case DeploymentStateVerifying:
		return true
	default:
		return false
	}
}

const (
	DiscoveryScanStateComplete   DiscoveryScanState = "complete"
	DiscoveryScanStateFailed     DiscoveryScanState = "failed"
	DiscoveryScanStateNotStarted DiscoveryScanState = "not_started"
	DiscoveryScanStateQueued     DiscoveryScanState = "queued"
	DiscoveryScanStateRunning    DiscoveryScanState = "running"
	DiscoveryScanStateStale      DiscoveryScanState = "stale"
)

func (e DiscoveryScanState) Valid() bool {
	switch e {
	case DiscoveryScanStateComplete:
		return true
	case DiscoveryScanStateFailed:
		return true
	case DiscoveryScanStateNotStarted:
		return true
	case DiscoveryScanStateQueued:
		return true
	case DiscoveryScanStateRunning:
		return true
	case DiscoveryScanStateStale:
		return true
	default:
		return false
	}
}

const (
	Critical FindingSeverity = "critical"
	High     FindingSeverity = "high"
	Info     FindingSeverity = "info"
	Low      FindingSeverity = "low"
	Medium   FindingSeverity = "medium"
)

func (e FindingSeverity) Valid() bool {
	switch e {
	case Critical:
		return true
	case High:
		return true
	case Info:
		return true
	case Low:
		return true
	case Medium:
		return true
	default:
		return false
	}
}

const (
	FindingStateDismissed  FindingState = "dismissed"
	FindingStateOpen       FindingState = "open"
	FindingStateResolved   FindingState = "resolved"
	FindingStateSnoozed    FindingState = "snoozed"
	FindingStateSuperseded FindingState = "superseded"
)

func (e FindingState) Valid() bool {
	switch e {
	case FindingStateDismissed:
		return true
	case FindingStateOpen:
		return true
	case FindingStateResolved:
		return true
	case FindingStateSnoozed:
		return true
	case FindingStateSuperseded:
		return true
	default:
		return false
	}
}

const (
	Assign  FindingUpdateAction = "assign"
	Dismiss FindingUpdateAction = "dismiss"
	Reopen  FindingUpdateAction = "reopen"
	Snooze  FindingUpdateAction = "snooze"
)

func (e FindingUpdateAction) Valid() bool {
	switch e {
	case Assign:
		return true
	case Dismiss:
		return true
	case Reopen:
		return true
	case Snooze:
		return true
	default:
		return false
	}
}

const (
	Import  InventoryJobKind = "import"
	Refresh InventoryJobKind = "refresh"
	Scan    InventoryJobKind = "scan"
)

func (e InventoryJobKind) Valid() bool {
	switch e {
	case Import:
		return true
	case Refresh:
		return true
	case Scan:
		return true
	default:
		return false
	}
}

const (
	InventoryJobStateCancelled InventoryJobState = "cancelled"
	InventoryJobStateComplete  InventoryJobState = "complete"
	InventoryJobStateFailed    InventoryJobState = "failed"
	InventoryJobStateQueued    InventoryJobState = "queued"
	InventoryJobStateRunning   InventoryJobState = "running"
	InventoryJobStateStale     InventoryJobState = "stale"
)

func (e InventoryJobState) Valid() bool {
	switch e {
	case InventoryJobStateCancelled:
		return true
	case InventoryJobStateComplete:
		return true
	case InventoryJobStateFailed:
		return true
	case InventoryJobStateQueued:
		return true
	case InventoryJobStateRunning:
		return true
	case InventoryJobStateStale:
		return true
	default:
		return false
	}
}

const (
	Dependabot MaintenanceBotIdentityKind = "dependabot"
	Renovate   MaintenanceBotIdentityKind = "renovate"
)

func (e MaintenanceBotIdentityKind) Valid() bool {
	switch e {
	case Dependabot:
		return true
	case Renovate:
		return true
	default:
		return false
	}
}

const (
	MaintenanceBotStatusAutomergeDisabled MaintenanceBotStatusAutomerge = "disabled"
	MaintenanceBotStatusAutomergeEnabled  MaintenanceBotStatusAutomerge = "enabled"
	MaintenanceBotStatusAutomergeUnknown  MaintenanceBotStatusAutomerge = "unknown"
)

func (e MaintenanceBotStatusAutomerge) Valid() bool {
	switch e {
	case MaintenanceBotStatusAutomergeDisabled:
		return true
	case MaintenanceBotStatusAutomergeEnabled:
		return true
	case MaintenanceBotStatusAutomergeUnknown:
		return true
	default:
		return false
	}
}

const (
	MaintenanceConfigMergeAuthorityBot     MaintenanceConfigMergeAuthority = "bot"
	MaintenanceConfigMergeAuthorityObserve MaintenanceConfigMergeAuthority = "observe"
	MaintenanceConfigMergeAuthorityReforge MaintenanceConfigMergeAuthority = "reforge"
)

func (e MaintenanceConfigMergeAuthority) Valid() bool {
	switch e {
	case MaintenanceConfigMergeAuthorityBot:
		return true
	case MaintenanceConfigMergeAuthorityObserve:
		return true
	case MaintenanceConfigMergeAuthorityReforge:
		return true
	default:
		return false
	}
}

const (
	Hosted     MetaEdition = "hosted"
	SelfHosted MetaEdition = "self-hosted"
)

func (e MetaEdition) Valid() bool {
	switch e {
	case Hosted:
		return true
	case SelfHosted:
		return true
	default:
		return false
	}
}

const (
	Maintenancev1 PolicyDocumentSchema = "maintenance/v1"
)

func (e PolicyDocumentSchema) Valid() bool {
	switch e {
	case Maintenancev1:
		return true
	default:
		return false
	}
}

const (
	PolicyInputStageDeploymentAdmission PolicyInputStage = "deployment_admission"
	PolicyInputStageEmpty               PolicyInputStage = ""
)

func (e PolicyInputStage) Valid() bool {
	switch e {
	case PolicyInputStageDeploymentAdmission:
		return true
	case PolicyInputStageEmpty:
		return true
	default:
		return false
	}
}

const (
	PolicyResultOutcomeAllow   PolicyResultOutcome = "allow"
	PolicyResultOutcomeDeny    PolicyResultOutcome = "deny"
	PolicyResultOutcomeUnknown PolicyResultOutcome = "unknown"
)

func (e PolicyResultOutcome) Valid() bool {
	switch e {
	case PolicyResultOutcomeAllow:
		return true
	case PolicyResultOutcomeDeny:
		return true
	case PolicyResultOutcomeUnknown:
		return true
	default:
		return false
	}
}

const (
	PolicyScopeKindOrganisation PolicyScopeKind = "organisation"
	PolicyScopeKindRepository   PolicyScopeKind = "repository"
	PolicyScopeKindTeam         PolicyScopeKind = "team"
)

func (e PolicyScopeKind) Valid() bool {
	switch e {
	case PolicyScopeKindOrganisation:
		return true
	case PolicyScopeKindRepository:
		return true
	case PolicyScopeKindTeam:
		return true
	default:
		return false
	}
}

const (
	PrivateOperationKindForgeApprovals        PrivateOperationKind = "forge.approvals"
	PrivateOperationKindForgeChecks           PrivateOperationKind = "forge.checks"
	PrivateOperationKindForgeInventory        PrivateOperationKind = "forge.inventory"
	PrivateOperationKindForgeProbe            PrivateOperationKind = "forge.probe"
	PrivateOperationKindForgeReadChange       PrivateOperationKind = "forge.read_change"
	PrivateOperationKindForgeReadFile         PrivateOperationKind = "forge.read_file"
	PrivateOperationKindForgeReconcileChanges PrivateOperationKind = "forge.reconcile_changes"
	PrivateOperationKindForgeRepository       PrivateOperationKind = "forge.repository"
	PrivateOperationKindForgeResolveRef       PrivateOperationKind = "forge.resolve_ref"
	PrivateOperationKindForgeSourceManifest   PrivateOperationKind = "forge.source_manifest"
	PrivateOperationKindModelList             PrivateOperationKind = "model.list"
	PrivateOperationKindModelProbe            PrivateOperationKind = "model.probe"
)

func (e PrivateOperationKind) Valid() bool {
	switch e {
	case PrivateOperationKindForgeApprovals:
		return true
	case PrivateOperationKindForgeChecks:
		return true
	case PrivateOperationKindForgeInventory:
		return true
	case PrivateOperationKindForgeProbe:
		return true
	case PrivateOperationKindForgeReadChange:
		return true
	case PrivateOperationKindForgeReadFile:
		return true
	case PrivateOperationKindForgeReconcileChanges:
		return true
	case PrivateOperationKindForgeRepository:
		return true
	case PrivateOperationKindForgeResolveRef:
		return true
	case PrivateOperationKindForgeSourceManifest:
		return true
	case PrivateOperationKindModelList:
		return true
	case PrivateOperationKindModelProbe:
		return true
	default:
		return false
	}
}

const (
	Gitea  RepositoryProvider = "gitea"
	Github RepositoryProvider = "github"
	Gitlab RepositoryProvider = "gitlab"
)

func (e RepositoryProvider) Valid() bool {
	switch e {
	case Gitea:
		return true
	case Github:
		return true
	case Gitlab:
		return true
	default:
		return false
	}
}

const (
	Admin      Role = "admin"
	Maintainer Role = "maintainer"
	Owner      Role = "owner"
	Reviewer   Role = "reviewer"
	Viewer     Role = "viewer"
)

func (e Role) Valid() bool {
	switch e {
	case Admin:
		return true
	case Maintainer:
		return true
	case Owner:
		return true
	case Reviewer:
		return true
	case Viewer:
		return true
	default:
		return false
	}
}

const (
	RunnerCompletionOutcomeCancelled RunnerCompletionOutcome = "cancelled"
	RunnerCompletionOutcomeCompleted RunnerCompletionOutcome = "completed"
	RunnerCompletionOutcomeFailed    RunnerCompletionOutcome = "failed"
	RunnerCompletionOutcomeUncertain RunnerCompletionOutcome = "uncertain"
)

func (e RunnerCompletionOutcome) Valid() bool {
	switch e {
	case RunnerCompletionOutcomeCancelled:
		return true
	case RunnerCompletionOutcomeCompleted:
		return true
	case RunnerCompletionOutcomeFailed:
		return true
	case RunnerCompletionOutcomeUncertain:
		return true
	default:
		return false
	}
}

const (
	RunnerPoolStateActive   RunnerPoolState = "active"
	RunnerPoolStateDraining RunnerPoolState = "draining"
	RunnerPoolStateRevoked  RunnerPoolState = "revoked"
)

func (e RunnerPoolState) Valid() bool {
	switch e {
	case RunnerPoolStateActive:
		return true
	case RunnerPoolStateDraining:
		return true
	case RunnerPoolStateRevoked:
		return true
	default:
		return false
	}
}

const (
	RunnerPoolInputStateActive   RunnerPoolInputState = "active"
	RunnerPoolInputStateDraining RunnerPoolInputState = "draining"
	RunnerPoolInputStateRevoked  RunnerPoolInputState = "revoked"
)

func (e RunnerPoolInputState) Valid() bool {
	switch e {
	case RunnerPoolInputStateActive:
		return true
	case RunnerPoolInputStateDraining:
		return true
	case RunnerPoolInputStateRevoked:
		return true
	default:
		return false
	}
}

const (
	TaskStateBlocked     TaskState = "blocked"
	TaskStateCancelled   TaskState = "cancelled"
	TaskStateCancelling  TaskState = "cancelling"
	TaskStateCompleted   TaskState = "completed"
	TaskStateFailed      TaskState = "failed"
	TaskStatePlanning    TaskState = "planning"
	TaskStatePublishing  TaskState = "publishing"
	TaskStateQueued      TaskState = "queued"
	TaskStateReconciling TaskState = "reconciling"
	TaskStateRepairing   TaskState = "repairing"
	TaskStateReproducing TaskState = "reproducing"
	TaskStateValidating  TaskState = "validating"
)

func (e TaskState) Valid() bool {
	switch e {
	case TaskStateBlocked:
		return true
	case TaskStateCancelled:
		return true
	case TaskStateCancelling:
		return true
	case TaskStateCompleted:
		return true
	case TaskStateFailed:
		return true
	case TaskStatePlanning:
		return true
	case TaskStatePublishing:
		return true
	case TaskStateQueued:
		return true
	case TaskStateReconciling:
		return true
	case TaskStateRepairing:
		return true
	case TaskStateReproducing:
		return true
	case TaskStateValidating:
		return true
	default:
		return false
	}
}

type APIError struct {
	Code      string                  `json:"code"`
	Details   *map[string]interface{} `json:"details,omitempty"`
	Message   string                  `json:"message"`
	RequestId string                  `json:"request_id"`
	Retryable bool                    `json:"retryable"`
}
type AdvisoryInput struct {
	AdvisoryId    string `json:"advisory_id"`
	AffectedRange string `json:"affected_range"`
	CommitSha     string `json:"commit_sha"`
	Ecosystem     string `json:"ecosystem"`
	Package       string `json:"package"`
	Path          string `json:"path"`
	ReferenceUrl  string `json:"reference_url"`
	RepositoryId  string `json:"repository_id"`
	Severity      string `json:"severity"`
	Title         string `json:"title"`
}
type ArtifactMetadata struct {
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Id           string    `json:"id"`
	MediaType    string    `json:"media_type"`
	Name         string    `json:"name"`
	OrgId        string    `json:"org_id"`
	RepositoryId string    `json:"repository_id"`
	Sha256       string    `json:"sha256"`
	Size         int64     `json:"size"`
	TaskId       string    `json:"task_id"`
}
type ArtifactProvenance struct {
	ArtifactDigest string    `json:"artifact_digest"`
	BuildId        string    `json:"build_id"`
	ExpiresAt      time.Time `json:"expires_at"`
	IssuedAt       time.Time `json:"issued_at"`
	OrgId          string    `json:"org_id"`
	RepositoryId   string    `json:"repository_id"`
	SourceSha      string    `json:"source_sha"`
}
type AuditEvent struct {
	Action       string                 `json:"action"`
	ActorId      string                 `json:"actor_id"`
	CreatedAt    time.Time              `json:"created_at"`
	Data         map[string]interface{} `json:"data"`
	Id           string                 `json:"id"`
	ObjectId     string                 `json:"object_id"`
	RepositoryId *string                `json:"repository_id,omitempty"`
	RequestId    string                 `json:"request_id"`
}
type AuditEventPage struct {
	Complete   bool         `json:"complete"`
	Items      []AuditEvent `json:"items"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
type BootstrapRequest struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}
type BotRevalidation struct {
	ChangeId    string               `json:"change_id"`
	CompanionId string               `json:"companion_id"`
	Gate        *MergeGate           `json:"gate,omitempty"`
	ObservedAt  *time.Time           `json:"observed_at"`
	Reason      string               `json:"reason"`
	State       BotRevalidationState `json:"state"`
	TaskId      string               `json:"task_id"`
}
type BotRevalidationState string
type BudgetAmount struct {
	Concurrency  int64 `json:"concurrency"`
	MicroUsd     int64 `json:"micro_usd"`
	Milliseconds int64 `json:"milliseconds"`
	Requests     int64 `json:"requests"`
	Tokens       int64 `json:"tokens"`
}
type BudgetCaps struct {
	Concurrency  *int64 `json:"concurrency,omitempty"`
	MicroUsd     *int64 `json:"micro_usd,omitempty"`
	Milliseconds *int64 `json:"milliseconds,omitempty"`
	Requests     *int64 `json:"requests,omitempty"`
	Tokens       *int64 `json:"tokens,omitempty"`
}
type BudgetLimit struct {
	Caps    BudgetCaps        `json:"caps"`
	End     *time.Time        `json:"end,omitempty"`
	Held    BudgetAmount      `json:"held"`
	Paused  bool              `json:"paused"`
	Period  BudgetLimitPeriod `json:"period"`
	Scope   BudgetScope       `json:"scope"`
	Spent   BudgetAmount      `json:"spent"`
	Start   *time.Time        `json:"start,omitempty"`
	Version int64             `json:"version"`
}
type BudgetLimitPeriod string
type BudgetLimitInput struct {
	Caps   BudgetCaps             `json:"caps"`
	End    *time.Time             `json:"end,omitempty"`
	Paused bool                   `json:"paused"`
	Period BudgetLimitInputPeriod `json:"period"`
	Start  *time.Time             `json:"start,omitempty"`
}
type BudgetLimitInputPeriod string
type BudgetQuote struct {
	InputTokens     int64  `json:"input_tokens"`
	MaxMilliseconds int64  `json:"max_milliseconds"`
	MaxOutputTokens int64  `json:"max_output_tokens"`
	MaxRequests     int64  `json:"max_requests"`
	Model           string `json:"model"`
	OperationId     string `json:"operation_id"`
	Route           string `json:"route"`
	RouteVersion    int64  `json:"route_version"`
}
type BudgetReservation struct {
	Actual            *BudgetAmount         `json:"actual,omitempty"`
	CampaignId        *string               `json:"campaign_id,omitempty"`
	ConnectionId      string                `json:"connection_id"`
	ConnectionVersion int64                 `json:"connection_version"`
	CreatedAt         time.Time             `json:"created_at"`
	Debt              BudgetAmount          `json:"debt"`
	DispatchedAt      *time.Time            `json:"dispatched_at,omitempty"`
	Id                string                `json:"id"`
	Lease             WorkflowLease         `json:"lease"`
	Maximum           BudgetAmount          `json:"maximum"`
	Quote             BudgetQuote           `json:"quote"`
	Reference         *string               `json:"reference,omitempty"`
	Route             BudgetRoute           `json:"route"`
	Scopes            []BudgetScopeSnapshot `json:"scopes"`
	State             string                `json:"state"`
}
type BudgetRoute struct {
	ConnectionId             string `json:"connection_id"`
	InputMicroUsdPerMillion  int64  `json:"input_micro_usd_per_million"`
	MaxInputTokens           int64  `json:"max_input_tokens"`
	MaxMilliseconds          int64  `json:"max_milliseconds"`
	MaxOutputTokens          int64  `json:"max_output_tokens"`
	MaxRequests              int64  `json:"max_requests"`
	Mode                     string `json:"mode"`
	Model                    string `json:"model"`
	Name                     string `json:"name"`
	OutputMicroUsdPerMillion int64  `json:"output_micro_usd_per_million"`
	Paused                   bool   `json:"paused"`
	PricingVersion           string `json:"pricing_version"`
	QualificationRef         string `json:"qualification_ref"`
	Qualified                bool   `json:"qualified"`
	RequestMicroUsd          int64  `json:"request_micro_usd"`
	Version                  int64  `json:"version"`
}
type BudgetRouteInput struct {
	InputMicroUsdPerMillion  int64  `json:"input_micro_usd_per_million"`
	MaxInputTokens           int64  `json:"max_input_tokens"`
	MaxMilliseconds          int64  `json:"max_milliseconds"`
	MaxOutputTokens          int64  `json:"max_output_tokens"`
	MaxRequests              int64  `json:"max_requests"`
	Mode                     string `json:"mode"`
	Model                    string `json:"model"`
	Name                     string `json:"name"`
	OutputMicroUsdPerMillion int64  `json:"output_micro_usd_per_million"`
	Paused                   bool   `json:"paused"`
	PricingVersion           string `json:"pricing_version"`
	RequestMicroUsd          int64  `json:"request_micro_usd"`
}
type BudgetScope struct {
	Id   string `json:"id"`
	Kind string `json:"kind"`
}
type BudgetScopeSnapshot struct {
	PeriodStart time.Time   `json:"period_start"`
	Scope       BudgetScope `json:"scope"`
	Version     int64       `json:"version"`
}
type Campaign struct {
	Counts         CampaignCounts `json:"counts"`
	CreatedAt      time.Time      `json:"created_at"`
	GrantExpiresAt time.Time      `json:"grant_expires_at"`
	Id             string         `json:"id"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	ObservingSince *time.Time     `json:"observing_since,omitempty"`
	Reason         string         `json:"reason"`
	RequestedBy    string         `json:"requested_by"`
	Spec           CampaignInput  `json:"spec"`
	Stage          int            `json:"stage"`
	State          string         `json:"state"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Version        int64          `json:"version"`
}
type CampaignControlInput struct {
	Reason        string                             `json:"reason"`
	StageDecision *CampaignControlInputStageDecision `json:"stage_decision,omitempty"`
}
type CampaignControlInputStageDecision string
type CampaignCounts struct {
	Excluded  int `json:"excluded"`
	Failed    int `json:"failed"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Total     int `json:"total"`
	Unknown   int `json:"unknown"`
}
type CampaignCreateInput struct {
	IdempotencyKey string `json:"idempotency_key"`
	PreviewId      string `json:"preview_id"`
}
type CampaignInput struct {
	BatchSize          int                   `json:"batch_size"`
	CanaryIds          []string              `json:"canary_ids"`
	CanarySize         int                   `json:"canary_size"`
	Concurrency        int                   `json:"concurrency"`
	FailureLimit       int                   `json:"failure_limit"`
	FailurePercent     int                   `json:"failure_percent"`
	Kind               CampaignInputKind     `json:"kind"`
	Members            []CampaignMemberInput `json:"members"`
	Name               string                `json:"name"`
	NotBefore          *time.Time            `json:"not_before,omitempty"`
	ObservationSeconds int64                 `json:"observation_seconds"`
	Selection          string                `json:"selection"`
	Success            CampaignInputSuccess  `json:"success"`
	Windows            []CampaignWindow      `json:"windows"`
}
type CampaignInputKind string
type CampaignInputSuccess string
type CampaignMember struct {
	ActionId       *string             `json:"action_id,omitempty"`
	Canary         bool                `json:"canary"`
	Group          string              `json:"group"`
	Halt           bool                `json:"halt"`
	Id             string              `json:"id"`
	Input          CampaignMemberInput `json:"input"`
	Pins           map[string]string   `json:"pins"`
	Reason         string              `json:"reason"`
	Repositories   []string            `json:"repositories"`
	RepositoryId   string              `json:"repository_id"`
	RepositoryName string              `json:"repository_name"`
	Stage          int                 `json:"stage"`
	State          string              `json:"state"`
	SucceededAt    *time.Time          `json:"succeeded_at,omitempty"`
}
type CampaignMemberInput struct {
	Environment  *string                 `json:"environment,omitempty"`
	Gitops       *GitOpsPreviewInput     `json:"gitops,omitempty"`
	Pipeline     *DeploymentPreviewInput `json:"pipeline,omitempty"`
	Repair       *RepairInput            `json:"repair,omitempty"`
	RepositoryId string                  `json:"repository_id"`
}
type CampaignMemberPage struct {
	Complete   bool             `json:"complete"`
	Items      []CampaignMember `json:"items"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}
type CampaignPage struct {
	Complete   bool       `json:"complete"`
	Items      []Campaign `json:"items"`
	NextCursor *string    `json:"next_cursor,omitempty"`
}
type CampaignPreview struct {
	Blockers  []string         `json:"blockers"`
	ExpiresAt time.Time        `json:"expires_at"`
	Hash      string           `json:"hash"`
	Id        string           `json:"id"`
	Input     CampaignInput    `json:"input"`
	Members   []CampaignMember `json:"members"`
}
type CampaignWindow struct {
	EndMinute   int   `json:"end_minute"`
	StartMinute int   `json:"start_minute"`
	Weekdays    []int `json:"weekdays"`
}
type Capability struct {
	LastChecked time.Time       `json:"last_checked"`
	Reason      string          `json:"reason"`
	Scope       string          `json:"scope"`
	Source      string          `json:"source"`
	State       CapabilityState `json:"state"`
	Version     string          `json:"version"`
}
type CapabilityState string
type ChangeState string
type Connection struct {
	Capabilities      map[string]ConnectionCapability `json:"capabilities"`
	CredentialVersion int64                           `json:"credential_version"`
	Endpoint          string                          `json:"endpoint"`
	Id                string                          `json:"id"`
	Kind              string                          `json:"kind"`
	Name              string                          `json:"name"`
	OrgId             string                          `json:"org_id"`
	PrivateRoute      *PrivateRoute                   `json:"private_route,omitempty"`
	Provider          string                          `json:"provider"`
	Reason            string                          `json:"reason"`
	ServerVersion     string                          `json:"server_version"`
	Settings          ConnectionSettings              `json:"settings"`
	State             string                          `json:"state"`
	VerifiedAt        *time.Time                      `json:"verified_at"`
	Version           int64                           `json:"version"`
}
type ConnectionCapability struct {
	LastChecked time.Time `json:"last_checked"`
	Reason      string    `json:"reason"`
	Scope       string    `json:"scope"`
	Source      string    `json:"source"`
	State       string    `json:"state"`
	Version     string    `json:"version"`
}
type ConnectionCreate struct {
	Endpoint     string             `json:"endpoint"`
	Kind         string             `json:"kind"`
	Name         string             `json:"name"`
	PrivateRoute *PrivateRoute      `json:"private_route,omitempty"`
	Provider     string             `json:"provider"`
	Secret       *string            `json:"secret,omitempty"`
	Settings     ConnectionSettings `json:"settings"`
}
type ConnectionPage struct {
	Complete   bool         `json:"complete"`
	Items      []Connection `json:"items"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
type ConnectionSettings struct {
	AllowedModels  *[]string `json:"allowed_models,omitempty"`
	AppId          *string   `json:"app_id,omitempty"`
	AuthKind       string    `json:"auth_kind"`
	BillingRoute   string    `json:"billing_route"`
	CaPem          *string   `json:"ca_pem,omitempty"`
	InstallationId *string   `json:"installation_id,omitempty"`
	Model          *string   `json:"model,omitempty"`
	Namespace      *string   `json:"namespace,omitempty"`
	Profile        *string   `json:"profile,omitempty"`
	RuntimeVersion *string   `json:"runtime_version,omitempty"`
}
type CredentialRotation struct {
	Secret *string `json:"secret,omitempty"`
}
type Decision struct {
	Blockers        []string        `json:"blockers"`
	Outcome         DecisionOutcome `json:"outcome"`
	PolicyHash      string          `json:"policy_hash"`
	RequiredActions []string        `json:"required_actions"`
	Rules           []string        `json:"rules"`
}
type DecisionOutcome string
type DeliveryPipelineRequest struct {
	ArtifactDigest string            `json:"artifact_digest"`
	ConfigSha256   string            `json:"config_sha256"`
	CorrelationId  string            `json:"correlation_id"`
	Environment    string            `json:"environment"`
	Inputs         map[string]string `json:"inputs"`
	ObserveOnly    bool              `json:"observe_only"`
	Ref            string            `json:"ref"`
	Repository     ForgeRepoRef      `json:"repository"`
	RequestedAt    time.Time         `json:"requested_at"`
	RulesHash      string            `json:"rules_hash"`
	RunId          string            `json:"run_id"`
	SourceSha      string            `json:"source_sha"`
	WorkflowId     string            `json:"workflow_id"`
	WorkflowPath   string            `json:"workflow_path"`
	WorkflowSha    string            `json:"workflow_sha"`
}
type DeliveryQualification struct {
	ConnectionVersion        int64     `json:"connection_version"`
	EnvironmentSerialization bool      `json:"environment_serialization"`
	EvidenceReference        string    `json:"evidence_reference"`
	EvidenceSha256           string    `json:"evidence_sha256"`
	ExpiresAt                time.Time `json:"expires_at"`
	NativeEnforcement        bool      `json:"native_enforcement"`
	NoBypass                 bool      `json:"no_bypass"`
	PinnedInputs             bool      `json:"pinned_inputs"`
	Provider                 string    `json:"provider"`
	ServerVersion            string    `json:"server_version"`
	VerifiedAt               time.Time `json:"verified_at"`
}
type DeliveryWorkflow struct {
	ConfigSha256 string            `json:"config_sha256"`
	Id           string            `json:"id"`
	Inputs       map[string]string `json:"inputs"`
	Path         string            `json:"path"`
	Ref          string            `json:"ref"`
	Sha          string            `json:"sha"`
}
type DeploymentConfiguration struct {
	DeadlineSeconds       int64                       `json:"deadline_seconds"`
	Enabled               bool                        `json:"enabled"`
	Environment           string                      `json:"environment"`
	HealthChecks          []string                    `json:"health_checks"`
	HealthPublicKey       string                      `json:"health_public_key"`
	MaxEvidenceAgeSeconds int64                       `json:"max_evidence_age_seconds"`
	Mode                  DeploymentConfigurationMode `json:"mode"`
	NativeEnvironment     string                      `json:"native_environment"`
	ObservationSeconds    int64                       `json:"observation_seconds"`
	ProvenancePublicKey   string                      `json:"provenance_public_key"`
	Qualification         DeliveryQualification       `json:"qualification"`
	RecoveryWorkflow      *DeliveryWorkflow           `json:"recovery_workflow,omitempty"`
	RepositoryId          string                      `json:"repository_id"`
	Version               int64                       `json:"version"`
	Workflow              DeliveryWorkflow            `json:"workflow"`
}
type DeploymentConfigurationMode string
type DeploymentConfigurationPage struct {
	Items []DeploymentConfiguration `json:"items"`
}
type DeploymentDetail struct {
	Gate      DeploymentGate      `json:"gate"`
	Health    *DeploymentHealth   `json:"health,omitempty"`
	Operation DeploymentOperation `json:"operation"`
}
type DeploymentGate struct {
	Binding              PolicyEvidenceBinding   `json:"binding"`
	Blockers             []string                `json:"blockers"`
	ConfigurationVersion int64                   `json:"configuration_version"`
	ConnectionId         string                  `json:"connection_id"`
	ConnectionVersion    int64                   `json:"connection_version"`
	Decision             PolicyResult            `json:"decision"`
	Environment          string                  `json:"environment"`
	ExpiresAt            time.Time               `json:"expires_at"`
	Id                   string                  `json:"id"`
	Native               NativeDeploymentGates   `json:"native"`
	Pipeline             DeliveryPipelineRequest `json:"pipeline"`
	RepositoryId         string                  `json:"repository_id"`
	Request              DeploymentPreviewInput  `json:"request"`
}
type DeploymentHealth struct {
	ArtifactDigest       string          `json:"artifact_digest"`
	Checks               map[string]bool `json:"checks"`
	ConfigurationVersion int64           `json:"configuration_version"`
	DeploymentId         string          `json:"deployment_id"`
	Environment          string          `json:"environment"`
	Healthy              bool            `json:"healthy"`
	Nonce                string          `json:"nonce"`
	ObservedAt           time.Time       `json:"observed_at"`
	OrgId                string          `json:"org_id"`
	Revision             string          `json:"revision"`
	RunAttempt           int64           `json:"run_attempt"`
	RunId                string          `json:"run_id"`
	SourceSha            string          `json:"source_sha"`
}
type DeploymentOperation struct {
	CancelRequested bool                           `json:"cancel_requested"`
	CancelState     DeploymentOperationCancelState `json:"cancel_state"`
	CreatedAt       time.Time                      `json:"created_at"`
	Environment     string                         `json:"environment"`
	FinishedAt      *time.Time                     `json:"finished_at,omitempty"`
	GateId          string                         `json:"gate_id"`
	Id              string                         `json:"id"`
	Native          *NativeDeploymentStatus        `json:"native,omitempty"`
	Reason          string                         `json:"reason"`
	RecoveryOf      *string                        `json:"recovery_of,omitempty"`
	RepositoryId    string                         `json:"repository_id"`
	RequestedBy     string                         `json:"requested_by"`
	State           string                         `json:"state"`
	UpdatedAt       time.Time                      `json:"updated_at"`
	Version         int64                          `json:"version"`
}
type DeploymentOperationCancelState string
type DeploymentOperationPage struct {
	Complete   bool                  `json:"complete"`
	Items      []DeploymentOperation `json:"items"`
	NextCursor *string               `json:"next_cursor,omitempty"`
}
type DeploymentPreviewInput struct {
	ArtifactDigest      string                   `json:"artifact_digest"`
	ChangeId            string                   `json:"change_id"`
	Provenance          SignedArtifactProvenance `json:"provenance"`
	RecoveryOf          *string                  `json:"recovery_of,omitempty"`
	RestoreDeploymentId *string                  `json:"restore_deployment_id,omitempty"`
	SourceSha           string                   `json:"source_sha"`
}
type DeploymentRequestInput struct {
	GateId         string `json:"gate_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
type DeploymentState string
type DeploymentTrackInput struct {
	ArtifactDigest      string                   `json:"artifact_digest"`
	ChangeId            string                   `json:"change_id"`
	IdempotencyKey      string                   `json:"idempotency_key"`
	Provenance          SignedArtifactProvenance `json:"provenance"`
	RecoveryOf          *string                  `json:"recovery_of,omitempty"`
	RestoreDeploymentId *string                  `json:"restore_deployment_id,omitempty"`
	RunId               string                   `json:"run_id"`
	SourceSha           string                   `json:"source_sha"`
}
type DiscoveryScan struct {
	ObservedAt   *time.Time         `json:"observed_at,omitempty"`
	Reason       string             `json:"reason"`
	RepositoryId string             `json:"repository_id"`
	State        DiscoveryScanState `json:"state"`
	Version      int64              `json:"version"`
}
type DiscoveryScanState string
type EnrollmentToken struct {
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token"`
}
type Event struct {
	AggregateId      string                 `json:"aggregate_id"`
	AggregateType    string                 `json:"aggregate_type"`
	AggregateVersion int64                  `json:"aggregate_version"`
	Data             map[string]interface{} `json:"data"`
	DataVersion      int                    `json:"data_version"`
	Id               int64                  `json:"id"`
	OccurredAt       time.Time              `json:"occurred_at"`
	OrgId            string                 `json:"org_id"`
	RepositoryId     *string                `json:"repository_id,omitempty"`
	RequestId        string                 `json:"request_id"`
	Type             string                 `json:"type"`
}
type EventPage struct {
	Complete bool    `json:"complete"`
	Cursor   int64   `json:"cursor"`
	Items    []Event `json:"items"`
}
type Finding struct {
	AssignedTo     *string             `json:"assigned_to,omitempty"`
	Category       string              `json:"category"`
	Evidence       MaintenanceEvidence `json:"evidence"`
	EvidenceDigest string              `json:"evidence_digest"`
	Fingerprint    string              `json:"fingerprint"`
	FirstSeen      time.Time           `json:"first_seen"`
	Id             string              `json:"id"`
	LastSeen       time.Time           `json:"last_seen"`
	OrgId          string              `json:"org_id"`
	Reason         string              `json:"reason"`
	RepositoryId   string              `json:"repository_id"`
	Severity       FindingSeverity     `json:"severity"`
	SnoozeUntil    *time.Time          `json:"snooze_until,omitempty"`
	Source         string              `json:"source"`
	SourceId       string              `json:"source_id"`
	State          FindingState        `json:"state"`
	SupersededBy   *string             `json:"superseded_by,omitempty"`
	Title          string              `json:"title"`
	Version        int64               `json:"version"`
}
type FindingSeverity string
type FindingState string
type FindingPage struct {
	Complete   bool      `json:"complete"`
	Items      []Finding `json:"items"`
	NextCursor *string   `json:"next_cursor,omitempty"`
}
type FindingUpdate struct {
	Action      FindingUpdateAction `json:"action"`
	AssignedTo  *string             `json:"assigned_to,omitempty"`
	Reason      *string             `json:"reason,omitempty"`
	SnoozeUntil *time.Time          `json:"snooze_until,omitempty"`
}
type FindingUpdateAction string
type ForgeApproval struct {
	ActorId   string `json:"actor_id"`
	Dismissed bool   `json:"dismissed"`
	HeadSha   string `json:"head_sha"`
	Id        string `json:"id"`
	State     string `json:"state"`
}
type ForgeChange struct {
	AuthorId         string       `json:"author_id"`
	AuthorLogin      string       `json:"author_login"`
	AuthorType       string       `json:"author_type"`
	Body             string       `json:"body"`
	Draft            bool         `json:"draft"`
	HeadBranch       string       `json:"head_branch"`
	HeadRepository   ForgeRepoRef `json:"head_repository"`
	HeadSha          string       `json:"head_sha"`
	Id               string       `json:"id"`
	MergeSha         string       `json:"merge_sha"`
	MergeStatus      string       `json:"merge_status"`
	OperationId      string       `json:"operation_id"`
	Repository       ForgeRepoRef `json:"repository"`
	State            string       `json:"state"`
	TargetBranch     string       `json:"target_branch"`
	TargetRepository ForgeRepoRef `json:"target_repository"`
	TargetSha        string       `json:"target_sha"`
	Title            string       `json:"title"`
	Url              string       `json:"url"`
}
type ForgeCheck struct {
	Conclusion  string `json:"conclusion"`
	HeadSha     string `json:"head_sha"`
	Id          string `json:"id"`
	Name        string `json:"name"`
	PublisherId string `json:"publisher_id"`
	Status      string `json:"status"`
	Url         string `json:"url"`
}
type ForgeCheckRule struct {
	Name        string `json:"name"`
	PublisherId string `json:"publisher_id"`
}
type ForgeMergeCapabilities struct {
	Features      map[string]Capability `json:"features"`
	Provider      string                `json:"provider"`
	ServerVersion string                `json:"server_version"`
}
type ForgeMergeResult struct {
	HeadSha  string `json:"head_sha"`
	MergeSha string `json:"merge_sha"`
	NativeId string `json:"native_id"`
	State    string `json:"state"`
	Url      string `json:"url"`
}
type ForgeNativeEligibility struct {
	Blockers  []string `json:"blockers"`
	HeadSha   string   `json:"head_sha"`
	State     string   `json:"state"`
	TargetSha string   `json:"target_sha"`
}
type ForgeQueueState struct {
	HeadSha   string `json:"head_sha"`
	Id        string `json:"id"`
	State     string `json:"state"`
	TargetSha string `json:"target_sha"`
	TestedSha string `json:"tested_sha"`
}
type ForgeRepoRef struct {
	FullName string `json:"full_name"`
	NativeId string `json:"native_id"`
}
type ForgeRepository struct {
	Archived      bool      `json:"archived"`
	CloneUrl      string    `json:"clone_url"`
	DefaultBranch string    `json:"default_branch"`
	FullName      string    `json:"full_name"`
	NativeId      string    `json:"native_id"`
	Permissions   *[]string `json:"permissions"`
	Private       bool      `json:"private"`
	Url           string    `json:"url"`
}
type ForgeRules struct {
	ActorCanBypass       bool             `json:"actor_can_bypass"`
	AllowedMergeMethods  []string         `json:"allowed_merge_methods"`
	CodeOwnersEnforced   string           `json:"code_owners_enforced"`
	DismissStaleReviews  bool             `json:"dismiss_stale_reviews"`
	Hash                 string           `json:"hash"`
	ObservedAt           time.Time        `json:"observed_at"`
	Reason               string           `json:"reason"`
	RequireCodeOwners    bool             `json:"require_code_owners"`
	RequireQueue         bool             `json:"require_queue"`
	RequireStrictTarget  bool             `json:"require_strict_target"`
	RequiredApprovals    int64            `json:"required_approvals"`
	RequiredChecks       []ForgeCheckRule `json:"required_checks"`
	State                string           `json:"state"`
	StrictTargetEnforced string           `json:"strict_target_enforced"`
}
type GitOpsConfiguration struct {
	DeadlineSeconds       int64    `json:"deadline_seconds"`
	DeliveryRepositoryId  string   `json:"delivery_repository_id"`
	Enabled               bool     `json:"enabled"`
	Environment           string   `json:"environment"`
	HealthChecks          []string `json:"health_checks"`
	HealthPublicKey       string   `json:"health_public_key"`
	ImageRepository       string   `json:"image_repository"`
	ManifestPath          string   `json:"manifest_path"`
	MaxEvidenceAgeSeconds int64    `json:"max_evidence_age_seconds"`
	ObservationSeconds    int64    `json:"observation_seconds"`
	Pointer               string   `json:"pointer"`
	ProvenancePublicKey   string   `json:"provenance_public_key"`
	RecoveryAllowed       bool     `json:"recovery_allowed"`
	SourceRepositoryId    string   `json:"source_repository_id"`
	TargetBranch          string   `json:"target_branch"`
	Version               int64    `json:"version"`
}
type GitOpsConfigurationPage struct {
	Items []GitOpsConfiguration `json:"items"`
}
type GitOpsDetail struct {
	Gate      GitOpsGate      `json:"gate"`
	Health    *GitOpsHealth   `json:"health,omitempty"`
	Promotion GitOpsPromotion `json:"promotion"`
}
type GitOpsGate struct {
	After         string              `json:"after"`
	Before        string              `json:"before"`
	Blockers      []string            `json:"blockers"`
	Configuration GitOpsConfiguration `json:"configuration"`
	Decision      PolicyResult        `json:"decision"`
	Delivery      struct {
		FullName string `json:"full_name"`
		NativeId string `json:"native_id"`
	} `json:"delivery"`
	DeliveryConnectionId      string             `json:"delivery_connection_id"`
	DeliveryConnectionVersion int64              `json:"delivery_connection_version"`
	DeliveryPolicyHash        string             `json:"delivery_policy_hash"`
	ExpiresAt                 time.Time          `json:"expires_at"`
	Id                        string             `json:"id"`
	ManifestSha256            string             `json:"manifest_sha256"`
	OperationId               string             `json:"operation_id"`
	PatchedManifest           []byte             `json:"patched_manifest"`
	Request                   GitOpsPreviewInput `json:"request"`
	Source                    struct {
		FullName string `json:"full_name"`
		NativeId string `json:"native_id"`
	} `json:"source"`
	SourceConnectionId      string `json:"source_connection_id"`
	SourceConnectionVersion int64  `json:"source_connection_version"`
	SourcePolicyHash        string `json:"source_policy_hash"`
	TargetSha               string `json:"target_sha"`
}
type GitOpsHealth struct {
	ArtifactDigest       string          `json:"artifact_digest"`
	Checks               map[string]bool `json:"checks"`
	ConfigurationVersion int64           `json:"configuration_version"`
	DeliveryRevision     string          `json:"delivery_revision"`
	Environment          string          `json:"environment"`
	Healthy              bool            `json:"healthy"`
	Nonce                string          `json:"nonce"`
	ObservedAt           time.Time       `json:"observed_at"`
	OrgId                string          `json:"org_id"`
	PromotionId          string          `json:"promotion_id"`
	SourceSha            string          `json:"source_sha"`
}
type GitOpsPreviewInput struct {
	ArtifactDigest     string                   `json:"artifact_digest"`
	ChangeId           string                   `json:"change_id"`
	Provenance         SignedArtifactProvenance `json:"provenance"`
	RecoveryOf         *string                  `json:"recovery_of,omitempty"`
	RestorePromotionId *string                  `json:"restore_promotion_id,omitempty"`
	SourceSha          string                   `json:"source_sha"`
}
type GitOpsPromotion struct {
	Branch               string       `json:"branch"`
	CancelRequested      bool         `json:"cancel_requested"`
	CandidateSha         string       `json:"candidate_sha"`
	Change               *ForgeChange `json:"change,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	DeliveryRepositoryId string       `json:"delivery_repository_id"`
	Environment          string       `json:"environment"`
	FinishedAt           *time.Time   `json:"finished_at,omitempty"`
	GateId               string       `json:"gate_id"`
	Id                   string       `json:"id"`
	MergeSha             string       `json:"merge_sha"`
	Reason               string       `json:"reason"`
	RecoveryOf           *string      `json:"recovery_of,omitempty"`
	RequestedBy          string       `json:"requested_by"`
	SourceRepositoryId   string       `json:"source_repository_id"`
	State                string       `json:"state"`
	UpdatedAt            time.Time    `json:"updated_at"`
	Version              int64        `json:"version"`
}
type GitOpsPromotionPage struct {
	Complete   bool              `json:"complete"`
	Items      []GitOpsPromotion `json:"items"`
	NextCursor *string           `json:"next_cursor,omitempty"`
}
type Health struct {
	Status string `json:"status"`
}
type InventoryCandidatePage struct {
	Complete   bool              `json:"complete"`
	Items      []ForgeRepository `json:"items"`
	NextCursor *string           `json:"next_cursor,omitempty"`
}
type InventoryChangePage struct {
	Complete          bool          `json:"complete"`
	ConnectionVersion int64         `json:"connection_version"`
	Items             []ForgeChange `json:"items"`
	NextCursor        *string       `json:"next_cursor,omitempty"`
	ObservedAt        *time.Time    `json:"observed_at,omitempty"`
	SnapshotState     string        `json:"snapshot_state"`
}
type InventoryImportInput struct {
	All       *bool     `json:"all,omitempty"`
	NativeIds *[]string `json:"native_ids,omitempty"`
	TeamIds   *[]string `json:"team_ids,omitempty"`
}
type InventoryJob struct {
	AvailableAt       time.Time         `json:"available_at"`
	ConnectionId      string            `json:"connection_id"`
	ConnectionVersion int64             `json:"connection_version"`
	CreatedAt         time.Time         `json:"created_at"`
	Failures          int64             `json:"failures"`
	Id                string            `json:"id"`
	Kind              InventoryJobKind  `json:"kind"`
	Namespace         string            `json:"namespace"`
	OrgId             string            `json:"org_id"`
	Pages             int64             `json:"pages"`
	ParentId          *string           `json:"parent_id,omitempty"`
	Processed         int64             `json:"processed"`
	Reason            *string           `json:"reason,omitempty"`
	RepositoryId      *string           `json:"repository_id,omitempty"`
	State             InventoryJobState `json:"state"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Version           int64             `json:"version"`
}
type InventoryJobKind string
type InventoryJobState string
type InventoryJobPage struct {
	Complete   bool           `json:"complete"`
	Items      []InventoryJob `json:"items"`
	NextCursor *string        `json:"next_cursor,omitempty"`
}
type InventorySyncInput struct {
	ConnectionId string  `json:"connection_id"`
	Namespace    *string `json:"namespace,omitempty"`
}
type InventoryWebhook struct {
	ConnectionId string `json:"connection_id"`
	Id           string `json:"id"`
	Path         string `json:"path"`
	Revoked      bool   `json:"revoked"`
	Version      int64  `json:"version"`
}
type IssuedInventoryWebhook struct {
	ConnectionId string `json:"connection_id"`
	Id           string `json:"id"`
	Path         string `json:"path"`
	Revoked      bool   `json:"revoked"`
	Secret       string `json:"secret"`
	Version      int64  `json:"version"`
}
type MaintenanceBotConfig struct {
	Dependabot MaintenanceBotStatus `json:"dependabot"`
	Renovate   MaintenanceBotStatus `json:"renovate"`
}
type MaintenanceBotIdentity struct {
	ActorId string                     `json:"actor_id"`
	Kind    MaintenanceBotIdentityKind `json:"kind"`
}
type MaintenanceBotIdentityKind string
type MaintenanceBotStatus struct {
	Automerge MaintenanceBotStatusAutomerge `json:"automerge"`
	Present   bool                          `json:"present"`
}
type MaintenanceBotStatusAutomerge string
type MaintenanceConfig struct {
	MergeAuthority MaintenanceConfigMergeAuthority `json:"merge_authority"`
	RepositoryId   string                          `json:"repository_id"`
	TrustedBots    []MaintenanceBotIdentity        `json:"trusted_bots"`
	Version        int64                           `json:"version"`
}
type MaintenanceConfigMergeAuthority string
type MaintenanceDependency struct {
	Ecosystem string `json:"ecosystem"`
	From      string `json:"from"`
	Manifest  string `json:"manifest"`
	Name      string `json:"name"`
	To        string `json:"to"`
}
type MaintenanceEvidence struct {
	AdvisoryId *string               `json:"advisory_id,omitempty"`
	Blockers   []string              `json:"blockers"`
	Bot        *string               `json:"bot,omitempty"`
	BotConfig  *MaintenanceBotConfig `json:"bot_config,omitempty"`
	Change     *ForgeChange          `json:"change,omitempty"`
	Checks     []struct {
		Conclusion  string `json:"conclusion"`
		HeadSha     string `json:"head_sha"`
		Id          string `json:"id"`
		Name        string `json:"name"`
		PublisherId string `json:"publisher_id"`
		Status      string `json:"status"`
		Url         string `json:"url"`
	} `json:"checks"`
	Complete          bool                    `json:"complete"`
	ConfigVersion     int64                   `json:"config_version"`
	ConnectionId      string                  `json:"connection_id"`
	ConnectionVersion int64                   `json:"connection_version"`
	Dependencies      []MaintenanceDependency `json:"dependencies"`
	HeadOwnership     *string                 `json:"head_ownership,omitempty"`
	HeadSha           string                  `json:"head_sha"`
	MergeBlockers     *[]string               `json:"merge_blockers,omitempty"`
	Ownership         string                  `json:"ownership"`
	Provenance        string                  `json:"provenance"`
	ReferenceUrl      *string                 `json:"reference_url,omitempty"`
	TargetBranch      string                  `json:"target_branch"`
	TargetSha         string                  `json:"target_sha"`
}
type Membership struct {
	AllRepositories bool     `json:"all_repositories"`
	OrgId           string   `json:"org_id"`
	RepositoryIds   []string `json:"repository_ids"`
	Role            Role     `json:"role"`
	TeamIds         []string `json:"team_ids"`
	UserId          *string  `json:"user_id,omitempty"`
	Version         *int64   `json:"version,omitempty"`
}
type MembershipInput struct {
	AllRepositories bool     `json:"all_repositories"`
	RepositoryIds   []string `json:"repository_ids"`
	Role            Role     `json:"role"`
	TeamIds         []string `json:"team_ids"`
}
type MembershipPage struct {
	Complete   bool         `json:"complete"`
	Items      []Membership `json:"items"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
type MergeCompanion struct {
	ChangeId string `json:"change_id"`
	HeadSha  string `json:"head_sha"`
	MergeSha string `json:"merge_sha"`
	State    string `json:"state"`
	TaskId   string `json:"task_id"`
}
type MergeConfiguration struct {
	CheckPublishers       map[string]string  `json:"check_publishers"`
	CooperationReference  string             `json:"cooperation_reference"`
	Enabled               bool               `json:"enabled"`
	InspectorConnectionId *string            `json:"inspector_connection_id,omitempty"`
	Qualification         MergeQualification `json:"qualification"`
	RepositoryId          string             `json:"repository_id"`
	Version               int64              `json:"version"`
}
type MergeGate struct {
	Binding              PolicyEvidenceBinding `json:"binding"`
	ChangedLines         int64                 `json:"changed_lines"`
	Companions           []MergeCompanion      `json:"companions"`
	ConfigurationVersion int64                 `json:"configuration_version"`
	ConnectionId         string                `json:"connection_id"`
	ConnectionVersion    int64                 `json:"connection_version"`
	Decision             PolicyResult          `json:"decision"`
	ExpiresAt            time.Time             `json:"expires_at"`
	Id                   string                `json:"id"`
	Method               string                `json:"method"`
	Paths                []string              `json:"paths"`
	Phase                *string               `json:"phase,omitempty"`
	RepositoryId         string                `json:"repository_id"`
	Snapshot             MergeSnapshot         `json:"snapshot"`
}
type MergeOperation struct {
	CancelRequested bool              `json:"cancel_requested"`
	ChangeId        string            `json:"change_id"`
	CreatedAt       time.Time         `json:"created_at"`
	GateId          string            `json:"gate_id"`
	Id              string            `json:"id"`
	NativeQueueId   *string           `json:"native_queue_id,omitempty"`
	NativeResult    *ForgeMergeResult `json:"native_result,omitempty"`
	Reason          string            `json:"reason"`
	RepositoryId    string            `json:"repository_id"`
	RequestedGateId string            `json:"requested_gate_id"`
	State           string            `json:"state"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Version         int64             `json:"version"`
}
type MergeOperationPage struct {
	Complete   bool             `json:"complete"`
	Items      []MergeOperation `json:"items"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}
type MergePreviewRequest struct {
	Method string `json:"method"`
}
type MergeQualification struct {
	CiConfigSha256     *string   `json:"ci_config_sha256,omitempty"`
	ConnectionVersion  int64     `json:"connection_version"`
	EvidenceReference  string    `json:"evidence_reference"`
	EvidenceSha256     string    `json:"evidence_sha256"`
	ExactHead          bool      `json:"exact_head"`
	ExpiresAt          time.Time `json:"expires_at"`
	InspectorVersion   int64     `json:"inspector_version"`
	Provider           string    `json:"provider"`
	QueueExecutionGate bool      `json:"queue_execution_gate"`
	ServerVersion      string    `json:"server_version"`
	StrictTarget       bool      `json:"strict_target"`
	VerifiedAt         time.Time `json:"verified_at"`
}
type MergeRequestInput struct {
	GateId         string `json:"gate_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
type MergeSnapshot struct {
	Approvals      []ForgeApproval        `json:"approvals"`
	Capabilities   ForgeMergeCapabilities `json:"capabilities"`
	Change         ForgeChange            `json:"change"`
	Checks         []ForgeCheck           `json:"checks"`
	ExecutionCheck *struct {
		Name        string `json:"name"`
		PublisherId string `json:"publisher_id"`
	} `json:"execution_check,omitempty"`
	Native     ForgeNativeEligibility  `json:"native"`
	ObservedAt time.Time               `json:"observed_at"`
	Queue      ForgeQueueState         `json:"queue"`
	Rules      ForgeRules              `json:"rules"`
	TrainGate  *map[string]interface{} `json:"train_gate,omitempty"`
}
type Meta struct {
	BootstrapRequired *bool       `json:"bootstrap_required,omitempty"`
	Development       bool        `json:"development"`
	Edition           MetaEdition `json:"edition"`
	FixtureAuth       bool        `json:"fixture_auth"`
	Name              string      `json:"name"`
	Version           string      `json:"version"`
}
type MetaEdition string
type ModelMessage struct {
	Role       string           `json:"role"`
	Text       string           `json:"text"`
	ToolCallId *string          `json:"tool_call_id,omitempty"`
	ToolCalls  *[]ModelToolCall `json:"tool_calls,omitempty"`
}
type ModelTool struct {
	Description string      `json:"description"`
	Name        string      `json:"name"`
	Schema      interface{} `json:"schema"`
}
type ModelToolCall struct {
	Arguments interface{} `json:"arguments"`
	Id        string      `json:"id"`
	Name      string      `json:"name"`
}
type ModelTurn struct {
	Continuation    interface{}     `json:"continuation,omitempty"`
	MaxOutputTokens int64           `json:"max_output_tokens"`
	Messages        *[]ModelMessage `json:"messages"`
	Model           string          `json:"model"`
	OperationId     string          `json:"operation_id"`
	System          string          `json:"system"`
	TimeoutMs       int64           `json:"timeout_ms"`
	Tools           *[]ModelTool    `json:"tools"`
}
type ModelTurnResult struct {
	Continuation interface{}      `json:"continuation,omitempty"`
	FinishReason string           `json:"finish_reason"`
	ProviderId   string           `json:"provider_id"`
	Text         string           `json:"text"`
	ToolCalls    *[]ModelToolCall `json:"tool_calls"`
	Usage        ModelUsage       `json:"usage"`
}
type ModelUsage struct {
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheTokens         int64  `json:"cache_tokens"`
	InputTokens         int64  `json:"input_tokens"`
	Known               bool   `json:"known"`
	OutputTokens        int64  `json:"output_tokens"`
	Source              string `json:"source"`
}
type NativeDeliveryWorkflow struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Ref  string `json:"ref"`
	Url  string `json:"url"`
}
type NativeDeploymentGates struct {
	ApprovalUrl    string   `json:"approval_url"`
	Blockers       []string `json:"blockers"`
	Environment    string   `json:"environment"`
	NativeEnforced string   `json:"native_enforced"`
	RulesHash      string   `json:"rules_hash"`
	State          string   `json:"state"`
}
type NativeDeploymentStatus struct {
	ArtifactDigest string    `json:"artifact_digest"`
	CorrelationId  string    `json:"correlation_id"`
	CreatedAt      time.Time `json:"created_at"`
	Environment    string    `json:"environment"`
	Event          string    `json:"event"`
	Health         string    `json:"health"`
	Id             string    `json:"id"`
	ObservedAt     time.Time `json:"observed_at"`
	Ref            string    `json:"ref"`
	RunAttempt     int64     `json:"run_attempt"`
	SourceSha      string    `json:"source_sha"`
	State          string    `json:"state"`
	UpdatedAt      time.Time `json:"updated_at"`
	Url            string    `json:"url"`
	WorkflowId     string    `json:"workflow_id"`
	WorkflowPath   string    `json:"workflow_path"`
	WorkflowSha    string    `json:"workflow_sha"`
}
type Organisation struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Paused  bool   `json:"paused"`
	Version int64  `json:"version"`
}
type Pause struct {
	Id      string `json:"id"`
	Kind    string `json:"kind"`
	Paused  bool   `json:"paused"`
	Version int64  `json:"version"`
}
type PauseInput struct {
	Paused bool `json:"paused"`
}
type PinnedSnapshot struct {
	CommitSha      string        `json:"commit_sha"`
	Complete       bool          `json:"complete"`
	Files          *[]SourceFile `json:"files"`
	ManifestSha256 string        `json:"manifest_sha256"`
}
type PolicyActivateRequest struct {
	PrimaryTeamId  *string `json:"primary_team_id,omitempty"`
	Reason         string  `json:"reason"`
	RepositoryId   *string `json:"repository_id,omitempty"`
	SimulationHash string  `json:"simulation_hash"`
}
type PolicyBindingVersion struct {
	Version int64 `json:"version"`
}
type PolicyDefaults struct {
	BranchPrefix *string `json:"branch_prefix,omitempty"`
	Model        *string `json:"model,omitempty"`
	Route        *string `json:"route,omitempty"`
}
type PolicyDocument struct {
	Allow                 *PolicyLists         `json:"allow,omitempty"`
	Defaults              *PolicyDefaults      `json:"defaults,omitempty"`
	Deny                  *[]string            `json:"deny,omitempty"`
	ForbiddenPaths        *[]string            `json:"forbidden_paths,omitempty"`
	Limits                *PolicyLimits        `json:"limits,omitempty"`
	MaxEvidenceAgeSeconds *int64               `json:"max_evidence_age_seconds,omitempty"`
	Paused                *bool                `json:"paused,omitempty"`
	Required              *[]PolicyRequirement `json:"required,omitempty"`
	Schema                PolicyDocumentSchema `json:"schema"`
}
type PolicyDocumentSchema string
type PolicyEvidence struct {
	Approvals  *int                  `json:"approvals,omitempty"`
	Binding    PolicyEvidenceBinding `json:"binding"`
	Id         string                `json:"id"`
	Identity   *string               `json:"identity,omitempty"`
	ObservedAt time.Time             `json:"observed_at"`
	Reference  string                `json:"reference"`
	State      string                `json:"state"`
}
type PolicyEvidenceBinding struct {
	Artifact          *string `json:"artifact,omitempty"`
	CapabilityVersion *string `json:"capability_version,omitempty"`
	Head              *string `json:"head,omitempty"`
	PolicyHash        *string `json:"policy_hash,omitempty"`
	ProviderRules     *string `json:"provider_rules,omitempty"`
	SourceSha         *string `json:"source_sha,omitempty"`
	Target            *string `json:"target,omitempty"`
	Tested            *string `json:"tested,omitempty"`
}
type PolicyInput struct {
	Action             string                 `json:"action"`
	Current            *PolicyEvidenceBinding `json:"current,omitempty"`
	Environment        *string                `json:"environment,omitempty"`
	Evidence           *[]PolicyEvidence      `json:"evidence,omitempty"`
	MergeMethod        *string                `json:"merge_method,omitempty"`
	Model              *string                `json:"model,omitempty"`
	Now                *time.Time             `json:"now,omitempty"`
	Paths              *[]string              `json:"paths,omitempty"`
	PausedScopes       *[]string              `json:"paused_scopes,omitempty"`
	Recipe             *string                `json:"recipe,omitempty"`
	Route              *string                `json:"route,omitempty"`
	Stage              *PolicyInputStage      `json:"stage,omitempty"`
	StartingPolicyHash *string                `json:"starting_policy_hash,omitempty"`
	Usage              *PolicyLimits          `json:"usage,omitempty"`
	Workflow           *string                `json:"workflow,omitempty"`
}
type PolicyInputStage string
type PolicyLayer struct {
	BindingVersion int64          `json:"binding_version"`
	Policy         PolicyDocument `json:"policy"`
	Scope          PolicyScope    `json:"scope"`
	VersionId      string         `json:"version_id"`
}
type PolicyLimits struct {
	Attempts     *int64 `json:"attempts,omitempty"`
	Budget       *int64 `json:"budget,omitempty"`
	ChangedFiles *int64 `json:"changed_files,omitempty"`
	ChangedLines *int64 `json:"changed_lines,omitempty"`
	Concurrency  *int64 `json:"concurrency,omitempty"`
	OpenChanges  *int64 `json:"open_changes,omitempty"`
}
type PolicyLists struct {
	Environments *[]string `json:"environments,omitempty"`
	MergeMethods *[]string `json:"merge_methods,omitempty"`
	Models       *[]string `json:"models,omitempty"`
	Recipes      *[]string `json:"recipes,omitempty"`
	Routes       *[]string `json:"routes,omitempty"`
	Workflows    *[]string `json:"workflows,omitempty"`
}
type PolicyRequirement struct {
	Actions   []string `json:"actions"`
	Approvals *int     `json:"approvals,omitempty"`
	Id        string   `json:"id"`
	Identity  *string  `json:"identity,omitempty"`
}
type PolicyResult struct {
	Bindings           []PolicyLayer       `json:"bindings"`
	Blockers           []string            `json:"blockers"`
	EvidenceReferences []string            `json:"evidence_references"`
	Outcome            PolicyResultOutcome `json:"outcome"`
	PolicyHash         string              `json:"policy_hash"`
	RequiredActions    []string            `json:"required_actions"`
	Rules              []string            `json:"rules"`
	StartingPolicyHash string              `json:"starting_policy_hash"`
}
type PolicyResultOutcome string
type PolicyScope struct {
	Id   string          `json:"id"`
	Kind PolicyScopeKind `json:"kind"`
}
type PolicyScopeKind string
type PolicySimulateRequest struct {
	Input         PolicyInput `json:"input"`
	PrimaryTeamId *string     `json:"primary_team_id,omitempty"`
	RepositoryId  *string     `json:"repository_id,omitempty"`
}
type PolicySimulation struct {
	Decision PolicyResult   `json:"decision"`
	Hash     string         `json:"hash"`
	Resolved ResolvedPolicy `json:"resolved"`
}
type PolicyVersion struct {
	ActorId   string         `json:"actor_id"`
	CreatedAt time.Time      `json:"created_at"`
	Hash      string         `json:"hash"`
	Id        string         `json:"id"`
	Policy    PolicyDocument `json:"policy"`
	Reason    string         `json:"reason"`
	Scope     PolicyScope    `json:"scope"`
}
type PolicyVersionCreate struct {
	Policy PolicyDocument `json:"policy"`
	Reason string         `json:"reason"`
	Scope  PolicyScope    `json:"scope"`
}
type PolicyVersionPage struct {
	Complete   bool            `json:"complete"`
	Items      []PolicyVersion `json:"items"`
	NextCursor *string         `json:"next_cursor,omitempty"`
}
type PrivateCompletion struct {
	GrantId string                 `json:"grant_id"`
	Result  map[string]interface{} `json:"result"`
}
type PrivateGrant struct {
	AuthorityId      string                 `json:"authority_id"`
	Connection       map[string]interface{} `json:"connection"`
	ExpiresAt        time.Time              `json:"expires_at"`
	Id               string                 `json:"id"`
	Operation        PrivateOperation       `json:"operation"`
	ResultCapability string                 `json:"result_capability"`
	RunnerVersion    *int64                 `json:"runner_version,omitempty"`
	Secret           string                 `json:"secret"`
	Target           struct {
		OrgId    string `json:"org_id"`
		RunnerId string `json:"runner_id"`
	} `json:"target"`
	TimeoutMs int `json:"timeout_ms"`
}
type PrivateOperation struct {
	Change     *map[string]interface{} `json:"change,omitempty"`
	Changes    *map[string]interface{} `json:"changes,omitempty"`
	Checks     *map[string]interface{} `json:"checks,omitempty"`
	File       *map[string]interface{} `json:"file,omitempty"`
	Id         string                  `json:"id"`
	Inventory  *map[string]interface{} `json:"inventory,omitempty"`
	Kind       PrivateOperationKind    `json:"kind"`
	Ref        *map[string]interface{} `json:"ref,omitempty"`
	Repository *map[string]interface{} `json:"repository,omitempty"`
	Source     *map[string]interface{} `json:"source,omitempty"`
}
type PrivateOperationKind string
type PrivateRoute struct {
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	Cidrs      []string   `json:"cidrs"`
	Host       string     `json:"host"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RunnerId   string     `json:"runner_id"`
}
type PrivateRouteChange struct {
	Route PrivateRoute `json:"route"`
}
type RepairCheck struct {
	Cases        map[string]string `json:"cases"`
	CommandId    string            `json:"command_id"`
	Complete     bool              `json:"complete"`
	Excerpt      *string           `json:"excerpt,omitempty"`
	ExitCode     int64             `json:"exit_code"`
	OutputSha256 string            `json:"output_sha256"`
	Reason       string            `json:"reason"`
}
type RepairCommand struct {
	Args           *[]string `json:"args"`
	Directory      string    `json:"directory"`
	Id             string    `json:"id"`
	ReportFormat   string    `json:"report_format"`
	TimeoutSeconds int64     `json:"timeout_seconds"`
}
type RepairExecution struct {
	BaselineRepository ForgeRepoRef `json:"baseline_repository"`
	ConnectionId       string       `json:"connection_id"`
	ConnectionVersion  int64        `json:"connection_version"`
	Finding            Finding      `json:"finding"`
	MaxAttempts        int64        `json:"max_attempts"`
	MaxOutputTokens    int64        `json:"max_output_tokens"`
	Model              string       `json:"model"`
	NativeHeadSha      *string      `json:"native_head_sha,omitempty"`
	Plan               RepairPlan   `json:"plan"`
	PolicyHash         string       `json:"policy_hash"`
	Repository         ForgeRepoRef `json:"repository"`
	Request            RepairInput  `json:"request"`
	TurnTimeoutMs      int64        `json:"turn_timeout_ms"`
}
type RepairInput struct {
	FindingId         string  `json:"finding_id"`
	FindingVersion    int64   `json:"finding_version"`
	IdempotencyKey    *string `json:"idempotency_key,omitempty"`
	ModelConnectionId string  `json:"model_connection_id"`
	ModelRoute        string  `json:"model_route"`
	PlanDigest        *string `json:"plan_digest,omitempty"`
	Recipe            string  `json:"recipe"`
	RunnerPoolId      string  `json:"runner_pool_id"`
}
type RepairPatch struct {
	Content []byte `json:"content"`
	Delete  *bool  `json:"delete,omitempty"`
	Path    string `json:"path"`
}
type RepairPlan struct {
	AuthorityHash   *string           `json:"authority_hash,omitempty"`
	BaselineSha     string            `json:"baseline_sha"`
	Digest          string            `json:"digest"`
	ForbiddenPaths  *[]string         `json:"forbidden_paths"`
	Image           string            `json:"image"`
	MaxChangedLines int64             `json:"max_changed_lines"`
	ProtectedHashes map[string]string `json:"protected_hashes"`
	Recipe          RepairRecipe      `json:"recipe"`
	TargetSha       string            `json:"target_sha"`
	Version         int64             `json:"version"`
}
type RepairPreview struct {
	Blockers  *[]string       `json:"blockers"`
	Context   RepairExecution `json:"context"`
	ExpiresAt time.Time       `json:"expires_at"`
}
type RepairPublication struct {
	ArtifactIds *[]string      `json:"artifact_ids"`
	Checks      *[]RepairCheck `json:"checks"`
	HeadSha     string         `json:"head_sha"`
	PlanDigest  string         `json:"plan_digest"`
}
type RepairRecipe struct {
	Commands       *[]RepairCommand `json:"commands"`
	ManifestPaths  *[]string        `json:"manifest_paths"`
	MaxFiles       int64            `json:"max_files"`
	MaxPatchBytes  int64            `json:"max_patch_bytes"`
	MaxTurns       int64            `json:"max_turns"`
	MinimumTests   int64            `json:"minimum_tests"`
	Name           string           `json:"name"`
	ProtectedPaths *[]string        `json:"protected_paths"`
	TimeoutSeconds int64            `json:"timeout_seconds"`
	Version        string           `json:"version"`
}
type RepairReport struct {
	Artifacts  *[]string      `json:"artifacts"`
	Baseline   *[]RepairCheck `json:"baseline"`
	Candidate  *[]RepairCheck `json:"candidate"`
	Diff       *string        `json:"diff,omitempty"`
	Patches    *[]RepairPatch `json:"patches"`
	PlanDigest string         `json:"plan_digest"`
	Reason     string         `json:"reason"`
	State      string         `json:"state"`
	Target     *[]RepairCheck `json:"target"`
	Turns      int64          `json:"turns"`
}
type RepairRun struct {
	Branch             string          `json:"branch"`
	CandidateArtifacts *[]string       `json:"candidate_artifacts"`
	CandidateChecks    *[]RepairCheck  `json:"candidate_checks"`
	CandidateSha       string          `json:"candidate_sha"`
	Change             *ForgeChange    `json:"change,omitempty"`
	Context            RepairExecution `json:"context"`
	Report             *RepairReport   `json:"report,omitempty"`
	State              string          `json:"state"`
	Task               Task            `json:"task"`
	UpdatedAt          time.Time       `json:"updated_at"`
	Version            int64           `json:"version"`
}
type Repository struct {
	Accessible        bool               `json:"accessible"`
	Archived          bool               `json:"archived"`
	ChangesObservedAt *time.Time         `json:"changes_observed_at,omitempty"`
	ConnectionId      string             `json:"connection_id"`
	ConnectionVersion *int64             `json:"connection_version,omitempty"`
	DefaultBranch     string             `json:"default_branch"`
	Id                string             `json:"id"`
	LastSyncedAt      *time.Time         `json:"last_synced_at"`
	Name              string             `json:"name"`
	NativeId          string             `json:"native_id"`
	OrgId             string             `json:"org_id"`
	Paused            bool               `json:"paused"`
	Provider          RepositoryProvider `json:"provider"`
	SyncReason        *string            `json:"sync_reason,omitempty"`
	SyncState         *string            `json:"sync_state,omitempty"`
	TeamIds           []string           `json:"team_ids"`
	Url               string             `json:"url"`
	Version           int64              `json:"version"`
}
type RepositoryProvider string
type RepositoryPage struct {
	Complete   bool         `json:"complete"`
	Items      []Repository `json:"items"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
type ResolvedPolicy struct {
	Hash            string         `json:"hash"`
	Layers          []PolicyLayer  `json:"layers"`
	MissingDefaults []string       `json:"missing_defaults"`
	Paused          bool           `json:"paused"`
	Policy          PolicyDocument `json:"policy"`
	PrimaryTeamId   *string        `json:"primary_team_id,omitempty"`
	Problems        []string       `json:"problems"`
	RepositoryId    string         `json:"repository_id"`
	ScopePaused     bool           `json:"scope_paused"`
}
type Role string
type Runner struct {
	CredentialExpiresAt time.Time `json:"credential_expires_at"`
	Id                  string    `json:"id"`
	Name                string    `json:"name"`
	OrgId               string    `json:"org_id"`
	PoolId              string    `json:"pool_id"`
	State               string    `json:"state"`
	Version             int64     `json:"version"`
}
type RunnerAssignment struct {
	ExpiresAt time.Time     `json:"expires_at"`
	Lease     WorkflowLease `json:"lease"`
	Task      Task          `json:"task"`
	Token     string        `json:"token"`
}
type RunnerCompletion struct {
	Outcome   RunnerCompletionOutcome `json:"outcome"`
	Retryable *bool                   `json:"retryable,omitempty"`
}
type RunnerCompletionOutcome string
type RunnerCredential struct {
	ExpiresAt time.Time `json:"expires_at"`
	Runner    Runner    `json:"runner"`
	Token     string    `json:"token"`
}
type RunnerHeartbeat struct {
	Lease  WorkflowLease `json:"lease"`
	Reason *string       `json:"reason,omitempty"`
	Stop   bool          `json:"stop"`
}
type RunnerPage struct {
	Complete   bool     `json:"complete"`
	Items      []Runner `json:"items"`
	NextCursor *string  `json:"next_cursor,omitempty"`
}
type RunnerPool struct {
	Id            string          `json:"id"`
	Name          string          `json:"name"`
	OrgId         string          `json:"org_id"`
	RepositoryIds []string        `json:"repository_ids"`
	State         RunnerPoolState `json:"state"`
	Version       int64           `json:"version"`
}
type RunnerPoolState string
type RunnerPoolInput struct {
	Name          string                `json:"name"`
	RepositoryIds []string              `json:"repository_ids"`
	State         *RunnerPoolInputState `json:"state,omitempty"`
}
type RunnerPoolInputState string
type RunnerPoolPage struct {
	Complete   bool         `json:"complete"`
	Items      []RunnerPool `json:"items"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
type Session struct {
	CsrfToken     string         `json:"csrf_token"`
	Memberships   []Membership   `json:"memberships"`
	Organisations []Organisation `json:"organisations"`
	User          struct {
		Email string `json:"email"`
		Id    string `json:"id"`
		Name  string `json:"name"`
	} `json:"user"`
}
type SignedArtifactProvenance struct {
	Document  ArtifactProvenance `json:"document"`
	Signature string             `json:"signature"`
}
type SourceFile struct {
	Content    []byte `json:"content"`
	Delete     *bool  `json:"delete,omitempty"`
	Executable *bool  `json:"executable,omitempty"`
	Path       string `json:"path"`
}
type Task struct {
	CampaignId            *string   `json:"campaign_id,omitempty"`
	CancelVersion         int64     `json:"cancel_version"`
	CancellationRequested bool      `json:"cancellation_requested"`
	CreatedAt             time.Time `json:"created_at"`
	Id                    string    `json:"id"`
	MaxAttempts           int64     `json:"max_attempts"`
	ModelConnectionId     *string   `json:"model_connection_id,omitempty"`
	ModelRoute            string    `json:"model_route"`
	OperationId           string    `json:"operation_id"`
	OrgId                 string    `json:"org_id"`
	PolicyHash            string    `json:"policy_hash"`
	Reason                string    `json:"reason"`
	Recipe                string    `json:"recipe"`
	RecipeVersion         string    `json:"recipe_version"`
	RepositoryId          string    `json:"repository_id"`
	RunnerPoolId          *string   `json:"runner_pool_id,omitempty"`
	StartingPolicyHash    string    `json:"starting_policy_hash"`
	State                 TaskState `json:"state"`
	TargetBranch          string    `json:"target_branch"`
	Version               int64     `json:"version"`
}
type TaskCreate struct {
	CampaignId        *string `json:"campaign_id,omitempty"`
	IdempotencyKey    string  `json:"idempotency_key"`
	MaxAttempts       *int64  `json:"max_attempts,omitempty"`
	ModelConnectionId *string `json:"model_connection_id,omitempty"`
	ModelRoute        *string `json:"model_route,omitempty"`
	PolicyHash        *string `json:"policy_hash,omitempty"`
	Priority          *int64  `json:"priority,omitempty"`
	Recipe            string  `json:"recipe"`
	RecipeVersion     string  `json:"recipe_version"`
	RepositoryId      string  `json:"repository_id"`
	RunnerPoolId      *string `json:"runner_pool_id,omitempty"`
	TargetBranch      string  `json:"target_branch"`
}
type TaskPage struct {
	Complete   bool    `json:"complete"`
	Items      []Task  `json:"items"`
	NextCursor *string `json:"next_cursor,omitempty"`
}
type TaskState string
type Team struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	RepositoryIds []string `json:"repository_ids"`
	Version       int64    `json:"version"`
}
type TeamInput struct {
	Name          string   `json:"name"`
	RepositoryIds []string `json:"repository_ids"`
}
type TeamPage struct {
	Complete   bool    `json:"complete"`
	Items      []Team  `json:"items"`
	NextCursor *string `json:"next_cursor,omitempty"`
}
type UsageEntry struct {
	Provider       string            `json:"provider"`
	Recipe         string            `json:"recipe"`
	RepositoryName string            `json:"repository_name"`
	Reservation    BudgetReservation `json:"reservation"`
}
type UsagePage struct {
	Complete   bool         `json:"complete"`
	Items      []UsageEntry `json:"items"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
type UsageSummary struct {
	Cancelled             int64        `json:"cancelled"`
	Dispatched            int64        `json:"dispatched"`
	EstimatedCostMicroUsd int64        `json:"estimated_cost_micro_usd"`
	Held                  BudgetAmount `json:"held"`
	KnownTokens           int64        `json:"known_tokens"`
	Records               int64        `json:"records"`
	Reserved              int64        `json:"reserved"`
	Settled               int64        `json:"settled"`
	Unknown               int64        `json:"unknown"`
	UnknownMaximum        BudgetAmount `json:"unknown_maximum"`
}
type WorkflowLease struct {
	AttemptId    string    `json:"attempt_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	Fence        int64     `json:"fence"`
	JobId        string    `json:"job_id"`
	OperationId  string    `json:"operation_id"`
	OrgId        string    `json:"org_id"`
	PolicyHash   string    `json:"policy_hash"`
	RepositoryId string    `json:"repository_id"`
	TaskId       string    `json:"task_id"`
	WorkerId     string    `json:"worker_id"`
}
type SubmitDeploymentHealthParams struct {
	XReforgeSignature string `json:"X-Reforge-Signature"`
}
type SubmitGitOpsHealthParams struct {
	XReforgeSignature string `json:"X-Reforge-Signature"`
}
type ListAuditEventsParams struct {
	RepositoryId *string    `form:"repository_id,omitempty" json:"repository_id,omitempty"`
	ActorId      *string    `form:"actor_id,omitempty" json:"actor_id,omitempty"`
	Action       *string    `form:"action,omitempty" json:"action,omitempty"`
	Since        *time.Time `form:"since,omitempty" json:"since,omitempty"`
	Until        *time.Time `form:"until,omitempty" json:"until,omitempty"`
	Cursor       *string    `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit        *int       `form:"limit,omitempty" json:"limit,omitempty"`
}
type ExportAuditPageParams struct {
	RepositoryId *string    `form:"repository_id,omitempty" json:"repository_id,omitempty"`
	ActorId      *string    `form:"actor_id,omitempty" json:"actor_id,omitempty"`
	Action       *string    `form:"action,omitempty" json:"action,omitempty"`
	Since        *time.Time `form:"since,omitempty" json:"since,omitempty"`
	Until        *time.Time `form:"until,omitempty" json:"until,omitempty"`
	Cursor       *string    `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit        *int       `form:"limit,omitempty" json:"limit,omitempty"`
}
type GetBudgetRouteParams struct {
	Model *string `form:"model,omitempty" json:"model,omitempty"`
	Route *string `form:"route,omitempty" json:"route,omitempty"`
}
type PutBudgetRouteParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type PutBudgetParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type PreviewCampaignParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListCampaignsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
	State  *string `form:"state,omitempty" json:"state,omitempty"`
}
type CreateCampaignParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type CancelCampaignParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListCampaignMembersParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type PauseCampaignParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ResumeCampaignParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type StartCampaignParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListConnectionsParams struct {
	Kind   *string `form:"kind,omitempty" json:"kind,omitempty"`
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type CreateConnectionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListAgentsConnectionsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type CreateAgentsConnectionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListDeliveryConnectionsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type CreateDeliveryConnectionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListForgesConnectionsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type CreateForgesConnectionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListModelsConnectionsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type CreateModelsConnectionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type RevokeConnectionParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type SetPrivateRouteParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type RewrapCredentialParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type RotateCredentialParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type TestConnectionParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type RevokeInventoryWebhookParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type RotateInventoryWebhookParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type PutDeploymentConfigurationParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type PreviewDeploymentParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type TrackDeploymentParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListDeploymentsParams struct {
	RepositoryId *string `form:"repository_id,omitempty" json:"repository_id,omitempty"`
	Environment  *string `form:"environment,omitempty" json:"environment,omitempty"`
	Cursor       *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit        *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type RequestDeploymentParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type CancelDeploymentParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ObserveDeploymentParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type StreamEventsParams struct {
	After       *int64  `form:"after,omitempty" json:"after,omitempty"`
	LastEventID *string `json:"Last-Event-ID,omitempty"`
}
type ReplayEventsParams struct {
	After *int64 `form:"after,omitempty" json:"after,omitempty"`
}
type ListFindingsParams struct {
	Limit        *int    `form:"limit,omitempty" json:"limit,omitempty"`
	Cursor       *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Q            *string `form:"q,omitempty" json:"q,omitempty"`
	RepositoryId *string `form:"repository_id,omitempty" json:"repository_id,omitempty"`
	State        *string `form:"state,omitempty" json:"state,omitempty"`
	Category     *string `form:"category,omitempty" json:"category,omitempty"`
	Severity     *string `form:"severity,omitempty" json:"severity,omitempty"`
}
type UpdateFindingParams struct {
	IfMatch string `json:"If-Match"`
}
type PutGitOpsConfigurationParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type PreviewGitOpsPromotionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListGitOpsPromotionsParams struct {
	Environment *string `form:"environment,omitempty" json:"environment,omitempty"`
	Cursor      *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit       *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type RequestGitOpsPromotionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type CancelGitOpsPromotionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ContinueGitOpsPromotionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type RequestGitOpsMergeParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type PreviewGitOpsMergeParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ObserveGitOpsPromotionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListInventoryJobsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type StartInventorySyncParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type CancelInventoryJobParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListInventoryCandidatesParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type ImportInventoryCandidatesParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListMembershipsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type DeleteMembershipParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type PutMembershipParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListMergeOperationsParams struct {
	RepositoryId string  `form:"repository_id" json:"repository_id"`
	ChangeId     *string `form:"change_id,omitempty" json:"change_id,omitempty"`
	Cursor       *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit        *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type RequestProtectedMergeParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type CancelMergeOperationParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ReconcileMergeOperationParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type SetPauseParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type GetEffectivePolicyParams struct {
	RepositoryId *string `form:"repository_id,omitempty" json:"repository_id,omitempty"`
}
type ListPolicyVersionsParams struct {
	ScopeKind *string `form:"scope_kind,omitempty" json:"scope_kind,omitempty"`
	ScopeId   *string `form:"scope_id,omitempty" json:"scope_id,omitempty"`
	Cursor    *string `form:"cursor,omitempty" json:"cursor,omitempty"`
}
type CreatePolicyVersionParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ActivatePolicyParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type SimulatePolicyParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type PreviewRepairParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type EnqueueRepairParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ReconcileRepairParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type RepositoriesParams struct {
	Cursor   *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit    *int    `form:"limit,omitempty" json:"limit,omitempty"`
	Q        *string `form:"q,omitempty" json:"q,omitempty"`
	Provider *string `form:"provider,omitempty" json:"provider,omitempty"`
	TeamId   *string `form:"team_id,omitempty" json:"team_id,omitempty"`
	Status   *string `form:"status,omitempty" json:"status,omitempty"`
}
type ListInventoryChangesParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type PreviewProtectedMergeParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListBotRevalidationsParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type PutMaintenanceConfigParams struct {
	IfMatch string `json:"If-Match"`
}
type PutMergeConfigurationParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListRunnerPoolsParams struct {
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
}
type CreateRunnerPoolParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type UpdateRunnerPoolParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type CreateRunnerEnrollmentParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListRunnersParams struct {
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
}
type RevokeRunnerParams struct {
	IfMatch    string `json:"If-Match"`
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ListTasksParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int64  `form:"limit,omitempty" json:"limit,omitempty"`
}
type EnqueueTaskParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type CancelTaskParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ResumeTaskParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListTeamsParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
}
type DeleteTeamParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type PutTeamParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
	IfMatch    string `json:"If-Match"`
}
type ListUsageParams struct {
	RepositoryId *string    `form:"repository_id,omitempty" json:"repository_id,omitempty"`
	TeamId       *string    `form:"team_id,omitempty" json:"team_id,omitempty"`
	Recipe       *string    `form:"recipe,omitempty" json:"recipe,omitempty"`
	Provider     *string    `form:"provider,omitempty" json:"provider,omitempty"`
	ConnectionId *string    `form:"connection_id,omitempty" json:"connection_id,omitempty"`
	State        *string    `form:"state,omitempty" json:"state,omitempty"`
	Since        *time.Time `form:"since,omitempty" json:"since,omitempty"`
	Until        *time.Time `form:"until,omitempty" json:"until,omitempty"`
	Cursor       *string    `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit        *int       `form:"limit,omitempty" json:"limit,omitempty"`
}
type SummarizeUsageParams struct {
	RepositoryId *string    `form:"repository_id,omitempty" json:"repository_id,omitempty"`
	TeamId       *string    `form:"team_id,omitempty" json:"team_id,omitempty"`
	Recipe       *string    `form:"recipe,omitempty" json:"recipe,omitempty"`
	Provider     *string    `form:"provider,omitempty" json:"provider,omitempty"`
	ConnectionId *string    `form:"connection_id,omitempty" json:"connection_id,omitempty"`
	State        *string    `form:"state,omitempty" json:"state,omitempty"`
	Since        *time.Time `form:"since,omitempty" json:"since,omitempty"`
	Until        *time.Time `form:"until,omitempty" json:"until,omitempty"`
}
type RevokeSessionsParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type BootstrapParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type OidcCallbackParams struct {
	State string `form:"state" json:"state"`
	Code  string `form:"code" json:"code"`
}
type LogoutParams struct {
	XCSRFToken string `json:"X-CSRF-Token"`
}
type ReceiveForgeWebhookJSONBody map[string]interface{}
type UploadRunnerArtifactJSONBody = []byte
type UploadRunnerArtifactTextBody = []byte
type UploadRunnerArtifactParams struct {
	XArtifactName string `json:"X-Artifact-Name"`
}
type EnrollRunnerJSONBody struct {
	Name string `json:"name"`
}
type InvokeRunnerOperationJSONBody struct {
	ConnectionId string                 `json:"connection_id"`
	Input        map[string]interface{} `json:"input"`
}
type PollPrivateGrantJSONBody = map[string]interface{}
type CompletePrivateGrantParams struct {
	XPrivateResultCapability string `json:"X-Private-Result-Capability"`
}
type RunnerProgressJSONBody struct {
	State TaskState `json:"state"`
}
type SubmitDeploymentHealthJSONRequestBody = DeploymentHealth
type SubmitGitOpsHealthJSONRequestBody = GitOpsHealth
type PutBudgetRouteJSONRequestBody = BudgetRouteInput
type PutBudgetJSONRequestBody = BudgetLimitInput
type PreviewCampaignJSONRequestBody = CampaignInput
type CreateCampaignJSONRequestBody = CampaignCreateInput
type CancelCampaignJSONRequestBody = CampaignControlInput
type PauseCampaignJSONRequestBody = CampaignControlInput
type ResumeCampaignJSONRequestBody = CampaignControlInput
type StartCampaignJSONRequestBody = CampaignControlInput
type CreateConnectionJSONRequestBody = ConnectionCreate
type CreateAgentsConnectionJSONRequestBody = ConnectionCreate
type CreateDeliveryConnectionJSONRequestBody = ConnectionCreate
type CreateForgesConnectionJSONRequestBody = ConnectionCreate
type CreateModelsConnectionJSONRequestBody = ConnectionCreate
type SetPrivateRouteJSONRequestBody = PrivateRouteChange
type RotateCredentialJSONRequestBody = CredentialRotation
type PutDeploymentConfigurationJSONRequestBody = DeploymentConfiguration
type PreviewDeploymentJSONRequestBody = DeploymentPreviewInput
type TrackDeploymentJSONRequestBody = DeploymentTrackInput
type RequestDeploymentJSONRequestBody = DeploymentRequestInput
type ImportAdvisoryJSONRequestBody = AdvisoryInput
type UpdateFindingJSONRequestBody = FindingUpdate
type PutGitOpsConfigurationJSONRequestBody = GitOpsConfiguration
type PreviewGitOpsPromotionJSONRequestBody = GitOpsPreviewInput
type RequestGitOpsPromotionJSONRequestBody = DeploymentRequestInput
type RequestGitOpsMergeJSONRequestBody = MergeRequestInput
type PreviewGitOpsMergeJSONRequestBody = MergePreviewRequest
type StartInventorySyncJSONRequestBody = InventorySyncInput
type ImportInventoryCandidatesJSONRequestBody = InventoryImportInput
type PutMembershipJSONRequestBody = MembershipInput
type RequestProtectedMergeJSONRequestBody = MergeRequestInput
type SetPauseJSONRequestBody = PauseInput
type CreatePolicyVersionJSONRequestBody = PolicyVersionCreate
type ActivatePolicyJSONRequestBody = PolicyActivateRequest
type SimulatePolicyJSONRequestBody = PolicySimulateRequest
type PreviewRepairJSONRequestBody = RepairInput
type EnqueueRepairJSONRequestBody = RepairInput
type PreviewProtectedMergeJSONRequestBody = MergePreviewRequest
type PutMaintenanceConfigJSONRequestBody = MaintenanceConfig
type PutMergeConfigurationJSONRequestBody = MergeConfiguration
type CreateRunnerPoolJSONRequestBody = RunnerPoolInput
type UpdateRunnerPoolJSONRequestBody = RunnerPoolInput
type EnqueueTaskJSONRequestBody = TaskCreate
type PutTeamJSONRequestBody = TeamInput
type BootstrapJSONRequestBody = BootstrapRequest
type ReceiveForgeWebhookJSONRequestBody ReceiveForgeWebhookJSONBody
type UploadRunnerArtifactJSONRequestBody = UploadRunnerArtifactJSONBody
type UploadRunnerArtifactTextRequestBody = UploadRunnerArtifactTextBody
type EnrollRunnerJSONRequestBody EnrollRunnerJSONBody
type RunnerModelTurnJSONRequestBody = ModelTurn
type InvokeRunnerOperationJSONRequestBody InvokeRunnerOperationJSONBody
type PollPrivateGrantJSONRequestBody = PollPrivateGrantJSONBody
type CompletePrivateGrantJSONRequestBody = PrivateCompletion
type RunnerProgressJSONRequestBody RunnerProgressJSONBody
type RunnerRecordNativeChecksJSONRequestBody = RepairPublication
type RunnerRepairPublishJSONRequestBody = RepairPublication
type RunnerRepairReportJSONRequestBody = RepairReport
type RunnerResultJSONRequestBody = RunnerCompletion
