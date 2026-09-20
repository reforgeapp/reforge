package privateconnector

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/model"
	"reforge/internal/network"
	"reforge/internal/runner"
)

const MaxResponse = 6 << 20
const MaxGrant = 1 << 20
const MaxTTL = 30 * time.Second
const DefaultTTL = 10 * time.Second

var ErrUnavailable = errors.New("private supervisor unavailable")
var ErrUnsupported = errors.New("private operation is not registered")
var ErrInvalid = errors.New("invalid private operation")
var ErrConflict = errors.New("private operation already active or consumed")
var ErrUncertain = errors.New("private operation outcome uncertain; reconcile before retry")

type Kind string

const (
	ForgeProbe            Kind = "forge.probe"
	ForgeInventory        Kind = "forge.inventory"
	ForgeRepository       Kind = "forge.repository"
	ForgeResolveRef       Kind = "forge.resolve_ref"
	ForgeReadFile         Kind = "forge.read_file"
	ForgeSourceManifest   Kind = "forge.source_manifest"
	ForgeReadChange       Kind = "forge.read_change"
	ForgeChecks           Kind = "forge.checks"
	ForgeApprovals        Kind = "forge.approvals"
	ForgeReconcileChanges Kind = "forge.reconcile_changes"
	ModelProbe            Kind = "model.probe"
	ModelList             Kind = "model.list"
	GiteaProbe                 = ForgeProbe
	GiteaInventory             = ForgeInventory
	GiteaRepository            = ForgeRepository
	GiteaResolveRef            = ForgeResolveRef
	GiteaReadFile              = ForgeReadFile
	GiteaReadChange            = ForgeReadChange
	GiteaChecks                = ForgeChecks
	GiteaApprovals             = ForgeApprovals
)

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
type ChangeArgs struct {
	Repository forge.RepoRef `json:"repository"`
	ChangeID   string        `json:"change_id"`
}
type ChecksArgs struct {
	Repository forge.RepoRef `json:"repository"`
	CommitSHA  string        `json:"commit_sha"`
}
type Operation struct {
	Source     *ChecksArgs     `json:"source,omitempty"`
	Changes    *ChangesArgs    `json:"changes,omitempty"`
	ID         string          `json:"id"`
	Kind       Kind            `json:"kind"`
	Inventory  *InventoryArgs  `json:"inventory,omitempty"`
	Repository *RepositoryArgs `json:"repository,omitempty"`
	Ref        *RefArgs        `json:"ref,omitempty"`
	File       *FileArgs       `json:"file,omitempty"`
	Change     *ChangeArgs     `json:"change,omitempty"`
	Checks     *ChecksArgs     `json:"checks,omitempty"`
}

func (o Operation) Validate() error { return o.validate() }

func (o Operation) validate() error {
	if !auth.ValidID(o.ID) {
		return ErrInvalid
	}
	count := 0
	for _, present := range []bool{o.Inventory != nil, o.Repository != nil, o.Ref != nil, o.File != nil, o.Change != nil, o.Checks != nil, o.Changes != nil, o.Source != nil} {
		if present {
			count++
		}
	}
	valid := false
	switch o.Kind {
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
	Kind              string               `json:"kind,omitempty"`
	AuthKind          string               `json:"auth_kind,omitempty"`
	AppID             string               `json:"app_id,omitempty"`
	InstallationID    string               `json:"installation_id,omitempty"`
	Model             string               `json:"model,omitempty"`
	Profile           string               `json:"profile,omitempty"`
	OrgID             string               `json:"org_id"`
	ID                string               `json:"id"`
	Version           int64                `json:"version"`
	CredentialVersion int64                `json:"credential_version"`
	Provider          string               `json:"provider"`
	Endpoint          string               `json:"endpoint"`
	Route             network.PrivateRoute `json:"route"`
	CAPEM             []byte               `json:"ca_pem,omitempty"`
	Secret            string               `json:"-"`
}

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
	Change            *forge.Change                  `json:"change,omitempty"`
	Checks            []forge.Check                  `json:"checks,omitempty"`
	Approvals         []forge.Approval               `json:"approvals,omitempty"`
}
type Authenticate func(context.Context, string) (runner.Runner, error)
type Deliver func(GrantSpec) (Result, error)
type Authorize func(context.Context, Ready, Deliver) error

type wireGrant struct {
	Grant
	Secret     string `json:"secret"`
	Capability string `json:"result_capability"`
}

func (g Grant) MarshalWire() ([]byte, error) {
	return json.Marshal(wireGrant{Grant: g, Secret: g.Connection.Secret, Capability: g.ResultCapability})
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
	w.Grant.ResultCapability = w.Capability
	return w.Grant, nil
}
