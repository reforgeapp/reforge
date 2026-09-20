package policy

import (
	"time"

	"reforge/internal/domain"
)

type Action string

const (
	Read    Action = "read"
	Repair  Action = "repair"
	Publish Action = "publish"
	Merge   Action = "merge"
	Deploy  Action = "deploy"
	Recover Action = "recover"
)

type Scope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Lists struct {
	Recipes      []string `json:"recipes"`
	Models       []string `json:"models"`
	Routes       []string `json:"routes"`
	MergeMethods []string `json:"merge_methods"`
	Environments []string `json:"environments"`
	Workflows    []string `json:"workflows"`
}

type Limits struct {
	Budget       *int64 `json:"budget,omitempty"`
	Concurrency  *int64 `json:"concurrency,omitempty"`
	Attempts     *int64 `json:"attempts,omitempty"`
	ChangedFiles *int64 `json:"changed_files,omitempty"`
	ChangedLines *int64 `json:"changed_lines,omitempty"`
	OpenChanges  *int64 `json:"open_changes,omitempty"`
}

type Defaults struct {
	Model        string `json:"model,omitempty"`
	Route        string `json:"route,omitempty"`
	BranchPrefix string `json:"branch_prefix,omitempty"`
}

type Requirement struct {
	ID        string   `json:"id"`
	Identity  string   `json:"identity,omitempty"`
	Actions   []Action `json:"actions"`
	Approvals int      `json:"approvals,omitempty"`
}

type Policy struct {
	Schema                string        `json:"schema"`
	Allow                 Lists         `json:"allow"`
	Deny                  []Action      `json:"deny"`
	ForbiddenPaths        []string      `json:"forbidden_paths"`
	Limits                Limits        `json:"limits"`
	Required              []Requirement `json:"required"`
	Defaults              Defaults      `json:"defaults"`
	MaxEvidenceAgeSeconds *int64        `json:"max_evidence_age_seconds,omitempty"`
	Paused                bool          `json:"paused"`
}

type Layer struct {
	Scope          Scope  `json:"scope"`
	VersionID      string `json:"version_id"`
	BindingVersion int64  `json:"binding_version"`
	Policy         Policy `json:"policy"`
}

type Resolved struct {
	Hash            string   `json:"hash"`
	Layers          []Layer  `json:"layers"`
	PrimaryTeamID   string   `json:"primary_team_id,omitempty"`
	RepositoryID    string   `json:"repository_id"`
	Paused          bool     `json:"paused"`
	Policy          Policy   `json:"policy"`
	ScopePaused     bool     `json:"scope_paused"`
	MissingDefaults []string `json:"missing_defaults"`
	Problems        []string `json:"problems"`
}

type Binding struct {
	Head              string `json:"head"`
	Target            string `json:"target"`
	Tested            string `json:"tested"`
	PolicyHash        string `json:"policy_hash"`
	ProviderRules     string `json:"provider_rules"`
	CapabilityVersion string `json:"capability_version"`
	SourceSHA         string `json:"source_sha"`
	Artifact          string `json:"artifact"`
}

type Evidence struct {
	ID         string    `json:"id"`
	Identity   string    `json:"identity,omitempty"`
	State      string    `json:"state"`
	Approvals  int       `json:"approvals"`
	Binding    Binding   `json:"binding"`
	ObservedAt time.Time `json:"observed_at"`
	Reference  string    `json:"reference"`
}

type Input struct {
	Stage              string     `json:"stage,omitempty"`
	Action             Action     `json:"action"`
	Recipe             string     `json:"recipe"`
	Model              string     `json:"model"`
	Route              string     `json:"route"`
	MergeMethod        string     `json:"merge_method"`
	Environment        string     `json:"environment"`
	Workflow           string     `json:"workflow"`
	Paths              []string   `json:"paths"`
	Usage              Limits     `json:"usage"`
	Current            Binding    `json:"current"`
	StartingPolicyHash string     `json:"starting_policy_hash"`
	Evidence           []Evidence `json:"evidence"`
	PausedScopes       []string   `json:"paused_scopes"`
	Now                time.Time  `json:"now"`
}

type Result struct {
	domain.Decision
	Bindings           []Layer  `json:"bindings"`
	EvidenceReferences []string `json:"evidence_references"`
	StartingPolicyHash string   `json:"starting_policy_hash"`
}

type Version struct {
	ID        string    `json:"id"`
	Scope     Scope     `json:"scope"`
	Policy    Policy    `json:"policy"`
	Hash      string    `json:"hash"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type Simulation struct {
	Hash     string   `json:"hash"`
	Resolved Resolved `json:"resolved"`
	Decision Result   `json:"decision"`
}
