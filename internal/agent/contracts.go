package agent

import (
	"context"
	"encoding/json"
	"reforge/internal/domain"
	"reforge/internal/sandbox"
)

type Probe struct {
	Runtime      string
	Version      string
	AuthState    string
	BillingRoute string
	Features     map[string]domain.Capability
}
type Request struct {
	JobID      string
	AttemptID  string
	Prompt     string
	Workspace  sandbox.Workspace
	Model      string
	PolicyHash string
	MaxTurns   int
}
type Event struct {
	Type       string
	ID         string
	Text       string
	ApprovalID string
	Command    *sandbox.Command
	Data       json.RawMessage
}
type Approval struct {
	ID         string
	Allow      bool
	Reason     string
	PolicyHash string
}
type AgentExecutor interface {
	ProbeVersionAndAuth(context.Context) (Probe, error)
	Start(context.Context, Request) (string, error)
	StreamEvents(context.Context, string, func(Event) error) error
	DecideApproval(context.Context, string, Approval) error
	DelegateIsolatedCommand(context.Context, string, sandbox.Command) (sandbox.CommandResult, error)
	ResumeIfSupported(context.Context, string) error
	Cancel(context.Context, string) error
}
