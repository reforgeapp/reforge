package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type CommandConfig struct {
	Executable     string
	Args           []string
	ExpectedSHA256 string
}

func NewCommandLauncher(config CommandConfig) Launcher {
	if !filepath.IsAbs(config.Executable) {
		return nil
	}
	args := append([]string(nil), config.Args...)
	expected := strings.ToLower(config.ExpectedSHA256)
	return func(ctx context.Context, binding Binding) (Runtime, error) {
		if expected != "" {
			sum, err := fileSHA256(config.Executable)
			if err != nil || sum != expected {
				return Runtime{}, ErrDisabled
			}
		}
		command := exec.CommandContext(ctx, config.Executable, args...)
		command.Env = []string{"PATH=/usr/bin:/bin"}
		process, err := StartPreparedRuntime(command)
		if err != nil {
			return Runtime{}, err
		}
		return Runtime{Transport: process, Binding: binding}, nil
	}
}

type ContainerConfig struct {
	Docker      string
	Image       string
	Executable  string
	Args        []string
	Environment []string
}

func NewContainerLauncher(config ContainerConfig) Launcher {
	docker := config.Docker
	if docker == "" {
		docker = "docker"
	}
	if !pinnedImage(config.Image) || config.Executable == "" || !strings.HasPrefix(config.Executable, "/") {
		return nil
	}
	args := []string{"run", "--rm", "-i", "--network", "none", "--read-only", "--tmpfs", "/tmp:size=64m", "--user", "65534:65534"}
	for _, env := range config.Environment {
		if !validEnv(env) {
			return nil
		}
		args = append(args, "--env", env)
	}
	args = append(args, config.Image, config.Executable)
	args = append(args, config.Args...)
	return func(ctx context.Context, binding Binding) (Runtime, error) {
		command := exec.CommandContext(ctx, docker, args...)
		command.Env = []string{"PATH=/usr/bin:/bin"}
		process, err := StartPreparedRuntime(command)
		if err != nil {
			return Runtime{}, err
		}
		return Runtime{Transport: process, Binding: binding}, nil
	}
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, io.LimitReader(file, 1<<30)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func pinnedImage(value string) bool {
	at := strings.LastIndex(value, "@sha256:")
	if at <= 0 || len(value)-at-8 != 64 {
		return false
	}
	for _, c := range value[at+8:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return !strings.ContainsAny(value[:at], " \t\n;|&`$<>")
}

func validEnv(value string) bool {
	key, _, ok := strings.Cut(value, "=")
	if !ok || key == "" || strings.ContainsAny(value, " \t\n;|&`$<>") {
		return false
	}
	for _, secret := range []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "CREDENTIAL"} {
		if strings.Contains(strings.ToUpper(key), secret) {
			return false
		}
	}
	return true
}

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
