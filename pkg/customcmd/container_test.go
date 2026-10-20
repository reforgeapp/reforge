package customcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
)

type cappedWriter struct {
	mu        sync.Mutex
	data      []byte
	limit     int64
	truncated bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	left := int(w.limit) - len(w.data)
	if len(p) > left {
		p = p[:left]
		w.truncated = true
	}
	w.data = append(w.data, p...)
	return n, nil
}

type dockerLauncher struct{ image string }

var dockerSeq int64

func (d dockerLauncher) Launch(ctx context.Context, spec Spec) (sandbox.CommandResult, error) {
	runctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	name := fmt.Sprintf("rf-customcmd-%d-%d", os.Getpid(), atomic.AddInt64(&dockerSeq, 1))
	defer exec.Command("docker", "rm", "-f", name).Run()
	args := []string{"run", "--rm", "--name", name, "-i", "--network", "none", "--read-only", "--tmpfs", "/tmp:size=8m", "--user", "65534:65534", d.image, spec.Executable}
	args = append(args, spec.Args...)
	cmd := exec.CommandContext(runctx, "docker", args...)
	cmd.Stdin = bytes.NewReader(spec.Stdin)
	out := &cappedWriter{limit: spec.MaxOutput}
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	res := sandbox.CommandResult{Output: out.data, Truncated: out.truncated, TimedOut: errors.Is(runctx.Err(), context.DeadlineExceeded)}
	if err == nil {
		return res, nil
	}
	if errors.Is(runctx.Err(), context.Canceled) {
		return res, runctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		res.ExitCode = exit.ExitCode()
		return res, nil
	}
	return res, err
}

func dockerImageDigest(t *testing.T) string {
	t.Helper()
	if os.Getenv("REFORGE_TEST_DOCKER") != "1" {
		t.Skip("set REFORGE_TEST_DOCKER=1 to run the real container profile test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is unavailable")
	}
	out, err := exec.Command("docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", "alpine:3.21").Output()
	if err != nil {
		t.Skipf("alpine:3.21 digest unavailable: %v", err)
	}
	value := strings.TrimSpace(string(out))
	idx := strings.LastIndex(value, "@sha256:")
	if idx < 0 {
		t.Fatalf("unexpected digest reference %q", value)
	}
	return value[idx+1:]
}

func containerProfile(digest string) Profile {
	p := approvedProfile()
	p.ImageDigest = digest
	p.Executable = "/bin/busybox"
	p.Argv = []string{"echo", "{\"type\":\"result\",\"data\":{\"outcome\":\"success\"}}"}
	return p
}

func TestRealContainerProfileProtocol(t *testing.T) {
	digest := dockerImageDigest(t)
	launcher := dockerLauncher{image: "alpine@" + digest}
	executor := NewExecutor(launcher)

	t.Run("input output and unknown usage", func(t *testing.T) {
		p := containerProfile(digest)
		out, err := executor.Run(context.Background(), p, Input{Workspace: workspace(), Request: []byte("{\"prompt\":\"review\"}")})
		if err != nil {
			t.Fatal(err)
		}
		if out.State != "completed_unverified" || out.Usage.Known {
			t.Fatalf("unexpected result %+v", out)
		}
	})

	t.Run("malformed event is uncertain", func(t *testing.T) {
		p := containerProfile(digest)
		p.Argv = []string{"echo", "not-json"}
		out, err := executor.Run(context.Background(), p, Input{Workspace: workspace(), Request: []byte("{}")})
		if !errors.Is(err, ErrProtocol) || out.State != "unknown" {
			t.Fatalf("expected protocol uncertainty, got state=%q err=%v", out.State, err)
		}
	})

	t.Run("nonzero exit fails", func(t *testing.T) {
		p := containerProfile(digest)
		p.Executable = "/bin/sh"
		p.Argv = []string{"-c", "exit 3"}
		out, err := executor.Run(context.Background(), p, Input{Workspace: workspace(), Request: []byte("{}")})
		if err != nil || out.State != "failed" || out.ExitCode != 3 {
			t.Fatalf("expected failed exit 3, got state=%q code=%d err=%v", out.State, out.ExitCode, err)
		}
	})

	t.Run("wall clock budget", func(t *testing.T) {
		p := containerProfile(digest)
		p.Argv = []string{"sleep", "5"}
		p.MaxWallSeconds = 1
		out, err := executor.Run(context.Background(), p, Input{Workspace: workspace(), Request: []byte("{}")})
		if err != nil || out.State != "timed_out" {
			t.Fatalf("expected timed_out, got state=%q err=%v", out.State, err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		p := containerProfile(digest)
		p.Argv = []string{"sleep", "30"}
		ctx, cancel := context.WithCancel(context.Background())
		go func() { time.Sleep(500 * time.Millisecond); cancel() }()
		out, _ := executor.Run(ctx, p, Input{Workspace: workspace(), Request: []byte("{}")})
		if out.State != "cancelled" {
			t.Fatalf("expected cancelled, got %q", out.State)
		}
	})

	t.Run("secret isolation", func(t *testing.T) {
		t.Setenv("REFORGE_CONTAINER_SECRET", "leaked-value")
		p := containerProfile(digest)
		p.Argv = []string{"env"}
		out, _ := executor.Run(context.Background(), p, Input{Workspace: workspace(), Request: []byte("{}")})
		if bytes.Contains(out.Output, []byte("leaked-value")) {
			t.Fatal("container inherited a host secret")
		}
	})
}
