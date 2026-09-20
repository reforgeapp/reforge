package generated

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

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
	GiteaApprovals  PrivateOperationKind = "gitea.approvals"
	GiteaChecks     PrivateOperationKind = "gitea.checks"
	GiteaInventory  PrivateOperationKind = "gitea.inventory"
	GiteaProbe      PrivateOperationKind = "gitea.probe"
	GiteaReadChange PrivateOperationKind = "gitea.read_change"
	GiteaReadFile   PrivateOperationKind = "gitea.read_file"
	GiteaRepository PrivateOperationKind = "gitea.repository"
	GiteaResolveRef PrivateOperationKind = "gitea.resolve_ref"
)

func (e PrivateOperationKind) Valid() bool {
	switch e {
	case GiteaApprovals:
		return true
	case GiteaChecks:
		return true
	case GiteaInventory:
		return true
	case GiteaProbe:
		return true
	case GiteaReadChange:
		return true
	case GiteaReadFile:
		return true
	case GiteaRepository:
		return true
	case GiteaResolveRef:
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
type ArtifactMetadata struct {
	CreatedAt    time.Time          `json:"created_at"`
	ExpiresAt    time.Time          `json:"expires_at"`
	Id           openapi_types.UUID `json:"id"`
	MediaType    string             `json:"media_type"`
	Name         string             `json:"name"`
	OrgId        openapi_types.UUID `json:"org_id"`
	RepositoryId openapi_types.UUID `json:"repository_id"`
	Sha256       string             `json:"sha256"`
	Size         int64              `json:"size"`
	TaskId       openapi_types.UUID `json:"task_id"`
}
type BootstrapRequest struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}
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
type DeploymentState string
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
type Health struct {
	Status string `json:"status"`
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
type Meta struct {
	BootstrapRequired *bool       `json:"bootstrap_required,omitempty"`
	Development       bool        `json:"development"`
	Edition           MetaEdition `json:"edition"`
	FixtureAuth       bool        `json:"fixture_auth"`
	Name              string      `json:"name"`
	Version           string      `json:"version"`
}
type MetaEdition string
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
	StartingPolicyHash *string                `json:"starting_policy_hash,omitempty"`
	Usage              *PolicyLimits          `json:"usage,omitempty"`
	Workflow           *string                `json:"workflow,omitempty"`
}
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
	GrantId openapi_types.UUID     `json:"grant_id"`
	Result  map[string]interface{} `json:"result"`
}
type PrivateGrant struct {
	AuthorityId      openapi_types.UUID     `json:"authority_id"`
	Connection       map[string]interface{} `json:"connection"`
	ExpiresAt        time.Time              `json:"expires_at"`
	Id               openapi_types.UUID     `json:"id"`
	Operation        PrivateOperation       `json:"operation"`
	ResultCapability string                 `json:"result_capability"`
	RunnerVersion    *int64                 `json:"runner_version,omitempty"`
	Secret           string                 `json:"secret"`
	Target           struct {
		OrgId    openapi_types.UUID `json:"org_id"`
		RunnerId openapi_types.UUID `json:"runner_id"`
	} `json:"target"`
	TimeoutMs int `json:"timeout_ms"`
}
type PrivateOperation struct {
	Change     *map[string]interface{} `json:"change,omitempty"`
	Checks     *map[string]interface{} `json:"checks,omitempty"`
	File       *map[string]interface{} `json:"file,omitempty"`
	Id         openapi_types.UUID      `json:"id"`
	Inventory  *map[string]interface{} `json:"inventory,omitempty"`
	Kind       PrivateOperationKind    `json:"kind"`
	Ref        *map[string]interface{} `json:"ref,omitempty"`
	Repository *map[string]interface{} `json:"repository,omitempty"`
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
type Repository struct {
	Accessible    bool               `json:"accessible"`
	Archived      bool               `json:"archived"`
	ConnectionId  string             `json:"connection_id"`
	DefaultBranch string             `json:"default_branch"`
	Id            string             `json:"id"`
	LastSyncedAt  *time.Time         `json:"last_synced_at"`
	Name          string             `json:"name"`
	NativeId      string             `json:"native_id"`
	OrgId         string             `json:"org_id"`
	Paused        bool               `json:"paused"`
	Provider      RepositoryProvider `json:"provider"`
	TeamIds       []string           `json:"team_ids"`
	Url           string             `json:"url"`
	Version       int64              `json:"version"`
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
	CredentialExpiresAt time.Time          `json:"credential_expires_at"`
	Id                  openapi_types.UUID `json:"id"`
	Name                string             `json:"name"`
	OrgId               openapi_types.UUID `json:"org_id"`
	PoolId              openapi_types.UUID `json:"pool_id"`
	State               string             `json:"state"`
	Version             int64              `json:"version"`
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
	Id            openapi_types.UUID   `json:"id"`
	Name          string               `json:"name"`
	OrgId         openapi_types.UUID   `json:"org_id"`
	RepositoryIds []openapi_types.UUID `json:"repository_ids"`
	State         RunnerPoolState      `json:"state"`
	Version       int64                `json:"version"`
}
type RunnerPoolState string
type RunnerPoolInput struct {
	Name          string                `json:"name"`
	RepositoryIds []openapi_types.UUID  `json:"repository_ids"`
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
type Task struct {
	CampaignId         *string   `json:"campaign_id,omitempty"`
	CancelVersion      int64     `json:"cancel_version"`
	CreatedAt          time.Time `json:"created_at"`
	Id                 string    `json:"id"`
	MaxAttempts        int64     `json:"max_attempts"`
	ModelConnectionId  *string   `json:"model_connection_id,omitempty"`
	ModelRoute         string    `json:"model_route"`
	OperationId        string    `json:"operation_id"`
	OrgId              string    `json:"org_id"`
	PolicyHash         string    `json:"policy_hash"`
	Reason             string    `json:"reason"`
	Recipe             string    `json:"recipe"`
	RecipeVersion      string    `json:"recipe_version"`
	RepositoryId       string    `json:"repository_id"`
	RunnerPoolId       *string   `json:"runner_pool_id,omitempty"`
	StartingPolicyHash string    `json:"starting_policy_hash"`
	State              TaskState `json:"state"`
	TargetBranch       string    `json:"target_branch"`
	Version            int64     `json:"version"`
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
type StreamEventsParams struct {
	After       *int64  `form:"after,omitempty" json:"after,omitempty"`
	LastEventID *string `json:"Last-Event-ID,omitempty"`
}
type ReplayEventsParams struct {
	After *int64 `form:"after,omitempty" json:"after,omitempty"`
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
type RepositoriesParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
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
type UploadRunnerArtifactJSONBody = openapi_types.File
type UploadRunnerArtifactTextBody = openapi_types.File
type UploadRunnerArtifactParams struct {
	XArtifactName string `json:"X-Artifact-Name"`
}
type EnrollRunnerJSONBody struct {
	Name string `json:"name"`
}
type InvokeRunnerOperationJSONBody struct {
	ConnectionId openapi_types.UUID     `json:"connection_id"`
	Input        map[string]interface{} `json:"input"`
}
type PollPrivateGrantJSONBody = map[string]interface{}
type CompletePrivateGrantParams struct {
	XPrivateResultCapability string `json:"X-Private-Result-Capability"`
}
type RunnerProgressJSONBody struct {
	State TaskState `json:"state"`
}
type PutBudgetRouteJSONRequestBody = BudgetRouteInput
type PutBudgetJSONRequestBody = BudgetLimitInput
type CreateConnectionJSONRequestBody = ConnectionCreate
type CreateAgentsConnectionJSONRequestBody = ConnectionCreate
type CreateDeliveryConnectionJSONRequestBody = ConnectionCreate
type CreateForgesConnectionJSONRequestBody = ConnectionCreate
type CreateModelsConnectionJSONRequestBody = ConnectionCreate
type SetPrivateRouteJSONRequestBody = PrivateRouteChange
type RotateCredentialJSONRequestBody = CredentialRotation
type PutMembershipJSONRequestBody = MembershipInput
type SetPauseJSONRequestBody = PauseInput
type CreatePolicyVersionJSONRequestBody = PolicyVersionCreate
type ActivatePolicyJSONRequestBody = PolicyActivateRequest
type SimulatePolicyJSONRequestBody = PolicySimulateRequest
type CreateRunnerPoolJSONRequestBody = RunnerPoolInput
type UpdateRunnerPoolJSONRequestBody = RunnerPoolInput
type EnqueueTaskJSONRequestBody = TaskCreate
type PutTeamJSONRequestBody = TeamInput
type BootstrapJSONRequestBody = BootstrapRequest
type UploadRunnerArtifactJSONRequestBody = UploadRunnerArtifactJSONBody
type UploadRunnerArtifactTextRequestBody = UploadRunnerArtifactTextBody
type EnrollRunnerJSONRequestBody EnrollRunnerJSONBody
type InvokeRunnerOperationJSONRequestBody InvokeRunnerOperationJSONBody
type PollPrivateGrantJSONRequestBody = PollPrivateGrantJSONBody
type CompletePrivateGrantJSONRequestBody = PrivateCompletion
type RunnerProgressJSONRequestBody RunnerProgressJSONBody
type RunnerResultJSONRequestBody = RunnerCompletion
