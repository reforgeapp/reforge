package forge

import (
	"context"
	"log/slog"
	"net/http"
	"reforge/internal/domain"
	"time"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	OrgID         string
	ConnectionID  string
	BaseURL       string
	Token         string     `json:"-"`
	WebhookSecret string     `json:"-"`
	Client        HTTPClient `json:"-"`
	ServerVersion string
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(slog.String("org_id", c.OrgID), slog.String("connection_id", c.ConnectionID))
}

func (c Config) String() string   { return "forge connection " + c.ConnectionID }
func (c Config) GoString() string { return c.String() }

type RepoRef struct {
	NativeID string `json:"native_id"`
	FullName string `json:"full_name"`
}

type Repository struct {
	RepoRef
	URL           string   `json:"url"`
	CloneURL      string   `json:"clone_url"`
	DefaultBranch string   `json:"default_branch"`
	Archived      bool     `json:"archived"`
	Private       bool     `json:"private"`
	Permissions   []string `json:"permissions"`
}

type InventoryRequest struct {
	Namespace string
	Cursor    string
	Limit     int
}

type Capabilities struct {
	Provider      string                       `json:"provider"`
	ServerVersion string                       `json:"server_version"`
	Features      map[string]domain.Capability `json:"features"`
}

type File struct {
	Path    string
	SHA     string
	Content []byte
}
type SourceEntry struct {
	Path string `json:"path"`
	SHA  string `json:"sha"`
	Mode string `json:"mode"`
	Type string `json:"type"`
}
type SourceManifest struct {
	Repository   RepoRef       `json:"repository"`
	CommitSHA    string        `json:"commit_sha"`
	TreeSHA      string        `json:"tree_sha"`
	ObjectFormat string        `json:"object_format"`
	Proof        string        `json:"proof"`
	Entries      []SourceEntry `json:"entries"`
	Complete     bool          `json:"complete"`
}
type ForgeSource interface {
	ReadSourceManifest(context.Context, RepoRef, string) (SourceManifest, error)
}
type Event struct {
	DeliveryID string
	Kind       string
	Repository RepoRef
	ChangeID   string
	HeadSHA    string
}
type Check struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PublisherID string `json:"publisher_id"`
	HeadSHA     string `json:"head_sha"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	URL         string `json:"url"`
}

type Approval struct {
	ID        string `json:"id"`
	ActorID   string `json:"actor_id"`
	HeadSHA   string `json:"head_sha"`
	State     string `json:"state"`
	Dismissed bool   `json:"dismissed"`
}

type Change struct {
	ID               string  `json:"id"`
	Repository       RepoRef `json:"repository"`
	HeadRepository   RepoRef `json:"head_repository"`
	TargetRepository RepoRef `json:"target_repository"`
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	URL              string  `json:"url"`
	HeadSHA          string  `json:"head_sha"`
	TargetSHA        string  `json:"target_sha"`
	HeadBranch       string  `json:"head_branch"`
	TargetBranch     string  `json:"target_branch"`
	AuthorID         string  `json:"author_id"`
	AuthorLogin      string  `json:"author_login"`
	AuthorType       string  `json:"author_type"`
	State            string  `json:"state"`
	Draft            bool    `json:"draft"`
	MergeSHA         string  `json:"merge_sha"`
	MergeStatus      string  `json:"merge_status"`
	OperationID      string  `json:"operation_id"`
}

type CheckRule struct {
	Name        string `json:"name"`
	PublisherID string `json:"publisher_id"`
}
type Rules struct {
	State                domain.CapabilityState `json:"state"`
	Reason               string                 `json:"reason"`
	Hash                 string                 `json:"hash"`
	ObservedAt           time.Time              `json:"observed_at"`
	RequiredChecks       []CheckRule            `json:"required_checks"`
	RequiredApprovals    int                    `json:"required_approvals"`
	DismissStaleReviews  bool                   `json:"dismiss_stale_reviews"`
	RequireCodeOwners    bool                   `json:"require_code_owners"`
	CodeOwnersEnforced   domain.CapabilityState `json:"code_owners_enforced"`
	RequireStrictTarget  bool                   `json:"require_strict_target"`
	StrictTargetEnforced domain.CapabilityState `json:"strict_target_enforced"`
	RequireQueue         bool                   `json:"require_queue"`
	AllowedMergeMethods  []string               `json:"allowed_merge_methods"`
	ActorCanBypass       bool                   `json:"actor_can_bypass"`
}

type NativeEligibility struct {
	State     string   `json:"state"`
	Blockers  []string `json:"blockers"`
	HeadSHA   string   `json:"head_sha"`
	TargetSHA string   `json:"target_sha"`
}
type QueueState struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	HeadSHA   string `json:"head_sha"`
	TestedSHA string `json:"tested_sha"`
	TargetSHA string `json:"target_sha"`
}
type MergeEvidence struct {
	ExecutionCheck CheckRule         `json:"execution_check"`
	Change         Change            `json:"change"`
	Rules          Rules             `json:"rules"`
	Checks         []Check           `json:"checks"`
	Approvals      []Approval        `json:"approvals"`
	Native         NativeEligibility `json:"native"`
	Queue          QueueState        `json:"queue"`
	Capabilities   Capabilities      `json:"capabilities"`
	ObservedAt     time.Time         `json:"observed_at"`
}

type CreateChangeRequest struct {
	Repository      RepoRef
	Title           string
	Body            string
	HeadBranch      string
	TargetBranch    string
	ExpectedHeadSHA string
	OperationID     string
	Draft           bool
}
type FileEdit struct {
	Path    string
	Content []byte
	Delete  bool
}
type UpdateBranchRequest struct {
	Repository     RepoRef
	Branch         string
	ExpectedOldSHA string
	BaseSHA        string
	Message        string
	Edits          []FileEdit
	OperationID    string
}
type MergeRequest struct {
	Repository        RepoRef
	ChangeID          string
	ExpectedHeadSHA   string
	ExpectedTargetSHA string
	RulesHash         string
	GateID            string
	Method            string
	OperationID       string
	Queue             bool
}
type MergeResult struct {
	State    string `json:"state"`
	NativeID string `json:"native_id"`
	MergeSHA string `json:"merge_sha"`
	HeadSHA  string `json:"head_sha"`
	URL      string `json:"url"`
}

type Workflow struct {
	ID   string
	Name string
	Ref  string
	Path string
	URL  string
}
type PipelineRequest struct {
	Repository     RepoRef
	WorkflowID     string
	Ref            string
	SourceSHA      string
	ArtifactDigest string
	Environment    string
	CorrelationID  string
	Inputs         map[string]string
	ObserveOnly    bool
}
type DeploymentGates struct {
	State          string
	NativeEnforced domain.CapabilityState
	Blockers       []string
	ApprovalURL    string
}
type DeploymentStatus struct {
	ID             string
	State          string
	SourceSHA      string
	ArtifactDigest string
	Environment    string
	CorrelationID  string
	URL            string
	Health         string
	ObservedAt     time.Time
}

type ForgeInventory interface {
	ProbeCapabilities(context.Context) (Capabilities, error)
	ListRepositories(context.Context, InventoryRequest) (domain.Page[Repository], error)
	GetRepository(context.Context, RepoRef) (Repository, error)
	ReadFileAtRef(context.Context, RepoRef, string, string) (File, error)
	ResolveRef(context.Context, RepoRef, string) (string, error)
}
type ForgeEvents interface {
	VerifyWebhook(http.Header, []byte) error
	DecodeEvent(http.Header, []byte) (Event, error)
	ReconcileChanges(context.Context, RepoRef, string) (domain.Page[Change], error)
	ListChecks(context.Context, RepoRef, string) ([]Check, error)
	ListBotWork(context.Context, RepoRef) ([]Change, error)
}
type ForgeChanges interface {
	CreateChange(context.Context, CreateChangeRequest) (Change, error)
	FindChangeByOperation(ctx context.Context, repository RepoRef, operationID, headBranch, targetBranch string) (*Change, error)
	UpdateAppBranch(context.Context, UpdateBranchRequest) (string, error)
	ReadChange(context.Context, RepoRef, string) (Change, error)
	RequestReview(context.Context, RepoRef, string, []string) error
}
type ForgeProtection interface {
	ReadEffectiveRules(context.Context, RepoRef, string) (Rules, error)
	EvaluateNativeEligibility(context.Context, RepoRef, string) (NativeEligibility, error)
	ReadApprovals(context.Context, RepoRef, string) ([]Approval, error)
	ReadQueueState(context.Context, RepoRef, string) (QueueState, error)
}
type ForgeMerge interface {
	RequestNativeMergeOrQueue(context.Context, MergeRequest) (MergeResult, error)
	ReadMergeResult(context.Context, RepoRef, string) (MergeResult, error)
}

type QueueCancelRequest struct {
	Repository      RepoRef
	ChangeID        string
	QueueID         string
	ExpectedHeadSHA string
	OperationID     string
}

type ForgeQueueControl interface {
	CancelNativeQueue(context.Context, QueueCancelRequest) (QueueState, error)
}

type ExecutionCheckRequest struct {
	Repository  RepoRef
	SHA         string
	Name        string
	State       string
	OperationID string
	CheckID     string
}

type ExecutionCheck struct {
	ID          string `json:"id"`
	SHA         string `json:"sha"`
	Name        string `json:"name"`
	State       string `json:"state"`
	PublisherID string `json:"publisher_id"`
	OperationID string `json:"operation_id"`
}

type ForgeExecutionChecks interface {
	WriteExecutionCheck(context.Context, ExecutionCheckRequest) (ExecutionCheck, error)
	ReadExecutionCheck(context.Context, RepoRef, string) (ExecutionCheck, error)
}

const QueueExecutionCheckName = "reforge/merge-policy"

type ForgeQueuePrerequisites interface {
	EvaluateQueuePrerequisites(context.Context, RepoRef, string) (NativeEligibility, CheckRule, error)
}
type ForgeDelivery interface {
	ListAllowedWorkflows(context.Context, RepoRef) ([]Workflow, error)
	TriggerOrObservePipeline(context.Context, PipelineRequest) (DeploymentStatus, error)
	ReadDeploymentGates(context.Context, RepoRef, string) (DeploymentGates, error)
	ReadDeploymentStatus(context.Context, RepoRef, string) (DeploymentStatus, error)
	RequestAllowedRecovery(context.Context, PipelineRequest) (DeploymentStatus, error)
}
type Provider interface {
	ForgeInventory
	ForgeEvents
	ForgeChanges
	ForgeProtection
	ForgeMerge
	ForgeDelivery
}

type CommitProof struct {
	SHA      string   `json:"sha"`
	Parents  []string `json:"parents"`
	Message  string   `json:"message"`
	AuthorID string   `json:"author_id"`
}
type ForgeCommits interface {
	ReadCommitProof(context.Context, RepoRef, string) (CommitProof, error)
}
