package privateconnector

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/model"
	"reforge/internal/network"
	"reforge/internal/runner"
)

const MaxResponse = 6 << 20
const MaxBatchFiles = 50
const batchBytes = 3 << 20
const MaxGrant = 1 << 20
const MaxModelTTL = 5 * time.Minute
const MaxTTL = 30 * time.Second
const DefaultTTL = 10 * time.Second

var ErrUnavailable = errors.New("private supervisor unavailable")
var ErrUnsupported = errors.New("private operation is not registered")
var ErrInvalid = errors.New("invalid private operation")
var ErrConflict = errors.New("private operation already active or consumed")
var ErrUncertain = errors.New("private operation outcome uncertain; reconcile before retry")

type Kind string

const (
	ForgePipelineCancel      Kind = "forge.pipeline_cancel"
	ForgePipelineInspect     Kind = "forge.pipeline_inspect"
	ForgeDeliveryWorkflows   Kind = "forge.delivery_workflows"
	ForgeDeliveryGates       Kind = "forge.delivery_gates"
	ForgeDeliveryStatus      Kind = "forge.delivery_status"
	ForgePipelineObserve     Kind = "forge.pipeline_observe"
	ForgePipelineTrigger     Kind = "forge.pipeline_trigger"
	ForgePipelineRecover     Kind = "forge.pipeline_recover"
	ForgeMergeInspect        Kind = "forge.merge_inspect"
	ForgeReadTrainGate       Kind = "forge.read_train_gate"
	ForgeReleaseTrainGate    Kind = "forge.release_train_gate"
	ForgeQueueInspect        Kind = "forge.queue_inspect"
	ForgeMerge               Kind = "forge.merge"
	ForgeMergeResult         Kind = "forge.merge_result"
	ForgeQueueState          Kind = "forge.queue_state"
	ForgeCancelQueue         Kind = "forge.cancel_queue"
	ForgeCloseChange         Kind = "forge.close_change"
	ForgeReadExecutionCheck  Kind = "forge.read_execution_check"
	ForgeWriteExecutionCheck Kind = "forge.write_execution_check"
	ForgeCommitProof         Kind = "forge.commit_proof"
	ForgeUpdateBranch        Kind = "forge.update_branch"
	ForgeCreateChange        Kind = "forge.create_change"
	ForgeFindChange          Kind = "forge.find_change"
	ForgeProbe               Kind = "forge.probe"
	ForgeInventory           Kind = "forge.inventory"
	ForgeRepository          Kind = "forge.repository"
	ForgeResolveRef          Kind = "forge.resolve_ref"
	ForgeReadFile            Kind = "forge.read_file"
	ForgeReadFiles           Kind = "forge.read_files"
	ForgeCheckLog            Kind = "forge.check_log"
	ForgeSourceManifest      Kind = "forge.source_manifest"
	ForgeReadChange          Kind = "forge.read_change"
	ForgeChecks              Kind = "forge.checks"
	ForgeApprovals           Kind = "forge.approvals"
	ForgeReconcileChanges    Kind = "forge.reconcile_changes"
	ModelProbe               Kind = "model.probe"
	ModelList                Kind = "model.list"
	ModelTurn                Kind = "model.turn"
	GiteaProbe                    = ForgeProbe
	GiteaInventory                = ForgeInventory
	GiteaRepository               = ForgeRepository
	GiteaResolveRef               = ForgeResolveRef
	GiteaReadFile                 = ForgeReadFile
	GiteaReadChange               = ForgeReadChange
	GiteaChecks                   = ForgeChecks
	GiteaApprovals                = ForgeApprovals
)

type FindChangeArgs struct {
	Repository   forge.RepoRef `json:"repository"`
	OperationID  string        `json:"operation_id"`
	HeadBranch   string        `json:"head_branch"`
	TargetBranch string        `json:"target_branch"`
}

type ChangesArgs struct {
	Repository forge.RepoRef `json:"repository"`
	Cursor     string        `json:"cursor,omitempty"`
}

type InventoryArgs struct {
	Namespace string `json:"namespace,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit"`
}
type RepositoryArgs struct {
	Repository forge.RepoRef `json:"repository"`
}
type RefArgs struct {
	Repository forge.RepoRef `json:"repository"`
	Ref        string        `json:"ref"`
}
type FileArgs struct {
	Repository forge.RepoRef `json:"repository"`
	CommitSHA  string        `json:"commit_sha"`
	Path       string        `json:"path"`
}
type FilesArgs struct {
	Repository forge.RepoRef `json:"repository"`
	CommitSHA  string        `json:"commit_sha"`
	Paths      []string      `json:"paths"`
}
type CheckLogArgs struct {
	Repository forge.RepoRef `json:"repository"`
	CheckID    string        `json:"check_id"`
}
type ChangeArgs struct {
	Repository forge.RepoRef `json:"repository"`
	ChangeID   string        `json:"change_id"`
}
type ChecksArgs struct {
	Repository forge.RepoRef `json:"repository"`
	CommitSHA  string        `json:"commit_sha"`
}
type DeliveryArgs struct {
	Repository  forge.RepoRef `json:"repository"`
	Environment string        `json:"environment"`
	RunID       string        `json:"run_id"`
}

type Operation struct {
	Delivery       *DeliveryArgs                `json:"delivery,omitempty"`
	Pipeline       *forge.PipelineRequest       `json:"pipeline,omitempty"`
	TrainGate      *forge.TrainGateRequest      `json:"train_gate,omitempty"`
	ExecutionCheck *forge.ExecutionCheckRequest `json:"execution_check,omitempty"`
	CancelQueue    *forge.QueueCancelRequest    `json:"cancel_queue,omitempty"`
	Close          *forge.CloseChangeRequest    `json:"close,omitempty"`
	Merge          *forge.MergeRequest          `json:"merge,omitempty"`
	Commit         *ChecksArgs                  `json:"commit,omitempty"`
	Branch         *forge.UpdateBranchRequest   `json:"branch,omitempty"`
	Create         *forge.CreateChangeRequest   `json:"create,omitempty"`
	Find           *FindChangeArgs              `json:"find,omitempty"`
	Turn           *model.Turn                  `json:"turn,omitempty"`
	Source         *ChecksArgs                  `json:"source,omitempty"`
	Changes        *ChangesArgs                 `json:"changes,omitempty"`
	ID             string                       `json:"id"`
	Kind           Kind                         `json:"kind"`
	Inventory      *InventoryArgs               `json:"inventory,omitempty"`
	Repository     *RepositoryArgs              `json:"repository,omitempty"`
	Ref            *RefArgs                     `json:"ref,omitempty"`
	File           *FileArgs                    `json:"file,omitempty"`
	Files          *FilesArgs                   `json:"files,omitempty"`
	CheckLog       *CheckLogArgs                `json:"check_log,omitempty"`
	Change         *ChangeArgs                  `json:"change,omitempty"`
	Checks         *ChecksArgs                  `json:"checks,omitempty"`
}

func (o Operation) Validate() error { return o.validate() }

func (o Operation) validate() error {
	if !auth.ValidID(o.ID) {
		return ErrInvalid
	}
	count := 0
	for _, present := range []bool{o.Delivery != nil, o.Pipeline != nil, o.TrainGate != nil, o.ExecutionCheck != nil, o.CancelQueue != nil, o.Close != nil, o.Merge != nil, o.Commit != nil, o.Branch != nil, o.Create != nil, o.Find != nil, o.Inventory != nil, o.Repository != nil, o.Ref != nil, o.File != nil, o.Files != nil, o.CheckLog != nil, o.Change != nil, o.Checks != nil, o.Changes != nil, o.Source != nil, o.Turn != nil} {
		if present {
			count++
		}
	}
	valid := false
	switch o.Kind {
	case ForgeDeliveryWorkflows:
		valid = o.Delivery != nil && o.Delivery.Repository.NativeID != "" && len(o.Delivery.Repository.FullName) <= 1024
	case ForgeDeliveryGates:
		valid = o.Delivery != nil && o.Delivery.Repository.NativeID != "" && forge.ValidEnvironment(o.Delivery.Environment)
	case ForgeDeliveryStatus:
		valid = o.Delivery != nil && o.Delivery.Repository.NativeID != "" && o.Delivery.RunID != "" && len(o.Delivery.RunID) <= 32
	case ForgePipelineCancel:
		valid = o.Pipeline != nil && forge.ValidPipelineRequest(*o.Pipeline) && !o.Pipeline.ObserveOnly && o.Pipeline.RunID != "" && len(o.Pipeline.RunID) <= 32
	case ForgePipelineInspect, ForgePipelineObserve, ForgePipelineTrigger, ForgePipelineRecover:
		valid = o.Pipeline != nil && forge.ValidPipelineRequest(*o.Pipeline) && o.Pipeline.ObserveOnly == (o.Kind == ForgePipelineObserve || o.Kind == ForgePipelineInspect) && (o.Kind == ForgePipelineObserve || o.Kind == ForgePipelineInspect || o.Pipeline.CorrelationID == o.ID)
	case ForgeReleaseTrainGate:
		v := o.TrainGate
		valid = v != nil && len(v.RulesHash) == 64 && v.OperationID == o.ID && v.Repository.NativeID != "" && len(v.Repository.FullName) <= 1024 && v.ChangeID != "" && len(v.ChangeID) <= 32 && len(v.Gate.JobID) > 0 && len(v.Gate.JobID) <= 32 && len(v.Gate.SHA) == 40 && len(v.Gate.HeadSHA) == 40 && len(v.Gate.TargetSHA) == 40 && len(v.Gate.CIConfigSHA256) == 64
	case ForgeReadExecutionCheck:
		valid = o.ExecutionCheck != nil && o.ExecutionCheck.Repository.NativeID != "" && len(o.ExecutionCheck.Repository.FullName) <= 1024 && o.ExecutionCheck.CheckID != "" && len(o.ExecutionCheck.CheckID) <= 128
	case ForgeWriteExecutionCheck:
		v := o.ExecutionCheck
		valid = v != nil && auth.ValidID(v.OperationID) && v.Repository.NativeID != "" && len(v.Repository.FullName) <= 1024 && len(v.SHA) == 40 && v.Name != "" && len(v.Name) <= 100 && len(v.CheckID) <= 128 && (v.State == "pending" || v.State == "success" || v.State == "failure")
	case ForgeCancelQueue:
		valid = o.CancelQueue != nil && o.CancelQueue.OperationID == o.ID && o.CancelQueue.Repository.NativeID != "" && len(o.CancelQueue.Repository.FullName) <= 1024 && len(o.CancelQueue.ExpectedHeadSHA) == 40 && o.CancelQueue.ChangeID != "" && len(o.CancelQueue.ChangeID) <= 32 && o.CancelQueue.QueueID != "" && len(o.CancelQueue.QueueID) <= 256
	case ForgeCloseChange:
		valid = o.Close != nil && o.Close.Valid() && len(o.Close.Repository.FullName) <= 1024
	case ForgeMerge:
		valid = o.Merge != nil && o.Merge.OperationID == o.ID && auth.ValidID(o.Merge.GateID) && len(o.Merge.ExpectedHeadSHA) == 40 && len(o.Merge.ExpectedTargetSHA) == 40 && len(o.Merge.RulesHash) == 64 && o.Merge.ChangeID != ""
	case ForgeReadTrainGate, ForgeMergeInspect, ForgeQueueInspect, ForgeMergeResult, ForgeQueueState:
		valid = o.Change != nil && o.Change.ChangeID != "" && len(o.Change.ChangeID) <= 32 && o.Change.Repository.NativeID != ""
	case ForgeCommitProof:
		valid = o.Commit != nil && len(o.Commit.CommitSHA) == 40 && o.Commit.Repository.NativeID != ""
	case ForgeUpdateBranch:
		valid = o.Branch != nil && o.Branch.OperationID == o.ID && strings.HasPrefix(o.Branch.Branch, "reforge/") && len(o.Branch.Edits) > 0 && len(o.Branch.Edits) <= 20 && len(o.Branch.BaseSHA) == 40
	case ForgeCreateChange:
		valid = o.Create != nil && o.Create.OperationID == o.ID && strings.HasPrefix(o.Create.HeadBranch, "reforge/") && len(o.Create.ExpectedHeadSHA) == 40
	case ForgeFindChange:
		valid = o.Find != nil && auth.ValidID(o.Find.OperationID) && strings.HasPrefix(o.Find.HeadBranch, "reforge/") && len(o.Find.TargetBranch) > 0 && len(o.Find.TargetBranch) <= 255
	case ModelTurn:
		valid = o.Turn != nil && o.Turn.Valid() && o.Turn.OperationID == o.ID
	case ForgeSourceManifest:
		valid = o.Source != nil && len(o.Source.CommitSHA) == 40 && o.Source.Repository.NativeID != "" && len(o.Source.Repository.NativeID) <= 256 && len(o.Source.Repository.FullName) <= 1024
	case ForgeProbe, ModelProbe, ModelList:
		valid = count == 0
	case ForgeReconcileChanges:
		valid = o.Changes != nil && len(o.Changes.Cursor) <= 32
	case GiteaInventory:
		valid = o.Inventory != nil && o.Inventory.Limit > 0 && o.Inventory.Limit <= 100 && len(o.Inventory.Namespace) <= 256 && len(o.Inventory.Cursor) <= 32
	case GiteaRepository:
		valid = o.Repository != nil
	case GiteaResolveRef:
		valid = o.Ref != nil && o.Ref.Ref != "" && len(o.Ref.Ref) <= 1024
	case GiteaReadFile:
		valid = o.File != nil && len(o.File.CommitSHA) <= 64 && o.File.Path != "" && len(o.File.Path) <= 1024
	case ForgeCheckLog:
		valid = o.CheckLog != nil && o.CheckLog.CheckID != "" && len(o.CheckLog.CheckID) <= 32 && o.CheckLog.Repository.NativeID != ""
	case ForgeReadFiles:
		valid = o.Files != nil && len(o.Files.CommitSHA) <= 64 && len(o.Files.Paths) > 0 && len(o.Files.Paths) <= MaxBatchFiles
		for _, path := range o.Files.Paths {
			valid = valid && path != "" && len(path) <= 1024
		}
	case GiteaReadChange, GiteaApprovals:
		valid = o.Change != nil && o.Change.ChangeID != "" && len(o.Change.ChangeID) <= 32
	case GiteaChecks:
		valid = o.Checks != nil && len(o.Checks.CommitSHA) <= 64
	default:
		return ErrUnsupported
	}
	if !valid || o.Kind != ForgeProbe && o.Kind != ModelProbe && o.Kind != ModelList && count != 1 {
		return ErrInvalid
	}
	b, _ := json.Marshal(o)
	if len(b) > MaxGrant/2 {
		return ErrInvalid
	}
	return nil
}

type Target struct {
	OrgID    string `json:"org_id"`
	RunnerID string `json:"runner_id"`
}
type Connection struct {
	Protection        *ProtectionCredential `json:"protection,omitempty"`
	CheckPublishers   map[string]string     `json:"check_publishers,omitempty"`
	Kind              string                `json:"kind,omitempty"`
	AuthKind          string                `json:"auth_kind,omitempty"`
	AppID             string                `json:"app_id,omitempty"`
	InstallationID    string                `json:"installation_id,omitempty"`
	Model             string                `json:"model,omitempty"`
	Profile           string                `json:"profile,omitempty"`
	OrgID             string                `json:"org_id"`
	ID                string                `json:"id"`
	Version           int64                 `json:"version"`
	CredentialVersion int64                 `json:"credential_version"`
	Provider          string                `json:"provider"`
	Endpoint          string                `json:"endpoint"`
	Route             network.PrivateRoute  `json:"route"`
	CAPEM             []byte                `json:"ca_pem,omitempty"`
	Secret            string                `json:"-"`
}

type ProtectionCredential struct {
	ID                string `json:"id"`
	Version           int64  `json:"version"`
	CredentialVersion int64  `json:"credential_version"`
	Secret            string `json:"-"`
}

func (p ProtectionCredential) String() string       { return "protection credential [redacted]" }
func (p ProtectionCredential) GoString() string     { return p.String() }
func (p ProtectionCredential) LogValue() slog.Value { return slog.StringValue(p.String()) }

func (c Connection) String() string       { return "private connection " + c.ID }
func (c Connection) GoString() string     { return c.String() }
func (c Connection) LogValue() slog.Value { return slog.StringValue(c.String()) }

type Ready struct {
	runner.Runner
	CredentialHash string `json:"-"`
}

func (r Ready) String() string       { return "private readiness " + r.ID }
func (r Ready) GoString() string     { return r.String() }
func (r Ready) LogValue() slog.Value { return slog.StringValue(r.String()) }

type GrantSpec struct {
	Check          func(context.Context) error `json:"-"`
	RunnerVersion  int64
	CredentialHash string `json:"-"`
	OperationID    string
	AuthorityID    string
	Connection     Connection
}

func (g GrantSpec) String() string       { return "private grant specification [redacted]" }
func (g GrantSpec) GoString() string     { return g.String() }
func (g GrantSpec) LogValue() slog.Value { return slog.StringValue(g.String()) }

type Grant struct {
	ID                string     `json:"id"`
	RunnerVersion     int64      `json:"runner_version"`
	Target            Target     `json:"target"`
	Operation         Operation  `json:"operation"`
	AuthorityID       string     `json:"authority_id"`
	Connection        Connection `json:"connection"`
	TimeoutMS         int64      `json:"timeout_ms"`
	executionDeadline time.Time
	ExpiresAt         time.Time `json:"expires_at"`
	ResultCapability  string    `json:"-"`
}

func (g Grant) String() string       { return "private grant " + g.ID }
func (g Grant) GoString() string     { return g.String() }
func (g Grant) LogValue() slog.Value { return slog.StringValue(g.String()) }

type Failure struct {
	RetryAfterMS int64  `json:"retry_after_ms,omitempty"`
	Code         string `json:"code"`
	Uncertain    bool   `json:"uncertain"`
}
type Result struct {
	Workflows         []forge.Workflow               `json:"workflows,omitempty"`
	DeploymentGates   *forge.DeploymentGates         `json:"deployment_gates,omitempty"`
	Deployment        *forge.DeploymentStatus        `json:"deployment,omitempty"`
	TrainGate         *forge.TrainGate               `json:"train_gate,omitempty"`
	ExecutionCheck    *forge.ExecutionCheck          `json:"execution_check,omitempty"`
	Queue             *forge.QueueState              `json:"queue,omitempty"`
	MergeEvidence     *forge.MergeEvidence           `json:"merge_evidence,omitempty"`
	Merge             *forge.MergeResult             `json:"merge,omitempty"`
	Commit            *forge.CommitProof             `json:"commit,omitempty"`
	Turn              *model.TurnResult              `json:"turn,omitempty"`
	Manifest          *forge.SourceManifest          `json:"manifest,omitempty"`
	Changes           *domain.Page[forge.Change]     `json:"changes,omitempty"`
	ModelCapabilities *model.Capabilities            `json:"model_capabilities,omitempty"`
	Models            []model.Model                  `json:"models,omitempty"`
	OperationID       string                         `json:"operation_id"`
	Failure           *Failure                       `json:"failure,omitempty"`
	Capabilities      *forge.Capabilities            `json:"capabilities,omitempty"`
	Inventory         *domain.Page[forge.Repository] `json:"inventory,omitempty"`
	Repository        *forge.Repository              `json:"repository,omitempty"`
	SHA               string                         `json:"sha,omitempty"`
	File              *forge.File                    `json:"file,omitempty"`
	Files             []forge.File                   `json:"files,omitempty"`
	Log               string                         `json:"log,omitempty"`
	Change            *forge.Change                  `json:"change,omitempty"`
	Checks            []forge.Check                  `json:"checks,omitempty"`
	Approvals         []forge.Approval               `json:"approvals,omitempty"`
}
type Authenticate func(context.Context, string) (runner.Runner, error)
type Deliver func(GrantSpec) (Result, error)
type Authorize func(context.Context, Ready, Deliver) error

type wireGrant struct {
	Grant
	ProtectionSecret string `json:"protection_secret,omitempty"`
	Secret           string `json:"secret"`
	Capability       string `json:"result_capability"`
}

func (g Grant) MarshalWire() ([]byte, error) {
	w := wireGrant{Grant: g, Secret: g.Connection.Secret, Capability: g.ResultCapability}
	if g.Connection.Protection != nil {
		w.ProtectionSecret = g.Connection.Protection.Secret
	}
	return json.Marshal(w)
}
func DecodeGrant(b []byte) (Grant, error) {
	if len(b) > MaxGrant {
		return Grant{}, ErrInvalid
	}
	var w wireGrant
	if err := strictJSON(b, &w); err != nil {
		return Grant{}, err
	}
	w.Grant.Connection.Secret = w.Secret
	if w.Grant.Connection.Protection != nil {
		w.Grant.Connection.Protection.Secret = w.ProtectionSecret
	} else if w.ProtectionSecret != "" {
		return Grant{}, ErrInvalid
	}
	w.Grant.ResultCapability = w.Capability
	return w.Grant, nil
}

func (o Operation) MaximumTTL() time.Duration {
	if o.Kind == ModelTurn {
		return MaxModelTTL
	}
	return MaxTTL
}
func (o Operation) ttl(fallback time.Duration) time.Duration {
	if o.Kind == ModelTurn && o.Turn != nil {
		return time.Duration(o.Turn.TimeoutMS) * time.Millisecond
	}
	return fallback
}

func (o Operation) Mutation() bool {
	return o.Kind == ForgePipelineCancel || o.Kind == ForgePipelineTrigger || o.Kind == ForgePipelineRecover || o.Kind == ForgeReleaseTrainGate || o.Kind == ForgeUpdateBranch || o.Kind == ForgeCreateChange || o.Kind == ForgeMerge || o.Kind == ForgeCancelQueue || o.Kind == ForgeCloseChange || o.Kind == ForgeWriteExecutionCheck
}
