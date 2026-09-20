package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type boundedOutput struct {
	mu        sync.Mutex
	data      []byte
	limit     int64
	truncated bool
}

func (w *boundedOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(data)
	left := int(w.limit) - len(w.data)
	if len(data) > left {
		data = data[:left]
		w.truncated = true
	}
	w.data = append(w.data, data...)
	return n, nil
}

func (r *Runtime) invoke(ctx context.Context, w *workspaceState, input []byte, limit int64, confined bool, args ...string) (CommandResult, error) {
	cmd := r.command(ctx, w, confined, args...)
	cmd.Stdin = bytes.NewReader(input)
	output := &boundedOutput{limit: limit}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	result := CommandResult{Output: output.data, Truncated: output.truncated}
	if err == nil {
		return result, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return result, ErrUnavailable
}

func (r *Runtime) command(ctx context.Context, w *workspaceState, confined bool, args ...string) *exec.Cmd {
	flags := []string{"--root=" + filepath.Join(r.config.StateRoot, "runsc"), "--network=none", "--platform=systrap", "--ignore-cgroups", "--file-access=exclusive"}
	if r.config.Rootless && len(args) > 0 && args[0] == "run" {
		flags = append(flags, "--rootless")
	}
	cmd := exec.CommandContext(ctx, r.config.Runsc, append(flags, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if confined && w.cgroup != nil {
		cmd.SysProcAttr.UseCgroupFD = true
		cmd.SysProcAttr.CgroupFD = int(w.cgroup.Fd())
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	return cmd
}
func (r *Runtime) start(ctx context.Context, w *workspaceState) error {
	w.processMu.Lock()
	if w.stopped || ctx.Err() != nil {
		w.processMu.Unlock()
		return ErrUnavailable
	}
	w.output = &boundedOutput{limit: 65536}
	w.process = r.command(ctx, w, true, "run", "--bundle", w.bundle, w.workspace.ID)
	w.process.Stdout = w.output
	w.process.Stderr = w.output
	if e := w.process.Start(); e != nil {
		w.processMu.Unlock()
		return ErrUnavailable
	}
	w.done = make(chan struct{})
	go func() { _ = w.process.Wait(); close(w.done); w.cancel() }()
	w.processMu.Unlock()
	ready, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			w.output.mu.Lock()
			message := string(w.output.data)
			w.output.mu.Unlock()
			return fmt.Errorf("sandbox startup: %w: %s", ErrUnavailable, message)
		case <-ready.Done():
			return ErrUnavailable
		case <-ticker.C:
			result, e := r.invoke(ready, w, nil, 65536, false, "state", w.workspace.ID)
			var state struct {
				Status string `json:"status"`
			}
			if e == nil && result.ExitCode == 0 && json.Unmarshal(result.Output, &state) == nil && state.Status == "running" {
				return nil
			}
		}
	}
}

func (r *Runtime) setupCgroup(w *workspaceState) error {
	if r.config.CgroupRoot == "" && r.config.Development {
		return nil
	}
	var fs unix.Statfs_t
	if err := unix.Statfs(r.config.CgroupRoot, &fs); err != nil || fs.Type != unix.CGROUP2_SUPER_MAGIC {
		return ErrUnavailable
	}
	info, err := os.Lstat(r.config.CgroupRoot)
	if err != nil || !info.IsDir() {
		return ErrUnavailable
	}
	group := filepath.Join(r.config.CgroupRoot, w.workspace.ID)
	if err = os.Mkdir(group, 0700); err != nil {
		return ErrUnavailable
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(group)
		}
	}()
	for _, setting := range []struct{ name, value string }{
		{"memory.max", strconv.FormatInt(r.config.MemoryBytes, 10)},
		{"memory.swap.max", "0"},
		{"memory.oom.group", "1"},
		{"pids.max", strconv.FormatInt(r.config.MaxProcesses, 10)},
		{"cpu.max", fmt.Sprintf("%d 100000", r.config.CPUs*100000)},
	} {
		if err = os.WriteFile(filepath.Join(group, setting.name), []byte(setting.value), 0600); err != nil {
			return ErrUnavailable
		}
		observed, err := os.ReadFile(filepath.Join(group, setting.name))
		if err != nil || strings.TrimSpace(string(observed)) != setting.value {
			return ErrUnavailable
		}
	}
	file, err := os.Open(group)
	if err != nil {
		return ErrUnavailable
	}
	w.cgroup = file
	w.cgroupPath = group
	ok = true
	return nil
}

func (r *Runtime) stop(w *workspaceState) {
	w.stopOnce.Do(func() {
		w.processMu.Lock()
		w.stopped = true
		process, done := w.process, w.done
		w.processMu.Unlock()
		if w.cgroupPath != "" {
			_ = os.WriteFile(filepath.Join(w.cgroupPath, "cgroup.kill"), []byte("1"), 0600)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = r.invoke(ctx, w, nil, 65536, false, "kill", "--all", w.workspace.ID, "KILL")
		if process != nil && process.Process != nil {
			select {
			case <-done:
			default:
				_ = process.Process.Kill()
			}
		}
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
			}
		}
	})
}

func (r *Runtime) cleanOrphans() error {
	root := filepath.Join(r.config.StateRoot, "runsc")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return ErrBoundary
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := r.invoke(ctx, &workspaceState{}, nil, 1<<20, false, "list", "--format=json")
	if err != nil || result.ExitCode != 0 || result.Truncated {
		return ErrUnavailable
	}
	var rows []struct {
		ID     string `json:"id"`
		Bundle string `json:"bundle"`
	}
	if json.Unmarshal(result.Output, &rows) != nil || len(rows) > 1000 {
		return ErrBoundary
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.ID, "rf-") || len(row.ID) != 39 || strings.ContainsAny(row.ID, "/\\") || filepath.Dir(row.Bundle) != r.config.StateRoot || !strings.HasPrefix(filepath.Base(row.Bundle), "bundle-") {
			return ErrBoundary
		}
		w := &workspaceState{workspace: Workspace{ID: row.ID}, bundle: row.Bundle}
		if r.config.CgroupRoot != "" {
			w.cgroupPath = filepath.Join(r.config.CgroupRoot, row.ID)
		}
		r.stop(w)
		result, err = r.invoke(ctx, w, nil, 65536, false, "delete", "--force", row.ID)
		if err != nil || result.ExitCode != 0 {
			return ErrUnavailable
		}
		if w.cgroupPath != "" {
			if err = os.Remove(w.cgroupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err = os.RemoveAll(row.Bundle); err != nil {
			return err
		}
	}
	return nil
}
