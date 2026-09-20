package generated

import (
	"time"
)

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
type BootstrapRequest struct {
	Name  string `json:"name"`
	Token string `json:"token"`
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
type Role string
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
type RepositoriesParams struct {
	Cursor *string `form:"cursor,omitempty" json:"cursor,omitempty"`
	Limit  *int    `form:"limit,omitempty" json:"limit,omitempty"`
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
type CreateConnectionJSONRequestBody = ConnectionCreate
type CreateAgentsConnectionJSONRequestBody = ConnectionCreate
type CreateDeliveryConnectionJSONRequestBody = ConnectionCreate
type CreateForgesConnectionJSONRequestBody = ConnectionCreate
type CreateModelsConnectionJSONRequestBody = ConnectionCreate
type SetPrivateRouteJSONRequestBody = PrivateRouteChange
type RotateCredentialJSONRequestBody = CredentialRotation
type PutMembershipJSONRequestBody = MembershipInput
type PutTeamJSONRequestBody = TeamInput
type BootstrapJSONRequestBody = BootstrapRequest
