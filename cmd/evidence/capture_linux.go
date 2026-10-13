package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
)

type captureOutput struct {
	mu        sync.Mutex
	body      []byte
	limit     int
	truncated bool
}

func (o *captureOutput) Write(body []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := len(body)
	if len(body) > o.limit-len(o.body) {
		body = body[:o.limit-len(o.body)]
		o.truncated = true
	}
	o.body = append(o.body, body...)
	return n, nil
}

func capture(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("capture", flag.ContinueOnError)
	timeout := flags.Duration("timeout", 10*time.Minute, "CI command time limit")
	limit := flags.Int("max-output-bytes", 1<<20, "Captured output limit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 || *timeout < time.Millisecond || *timeout > time.Hour || *limit < 1 || *limit > 1<<20 {
		return errors.New("bounded CI command required after --")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, flags.Arg(0), flags.Args()[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	output := &captureOutput{limit: *limit}
	cmd.Stdout, cmd.Stderr = output, output
	started := time.Now()
	err := cmd.Run()
	result := sandbox.CommandResult{Output: output.body, Duration: time.Since(started), Truncated: output.truncated, TimedOut: ctx.Err() != nil}
	if err != nil {
		result.ExitCode = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.ExitCode = exit.ExitCode()
		}
	}
	if encodeErr := json.NewEncoder(out).Encode(result); encodeErr != nil {
		return encodeErr
	}
	if err != nil {
		return errors.New("CI command failed; captured result records failure")
	}
	if result.Truncated {
		return errors.New("CI output exceeded capture limit")
	}
	return nil
}
