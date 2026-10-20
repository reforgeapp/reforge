package customcmd

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
)

const ProtocolVersion = 1

var (
	ErrInvalid     = errors.New("custom command profile is invalid")
	ErrNotApproved = errors.New("custom command profile is not approved")
	ErrProtocol    = errors.New("custom command protocol violation")
	ErrCapacity    = errors.New("custom command profile concurrency exhausted")
	ErrNoProfile   = errors.New("task is not bound to a custom command profile")
)

type ProfileSpec struct {
	ID              string   `json:"id"`
	Version         int64    `json:"version"`
	ImageDigest     string   `json:"image_digest"`
	Executable      string   `json:"executable"`
	Argv            []string `json:"argv"`
	ProtocolVersion int      `json:"protocol_version"`
	MaxWallSeconds  int      `json:"max_wall_seconds"`
	MaxOutputBytes  int64    `json:"max_output_bytes"`
	MaxTurns        int      `json:"max_turns"`
	Concurrency     int      `json:"concurrency"`
}

func SpecFromProfile(p Profile) ProfileSpec {
	return ProfileSpec{ID: p.ID, Version: p.Version, ImageDigest: p.ImageDigest, Executable: p.Executable, Argv: append([]string(nil), p.Argv...), ProtocolVersion: p.ProtocolVersion, MaxWallSeconds: p.MaxWallSeconds, MaxOutputBytes: p.MaxOutputBytes, MaxTurns: p.MaxTurns, Concurrency: p.Concurrency}
}

type Profile struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Version          int64      `json:"version"`
	ImageDigest      string     `json:"image_digest"`
	Executable       string     `json:"executable"`
	Argv             []string   `json:"argv"`
	ProtocolVersion  int        `json:"protocol_version"`
	MaxWallSeconds   int        `json:"max_wall_seconds"`
	MaxOutputBytes   int64      `json:"max_output_bytes"`
	MaxTurns         int        `json:"max_turns"`
	Concurrency      int        `json:"concurrency"`
	ApprovalEvidence string     `json:"approval_evidence"`
	ApprovedBy       string     `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

func (p Profile) State() string {
	if p.RevokedAt != nil {
		return "revoked"
	}
	if p.ApprovedAt != nil && p.ApprovedBy != "" {
		return "approved"
	}
	return "draft"
}

type Event struct {
	Type    string          `json:"type"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Usage struct {
	Known        bool  `json:"known"`
	Tokens       int64 `json:"tokens,omitempty"`
	MicroUSD     int64 `json:"micro_usd,omitempty"`
	Milliseconds int64 `json:"milliseconds,omitempty"`
	Requests     int64 `json:"requests,omitempty"`
}

type Result struct {
	State     string  `json:"state"`
	Reason    string  `json:"reason"`
	Output    []byte  `json:"-"`
	Events    []Event `json:"events"`
	Usage     Usage   `json:"usage"`
	Truncated bool    `json:"truncated"`
	ExitCode  int     `json:"exit_code"`
	TimedOut  bool    `json:"timed_out"`
}

type Input struct {
	JobID      string
	AttemptID  string
	Workspace  sandbox.Workspace
	Request    []byte
	PolicyHash string
}

type Spec struct {
	Workspace  sandbox.Workspace
	Executable string
	Args       []string
	Directory  string
	Stdin      []byte
	Timeout    time.Duration
	MaxOutput  int64
}

type Launcher interface {
	Launch(context.Context, Spec) (sandbox.CommandResult, error)
}

type Executor struct {
	launcher Launcher
}

func NewExecutor(launcher Launcher) *Executor { return &Executor{launcher: launcher} }
