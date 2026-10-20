package runnerclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
)

const sandboxUser = 65532

var goManifests = map[string]bool{"go.mod": true, "go.sum": true, "go.work": true, "go.work.sum": true}

func prefetch(ctx context.Context, cfg sandbox.RuntimeConfig, org, image string, snapshots ...sandbox.Snapshot) (string, error) {
	root, ok := cfg.Images[image]
	goBinary := filepath.Join(root, "usr/local/go/bin/go")
	if _, err := os.Stat(goBinary); !ok || err != nil || cfg.DependencyRoot == "" {
		return "", nil
	}
	cache := filepath.Join(cfg.DependencyRoot, org)
	if err := os.MkdirAll(cfg.DependencyRoot, 0755); err != nil {
		return "", err
	}
	for _, dir := range []string{cache, filepath.Join(cache, "go")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		if os.Getuid() == 0 {
			if err := os.Chown(dir, sandboxUser, sandboxUser); err != nil {
				return "", err
			}
		}
	}
	found := false
	for _, snapshot := range snapshots {
		work, err := os.MkdirTemp(cfg.DependencyRoot, "src-")
		if err != nil {
			return "", err
		}
		modules, err := writeManifests(work, snapshot)
		if err == nil && os.Getuid() == 0 {
			err = filepath.Walk(work, func(name string, _ os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				return os.Chown(name, sandboxUser, sandboxUser)
			})
		}
		for _, module := range modules {
			if err != nil {
				break
			}
			found = true
			err = download(ctx, root, goBinary, filepath.Join(work, module), filepath.Join(cache, "go"))
		}
		os.RemoveAll(work)
		if err != nil {
			return "", err
		}
	}
	if !found {
		return "", nil
	}
	return cache, nil
}

func writeManifests(work string, snapshot sandbox.Snapshot) ([]string, error) {
	var modules []string
	for _, file := range snapshot.Files {
		if !goManifests[path.Base(file.Path)] {
			continue
		}
		target := filepath.Join(work, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, file.Content, 0644); err != nil {
			return nil, err
		}
		if path.Base(file.Path) == "go.mod" {
			modules = append(modules, filepath.FromSlash(path.Dir(file.Path)))
		}
	}
	return modules, nil
}

func download(ctx context.Context, root, goBinary, dir, cache string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	home, err := os.MkdirTemp("", "reforge-go-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	if os.Getuid() == 0 {
		if err = os.Chown(home, sandboxUser, sandboxUser); err != nil {
			return err
		}
	}
	env := []string{"PATH=" + filepath.Dir(goBinary) + ":/usr/bin:/bin", "HOME=" + home, "GOROOT=" + filepath.Join(root, "usr/local/go"), "GOMODCACHE=" + cache, "GOCACHE=" + filepath.Join(home, "build"), "GOPATH=" + filepath.Join(home, "gopath"), "GOPROXY=https://proxy.golang.org", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local", "CGO_ENABLED=0"}
	if err = runTool(ctx, dir, env, goBinary, "mod", "download"); err != nil {
		return fmt.Errorf("go mod download failed in %s: %w", filepath.Base(dir), err)
	}
	return nil
}

func runTool(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if os.Getuid() == 0 {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: sandboxUser, Gid: sandboxUser}
	}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		tail := output.Bytes()
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		return errors.New(string(tail))
	}
	return nil
}
