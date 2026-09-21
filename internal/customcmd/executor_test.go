package customcmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"reforge/internal/sandbox"
)

type fakeLauncher struct {
	res  sandbox.CommandResult
	err  error
	last Spec
}

func (f *fakeLauncher) Launch(_ context.Context, spec Spec) (sandbox.CommandResult, error) {
	f.last = spec
	if f.err != nil {
		return sandbox.CommandResult{}, f.err
	}
	return f.res, nil
}

func approvedProfile() Profile {
	at := time.Now().Add(-time.Hour)
	return Profile{
		ID:               "00000000-0000-4000-8000-000000000001",
		Name:             "reviewer",
		Version:          1,
		ImageDigest:      "sha256:" + strings.Repeat("a", 64),
		Executable:       "/bin/review",
		Argv:             []string{"--mode", "review"},
		ProtocolVersion:  ProtocolVersion,
		MaxWallSeconds:   30,
		MaxOutputBytes:   1 << 20,
		MaxTurns:         4,
		Concurrency:      1,
		ApprovalEvidence: "ticket-1",
		ApprovedBy:       "00000000-0000-4000-8000-0000000000aa",
		ApprovedAt:       &at,
	}
}

func workspace() sandbox.Workspace {
	return sandbox.Workspace{ID: "rf-test", Root: "/workspace", Image: "reviewer"}
}

func TestExitZeroIsNotVerified(t *testing.T) {
	fake := &fakeLauncher{res: sandbox.CommandResult{ExitCode: 0, Output: []byte("{\"type\":\"result\",\"message\":\"ok\"}\n")}}
	out, err := NewExecutor(fake).Run(context.Background(), approvedProfile(), Input{Workspace: workspace(), Request: []byte("{}")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.State != "completed_unverified" {
		t.Fatalf("exit 0 must not be verified, got %q", out.State)
	}
	if out.Usage.Known {
		t.Fatal("absent usage must remain unknown")
	}
	if len(out.Events) != 1 {
		t.Fatalf("expected one parsed event, got %d", len(out.Events))
	}
}

func TestKnownUsageIsRecorded(t *testing.T) {
	fake := &fakeLauncher{res: sandbox.CommandResult{Output: []byte("{\"type\":\"usage\",\"data\":{\"tokens\":120,\"milliseconds\":40}}\n")}}
	out, err := NewExecutor(fake).Run(context.Background(), approvedProfile(), Input{Workspace: workspace(), Request: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Usage.Known || out.Usage.Tokens != 120 || out.Usage.Milliseconds != 40 {
		t.Fatalf("usage not recorded: %+v", out.Usage)
	}
}

func TestNonzeroExitFails(t *testing.T) {
	fake := &fakeLauncher{res: sandbox.CommandResult{ExitCode: 2, Output: []byte("{\"type\":\"error\"}\n")}}
	out, err := NewExecutor(fake).Run(context.Background(), approvedProfile(), Input{Workspace: workspace(), Request: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "failed" {
		t.Fatalf("expected failed, got %q", out.State)
	}
}

func TestMalformedAndUnknownEventsAreUncertain(t *testing.T) {
	for _, payload := range []string{"not-json\n", "{\"type\":\"surprise\"}\n", "{\"message\":\"no type\"}\n"} {
		fake := &fakeLauncher{res: sandbox.CommandResult{Output: []byte(payload)}}
		out, err := NewExecutor(fake).Run(context.Background(), approvedProfile(), Input{Workspace: workspace(), Request: []byte("{}")})
		if !errors.Is(err, ErrProtocol) || out.State != "unknown" {
			t.Fatalf("payload %q: expected protocol uncertainty, got state=%q err=%v", payload, out.State, err)
		}
	}
}

func TestTimeoutAndTruncationAreUncertain(t *testing.T) {
	fake := &fakeLauncher{res: sandbox.CommandResult{TimedOut: true, Output: []byte("{\"type\":\"log\"}\n")}}
	out, _ := NewExecutor(fake).Run(context.Background(), approvedProfile(), Input{Workspace: workspace(), Request: []byte("{}")})
	if out.State != "timed_out" {
		t.Fatalf("expected timed_out, got %q", out.State)
	}
	fake = &fakeLauncher{res: sandbox.CommandResult{Truncated: true, Output: []byte("{\"type\":\"log\"}\n")}}
	out, _ = NewExecutor(fake).Run(context.Background(), approvedProfile(), Input{Workspace: workspace(), Request: []byte("{}")})
	if out.State != "unknown" {
		t.Fatalf("expected unknown on truncation, got %q", out.State)
	}
}

func TestUnapprovedAndRevokedAreBlocked(t *testing.T) {
	draft := approvedProfile()
	draft.ApprovedAt, draft.ApprovedBy = nil, ""
	if _, err := NewExecutor(&fakeLauncher{}).Run(context.Background(), draft, Input{Workspace: workspace(), Request: []byte("{}")}); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("draft profile must be blocked, got %v", err)
	}
	revoked := approvedProfile()
	now := time.Now()
	revoked.RevokedAt = &now
	out, err := NewExecutor(&fakeLauncher{}).Run(context.Background(), revoked, Input{Workspace: workspace(), Request: []byte("{}")})
	if !errors.Is(err, ErrNotApproved) || out.State != "blocked" {
		t.Fatalf("revoked profile must be blocked, got state=%q err=%v", out.State, err)
	}
}

func TestCancellationIsReported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeLauncher{err: context.Canceled}
	out, _ := NewExecutor(fake).Run(ctx, approvedProfile(), Input{Workspace: workspace(), Request: []byte("{}")})
	if out.State != "cancelled" {
		t.Fatalf("expected cancelled, got %q", out.State)
	}
}

func TestValidationRejectsUnsafeProfiles(t *testing.T) {
	cases := map[string]func(*Profile){
		"tag digest":        func(p *Profile) { p.ImageDigest = "alpine:3.21" },
		"relative exe":      func(p *Profile) { p.Executable = "review" },
		"shell metachar":    func(p *Profile) { p.Argv = []string{"--x", "a;rm -rf /"} },
		"interpolation":     func(p *Profile) { p.Argv = []string{"$HOME"} },
		"bad protocol":      func(p *Profile) { p.ProtocolVersion = 2 },
		"wall clock":        func(p *Profile) { p.MaxWallSeconds = 0 },
		"output budget":     func(p *Profile) { p.MaxOutputBytes = 0 },
		"turn budget":       func(p *Profile) { p.MaxTurns = 0 },
		"concurrency limit": func(p *Profile) { p.Concurrency = 0 },
	}
	for name, mutate := range cases {
		p := approvedProfile()
		mutate(&p)
		if err := Validate(p); err == nil {
			t.Fatalf("%s: expected validation failure", name)
		}
	}
}

func TestLauncherReceivesTypedArgvWithoutShell(t *testing.T) {
	fake := &fakeLauncher{res: sandbox.CommandResult{Output: []byte("{\"type\":\"result\"}\n")}}
	if _, err := NewExecutor(fake).Run(context.Background(), approvedProfile(), Input{Workspace: workspace(), Request: []byte("{\"prompt\":\"x\"}")}); err != nil {
		t.Fatal(err)
	}
	if fake.last.Executable != "/bin/review" {
		t.Fatalf("executable must be separate from argv: %q", fake.last.Executable)
	}
	if strings.Join(fake.last.Args, " ") != "--mode review" {
		t.Fatalf("unexpected argv: %v", fake.last.Args)
	}
	if string(fake.last.Stdin) != "{\"prompt\":\"x\"}" {
		t.Fatalf("stdin not forwarded: %q", fake.last.Stdin)
	}
}
