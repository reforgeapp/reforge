package runnerclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

const (
	maxGeneratedFiles = 8000
	maxGeneratedBytes = 128 << 20
)

var interpreters = map[string]string{"bash": "/bin/bash", "sh": "/bin/sh", "python": "/usr/local/bin/python3", "python3": "/usr/local/bin/python3", "node": "/usr/local/bin/node"}

func scriptInterpreter(body []byte) string {
	line, _, _ := bytes.Cut(body, []byte("\n"))
	if !bytes.HasPrefix(line, []byte("#!")) {
		return "/bin/bash"
	}
	fields := strings.Fields(string(line[2:]))
	if len(fields) > 1 && path.Base(fields[0]) == "env" {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	return interpreters[path.Base(fields[0])]
}

func (u updater) regenerate(ctx context.Context, files map[string][]byte, g repair.Regeneration) (map[string][]byte, error) {
	if !g.Valid() {
		return nil, errors.New("invalid regeneration")
	}
	if u.cfg.Backend != "kubernetes" || u.cfg.Kubernetes == nil {
		return nil, errors.New("regeneration is not available on this runner")
	}
	image := u.cfg.Kubernetes.Toolchains["maintenance"]
	if image == "" {
		return nil, errors.New("regeneration needs the maintenance workspace image on this runner")
	}
	if _, ok := files[g.Script]; !ok {
		return nil, errors.New(g.Script + " not found")
	}
	interpreter := scriptInterpreter(files[g.Script])
	if interpreter == "" {
		return nil, errors.New(g.Script + " shebang names an interpreter this workspace lacks; use bash, sh, python3 or node")
	}
	ctx, cancel := context.WithTimeout(ctx, 70*time.Minute)
	defer cancel()
	request := u.request
	request.Image, request.Egress, request.Dependencies, request.Timeout = image, "registry", "", 70*time.Minute
	workspace, err := u.runtime.PreparePinnedWorkspace(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = u.runtime.Destroy(cleanup, workspace)
	}()
	if patches := changed(u.target, files); len(patches) > 0 {
		if err = u.runtime.ApplyPatch(ctx, workspace, patches); err != nil {
			return nil, err
		}
	}
	args := append([]string{"/usr/bin/env", "GOMODCACHE=/tmp/gomod", "GOPROXY=https://proxy.golang.org", "GOSUMDB=sum.golang.org", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local", interpreter, g.Script}, g.Args...)
	result, err := u.runtime.ExecuteBoundedCommand(ctx, workspace, sandbox.Command{Args: args, Directory: ".", Timeout: time.Hour, MaxOutputBytes: 64 << 10, NetworkProfile: "egress"})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 || result.TimedOut {
		tail := result.Output
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		return nil, fmt.Errorf("%s exited %d (timed out: %v): %s", g.Script, result.ExitCode, result.TimedOut, tail)
	}
	list := append([]string{"/bin/sh", "-c", `for p do [ ! -e "$p" ] || find "$p" -type f; done`, "reforge-outputs"}, g.Outputs...)
	listing, err := u.runtime.ExecuteBoundedCommand(ctx, workspace, sandbox.Command{Args: list, Directory: ".", Timeout: time.Minute, MaxOutputBytes: 1 << 20, NetworkProfile: "none"})
	if err != nil {
		return nil, err
	}
	if listing.ExitCode != 0 || listing.Truncated {
		return nil, errors.New("generated outputs could not be listed")
	}
	out := map[string][]byte{}
	total := 0
	for _, name := range strings.Split(strings.TrimSpace(string(listing.Output)), "\n") {
		if name == "" {
			continue
		}
		if len(out) == maxGeneratedFiles {
			return nil, fmt.Errorf("outputs exceed %d files", maxGeneratedFiles)
		}
		artifact, err := u.runtime.CollectArtifact(ctx, workspace, name)
		if err != nil {
			return nil, err
		}
		if total += len(artifact.Data); total > maxGeneratedBytes {
			return nil, fmt.Errorf("outputs exceed %d bytes", maxGeneratedBytes)
		}
		out[name] = artifact.Data
	}
	return out, nil
}
