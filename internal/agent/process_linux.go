package agent

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type StdioProcess struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  io.ReadCloser
	once    sync.Once
	done    chan struct{}
}

func StartPreparedRuntime(command *exec.Cmd) (*StdioProcess, error) {
	if command == nil || !filepath.IsAbs(command.Path) || command.Env == nil || command.Stdin != nil || command.Stdout != nil || command.Stderr != nil {
		return nil, ErrDisabled
	}
	for _, entry := range command.Env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || (key != "PATH" && key != "LANG" && key != "LC_ALL" && key != "TZ") {
			return nil, ErrDisabled
		}
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
	command.WaitDelay = time.Second
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, err
	}
	if err = command.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		_ = stderr.Close()
		return nil, errors.New("official runtime could not start")
	}
	process := &StdioProcess{command: command, input: input, output: output, done: make(chan struct{})}
	go func() {
		count, _ := io.Copy(io.Discard, io.LimitReader(stderr, 256<<10+1))
		if count > 256<<10 {
			_ = process.Close()
		}
	}()
	go func() { _ = command.Wait(); close(process.done) }()
	return process, nil
}
func (p *StdioProcess) Read(b []byte) (int, error)  { return p.output.Read(b) }
func (p *StdioProcess) Write(b []byte) (int, error) { return p.input.Write(b) }
func (p *StdioProcess) Close() error {
	p.once.Do(func() {
		_ = p.input.Close()
		_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
		_ = p.output.Close()
	})
	select {
	case <-p.done:
		return nil
	case <-time.After(2 * time.Second):
		return ErrUncertain
	}
}
