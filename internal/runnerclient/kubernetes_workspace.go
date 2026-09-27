package runnerclient

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"time"

	"reforge/internal/maintenance/repair"
	"reforge/internal/sandbox"
)

func (p commandPreparer) prepareKubernetes(ctx context.Context, request sandbox.WorkspaceRequest, patches []sandbox.Patch, command sandbox.Command) (sandbox.Workspace, error) {
	if p.cfg.Kubernetes == nil || len(command.Args) == 0 || request.Dependencies != "" || request.Egress != "" {
		return sandbox.Workspace{}, sandbox.ErrBoundary
	}
	request.Image = p.kubernetesImage(request.Image, path.Base(command.Args[0]))
	if request.Image == "" {
		return sandbox.Workspace{}, sandbox.ErrUnavailable
	}
	fetch := p.fetch
	if fetch == nil {
		fetch = p.cfg.Fetch
	}
	if fetch == nil {
		return sandbox.Workspace{}, sandbox.ErrBoundary
	}
	snapshot, err := fetch(ctx, request)
	if err != nil {
		return sandbox.Workspace{}, err
	}
	patched := applySnapshotPatches(snapshot, patches)
	bootstrap, needsEgress, err := kubernetesBootstrapCommand(patched, command)
	if err != nil {
		return sandbox.Workspace{}, err
	}
	request.Dependencies = ""
	request.Egress = ""
	if needsEgress {
		request.Egress = "registry"
	}
	workspace, err := p.runtime.PreparePinnedWorkspace(ctx, request)
	if err != nil {
		return sandbox.Workspace{}, err
	}
	destroy := func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = p.runtime.Destroy(cleanup, workspace)
	}
	if len(patches) > 0 {
		if err = p.runtime.ApplyPatch(ctx, workspace, patches); err != nil {
			destroy()
			return sandbox.Workspace{}, err
		}
	}
	if needsEgress {
		result, runErr := p.runtime.ExecuteBoundedCommand(ctx, workspace, bootstrap)
		if runErr != nil {
			destroy()
			return sandbox.Workspace{}, runErr
		}
		if result.ExitCode != 0 || result.TimedOut || result.Truncated {
			if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			destroy()
			return sandbox.Workspace{}, &sandbox.CommandSetupError{Result: result}
		}
	}
	return workspace, nil
}

func (p commandPreparer) kubernetesImage(fallback, executable string) string {
	toolchain := map[string]string{
		"go": "go", "node": "javascript", "npm": "javascript", "npx": "javascript",
		"python": "python", "python3": "python",
	}[executable]
	if toolchain == "" {
		return fallback
	}
	return p.cfg.Kubernetes.Toolchains[toolchain]
}

func kubernetesBootstrapCommand(snapshot sandbox.Snapshot, command sandbox.Command) (sandbox.Command, bool, error) {
	executable := path.Base(firstArg(command))
	switch executable {
	case "go":
		modules := goModuleDirectories(snapshot)
		if len(modules) == 0 {
			return sandbox.Command{}, false, nil
		}
		script := `set -eu; for module do (cd -- "$module" && /usr/local/go/bin/go mod download); done`
		args := []string{"/usr/bin/env", "GOMODCACHE=/tmp/gomod", "GOPROXY=https://proxy.golang.org", "GOSUMDB=sum.golang.org", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local", "/bin/sh", "-c", script, "reforge-bootstrap"}
		args = append(args, modules...)
		return sandbox.Command{Args: args, Directory: ".", Timeout: 10 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "egress"}, true, nil
	case "node", "npm", "npx":
		directory, ok := npmLockfileDirectory(snapshot, nil, command.Directory)
		if !ok {
			return sandbox.Command{}, false, nil
		}
		return sandbox.Command{Args: []string{"/usr/local/bin/node", npmCLI, "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--registry", npmRegistry}, Directory: directory, Timeout: 10 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "egress"}, true, nil
	default:
		return sandbox.Command{}, false, nil
	}
}

func goModuleDirectories(snapshot sandbox.Snapshot) []string {
	modules := make([]string, 0)
	for _, file := range snapshot.Files {
		if path.Base(file.Path) != "go.mod" {
			continue
		}
		modules = append(modules, path.Dir(file.Path))
	}
	sort.Strings(modules)
	return modules
}

func (u updater) updateKubernetes(ctx context.Context, files map[string][]byte, update repair.DependencyUpdate, paths []string) (map[string][]byte, error) {
	if u.cfg.Kubernetes == nil {
		return nil, sandbox.ErrBoundary
	}
	if _, ok := files[paths[0]]; !ok {
		return nil, errors.New(paths[0] + " not found")
	}
	binary := "usr/local/go/bin/go"
	args := []string{"/usr/bin/env", "GOMODCACHE=/tmp/gomod", "GOPROXY=https://proxy.golang.org", "GOSUMDB=sum.golang.org", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local", "/usr/local/go/bin/go", "get", update.Package + "@" + goVersion(update.Version)}
	if update.Ecosystem == "npm" {
		binary = "usr/local/bin/node"
		installArgs := []string{"install", update.Package + "@" + update.Version}
		switch update.Strategy {
		case "update":
			installArgs = []string{"update", update.Package}
		case "override":
			manifest, err := withOverride(files[paths[0]], update.Package, update.Version)
			if err != nil {
				return nil, err
			}
			files = copyFiles(files)
			files[paths[0]] = manifest
			installArgs = []string{"install"}
		}
		args = append([]string{"/usr/local/bin/node", npmCLI}, append(installArgs, "--package-lock-only", "--ignore-scripts", "--no-audit", "--no-fund", "--registry", npmRegistry)...)
	}
	image := u.image(binary)
	if image == "" {
		return nil, errors.New(update.Ecosystem + " tooling is not available on this runner")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	request := u.request
	request.Image, request.Egress, request.Dependencies, request.Timeout = image, "registry", "", 10*time.Minute
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
	result, err := u.runtime.ExecuteBoundedCommand(ctx, workspace, sandbox.Command{Args: args, Directory: path.Clean(update.Directory), Timeout: 5 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "egress"})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 || result.TimedOut || result.Truncated {
		tail := result.Output
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		return nil, fmt.Errorf("%s exited %d: %s", update.Ecosystem, result.ExitCode, tail)
	}
	out := map[string][]byte{}
	for _, name := range paths {
		artifact, err := u.runtime.CollectArtifact(ctx, workspace, name)
		if err != nil {
			return nil, err
		}
		out[name] = artifact.Data
	}
	return out, nil
}
