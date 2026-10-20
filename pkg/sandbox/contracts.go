package sandbox

import (
	"context"
	"time"
)

type WorkspaceRequest struct {
	JobID         string
	AttemptID     string
	RepositoryURL string
	CommitSHA     string
	Image         string
	Trust         string
	Timeout       time.Duration
	Dependencies  string
	Egress        string
}
type Workspace struct {
	ID        string
	CommitSHA string
	Root      string
	Image     string
}
type Command struct {
	Args           []string
	Directory      string
	Timeout        time.Duration
	MaxOutputBytes int64
	NetworkProfile string
	Stdin          []byte
}
type CommandResult struct {
	ExitCode  int
	Output    []byte
	Truncated bool
	Duration  time.Duration
	TimedOut  bool
}
type CommandSetupError struct {
	Result CommandResult
}

func (e *CommandSetupError) Error() string {
	return "dependency preparation failed: " + string(e.Result.Output)
}

type Patch struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
	Delete  bool   `json:"delete,omitempty"`
}
type Artifact struct {
	Name      string
	MediaType string
	Data      []byte
	SHA256    string
}
type SandboxRuntime interface {
	PreparePinnedWorkspace(context.Context, WorkspaceRequest) (Workspace, error)
	ExecuteBoundedCommand(context.Context, Workspace, Command) (CommandResult, error)
	ApplyPatch(context.Context, Workspace, []Patch) error
	CollectArtifact(context.Context, Workspace, string) (Artifact, error)
	Destroy(context.Context, Workspace) error
}
