package customcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"reforge/internal/sandbox"
)

const maxInputBytes = 1 << 20

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
	spec := Spec{
		Workspace:  in.Workspace,
		Executable: profile.Executable,
		Args:       append([]string{}, profile.Argv...),
		Directory:  in.Workspace.Root,
		Stdin:      in.Request,
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
	for _, event := range events {
		if event.Type == "usage" {
			if err := json.Unmarshal(event.Data, &out.Usage); err != nil {
				out.State = "unknown"
				out.Reason = "Profile usage event is malformed"
				return out, ErrProtocol
			}
		}
	}
	out.Usage.Known = hasKnownUsage(out.Usage)
	if out.Truncated {
		out.State = "unknown"
		out.Reason = "Profile output exceeded its byte budget"
		return out, nil
	}
	out.State = "completed_unverified"
	out.Reason = "Exit 0 is not a validated repair; apply the declared validation before any publication"
	return out, nil
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
		case "progress", "log", "usage", "result", "error":
		default:
			return events, true
		}
		events = append(events, event)
	}
	return events, false
}
