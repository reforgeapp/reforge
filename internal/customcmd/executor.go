package customcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"reforge/internal/sandbox"
	"reforge/internal/skills"
)

const maxInputBytes = 1 << 20

func (e *Executor) RunSpec(ctx context.Context, spec ProfileSpec, in Input) (Result, error) {
	if e == nil || e.launcher == nil {
		return Result{}, ErrInvalid
	}
	if err := validateSpec(spec); err != nil {
		return Result{}, err
	}
	if len(in.Request) > maxInputBytes || in.Workspace.ID == "" {
		return Result{}, ErrInvalid
	}
	request, err := requestWithSkills(in.Request)
	if err != nil {
		return Result{}, err
	}
	spec2 := Spec{
		Workspace:  in.Workspace,
		Executable: spec.Executable,
		Args:       append([]string{}, spec.Argv...),
		Directory:  in.Workspace.Root,
		Stdin:      request,
		Timeout:    time.Duration(spec.MaxWallSeconds) * time.Second,
		MaxOutput:  spec.MaxOutputBytes,
	}
	raw, err := e.launcher.Launch(ctx, spec2)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Result{State: "cancelled", Reason: "Run was cancelled before the profile confirmed completion"}, nil
		}
		return Result{State: "unknown", Reason: "Profile process outcome could not be confirmed; reconcile before retry"}, err
	}
	return e.classify(raw, Profile{MaxTurns: spec.MaxTurns})
}

func (e *Executor) Run(ctx context.Context, profile Profile, in Input) (Result, error) {
	if e == nil || e.launcher == nil {
		return Result{}, ErrInvalid
	}
	if err := Validate(profile); err != nil {
		return Result{}, err
	}
	if !Approved(profile, time.Now()) {
		return Result{State: "blocked", Reason: "Profile is not approved or has been revoked"}, ErrNotApproved
	}
	if len(in.Request) > maxInputBytes || in.Workspace.ID == "" {
		return Result{}, ErrInvalid
	}
	request, err := requestWithSkills(in.Request)
	if err != nil {
		return Result{}, err
	}
	spec := Spec{
		Workspace:  in.Workspace,
		Executable: profile.Executable,
		Args:       append([]string{}, profile.Argv...),
		Directory:  in.Workspace.Root,
		Stdin:      request,
		Timeout:    time.Duration(profile.MaxWallSeconds) * time.Second,
		MaxOutput:  profile.MaxOutputBytes,
	}
	raw, err := e.launcher.Launch(ctx, spec)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Result{State: "cancelled", Reason: "Run was cancelled before the profile confirmed completion"}, nil
		}
		return Result{State: "unknown", Reason: "Profile process outcome could not be confirmed; reconcile before retry"}, err
	}
	return e.classify(raw, profile)
}

func requestWithSkills(raw []byte) ([]byte, error) {
	var request map[string]json.RawMessage
	if len(raw) > maxInputBytes || json.Unmarshal(raw, &request) != nil || request == nil {
		return nil, ErrInvalid
	}
	bundle, err := skills.Context()
	if err != nil {
		return nil, err
	}
	context, err := json.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	request["reforge_skills"] = context
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > maxInputBytes {
		return nil, ErrInvalid
	}
	return encoded, nil
}

func (e *Executor) classify(raw sandbox.CommandResult, profile Profile) (Result, error) {
	out := Result{Output: raw.Output, ExitCode: raw.ExitCode, TimedOut: raw.TimedOut, Truncated: raw.Truncated, Usage: Usage{Known: false}}
	if raw.TimedOut {
		out.State = "timed_out"
		out.Reason = "Profile exceeded its wall-clock budget"
		out.Events = parseEvents(raw.Output)
		return out, nil
	}
	events, malformed := parseEventsChecked(raw.Output)
	out.Events = events
	if malformed {
		out.State = "unknown"
		out.Reason = "Profile emitted a malformed or unknown event; reconcile before retry"
		return out, ErrProtocol
	}
	if raw.ExitCode != 0 {
		out.State = "failed"
		out.Reason = "Profile exited nonzero"
		return out, nil
	}
	turns := 0
	resultCount := 0
	resultIndex := -1
	resultSuccess := false
	resultFailed := false
	resultMalformed := false
	hasError := false
	for index, event := range events {
		switch event.Type {
		case "usage":
			if err := json.Unmarshal(event.Data, &out.Usage); err != nil {
				out.State = "unknown"
				out.Reason = "Profile usage event is malformed"
				return out, ErrProtocol
			}
		case "turn":
			turns++
		case "error":
			hasError = true
		case "result":
			resultCount++
			resultIndex = index
			resultSuccess, resultFailed, resultMalformed = parseResultOutcome(event)
		}
	}
	if profile.MaxTurns > 0 && turns > profile.MaxTurns {
		out.State = "unknown"
		out.Reason = "Profile exceeded its declared turn budget; reconcile before retry"
		return out, ErrProtocol
	}
	out.Usage.Known = hasKnownUsage(out.Usage)
	if out.Truncated {
		out.State = "unknown"
		out.Reason = "Profile output exceeded its byte budget"
		return out, nil
	}
	if hasError {
		out.State = "failed"
		out.Reason = "Profile emitted an error event"
		return out, nil
	}
	if resultCount != 1 || resultIndex != len(events)-1 {
		out.State = "unknown"
		out.Reason = "Profile must emit exactly one terminal result as its final event"
		return out, ErrProtocol
	}
	if resultMalformed {
		out.State = "unknown"
		out.Reason = "Profile terminal result is malformed"
		return out, ErrProtocol
	}
	if resultFailed || !resultSuccess {
		out.State = "failed"
		out.Reason = "Profile terminal result did not report success"
		return out, nil
	}
	out.State = "completed_unverified"
	out.Reason = "Exit 0 is not a validated repair; apply the declared validation before any publication"
	return out, nil
}

func parseResultOutcome(event Event) (bool, bool, bool) {
	if len(event.Data) == 0 {
		return false, false, true
	}
	var result struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(event.Data, &result); err != nil || result.Outcome == "" {
		return false, false, true
	}
	switch result.Outcome {
	case "success":
		return true, false, false
	case "failed", "failure", "error":
		return false, true, false
	default:
		return false, false, true
	}
}

func hasKnownUsage(u Usage) bool {
	return u.Tokens != 0 || u.MicroUSD != 0 || u.Milliseconds != 0 || u.Requests != 0
}

func parseEvents(output []byte) []Event {
	events, _ := parseEventsChecked(output)
	return events
}

func parseEventsChecked(output []byte) ([]Event, bool) {
	events := []Event{}
	for _, line := range bytes.Split(output, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil || event.Type == "" {
			return events, true
		}
		switch event.Type {
		case "progress", "log", "turn", "usage", "result", "error":
		default:
			return events, true
		}
		events = append(events, event)
	}
	return events, false
}
