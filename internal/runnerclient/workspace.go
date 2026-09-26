package runnerclient

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reforge/internal/egress"
	"reforge/internal/sandbox"
	"reforge/internal/sandbox/guest"
)

type commandPreparer struct {
	cfg     sandbox.RuntimeConfig
	runtime sandbox.SandboxRuntime
	org     string
	fetch   func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error)
}

func (p commandPreparer) prepare(ctx context.Context, request sandbox.WorkspaceRequest, patches []sandbox.Patch, command sandbox.Command) (sandbox.Workspace, error) {
	image := request.Image
	if len(command.Args) > 0 {
		binary := map[string]string{
			"go":      "usr/local/go/bin/go",
			"node":    "usr/local/bin/node",
			"npm":     "usr/local/bin/node",
			"npx":     "usr/local/bin/node",
			"python":  "usr/local/bin/python3",
			"python3": "usr/local/bin/python3",
		}[path.Base(command.Args[0])]
		if binary != "" {
			if candidate := (updater{cfg: p.cfg}).image(binary); candidate != "" {
				image = candidate
			}
		}
	}
	request.Image = image

	fetch := p.fetch
	if fetch == nil {
		fetch = p.cfg.Fetch
	}
	var snapshot sandbox.Snapshot
	var haveSnapshot bool
	if fetch != nil {
		var err error
		snapshot, err = fetch(ctx, request)
		if err != nil {
			return sandbox.Workspace{}, err
		}
		haveSnapshot = true
	}

	if path.Base(firstArg(command)) == "go" && haveSnapshot {
		patched := applySnapshotPatches(snapshot, patches)
		dependencies, err := prefetch(ctx, p.cfg, p.org, image, patched)
		if err != nil {
			return sandbox.Workspace{}, err
		}
		request.Dependencies = dependencies
	}

	installDir, hasLock := npmLockfileDirectory(snapshot, patches, command.Directory)
	install := haveSnapshot && path.Base(firstArg(command)) == "node" && hasLock
	var proxy *egress.Proxy
	var egressDir string
	if install {
		root := filepath.Join(p.cfg.StateRoot, "egress")
		if err := os.MkdirAll(root, 0755); err != nil {
			return sandbox.Workspace{}, err
		}
		var err error
		egressDir, err = os.MkdirTemp(root, "workspace-")
		if err != nil {
			return sandbox.Workspace{}, err
		}
		defer os.RemoveAll(egressDir)
		if err = os.Chmod(egressDir, 0755); err != nil {
			return sandbox.Workspace{}, err
		}
		proxy, err = egress.Listen(filepath.Join(egressDir, "egress.sock"), egress.Registries)
		if err != nil {
			return sandbox.Workspace{}, err
		}
		request.Egress = egressDir
	}

	workspace, err := p.runtime.PreparePinnedWorkspace(ctx, request)
	if err != nil {
		if proxy != nil {
			_ = proxy.Close()
		}
		return sandbox.Workspace{}, err
	}
	destroy := func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = p.runtime.Destroy(cleanup, workspace)
	}
	if len(patches) > 0 {
		if err = p.runtime.ApplyPatch(ctx, workspace, patches); err != nil {
			if proxy != nil {
				_ = proxy.Close()
			}
			destroy()
			return sandbox.Workspace{}, err
		}
	}
	if install {
		installCommand := sandbox.Command{
			Args:           []string{"/opt/reforge/tool", "egress", "/usr/local/bin/node", npmCLI, "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--registry", npmRegistry},
			Directory:      installDir,
			Timeout:        10 * time.Minute,
			MaxOutputBytes: 64 << 10,
			NetworkProfile: "egress",
		}
		result, runErr := p.runtime.ExecuteBoundedCommand(ctx, workspace, installCommand)
		closeErr := proxy.Close()
		proxy = nil
		if runErr != nil {
			destroy()
			return sandbox.Workspace{}, runErr
		}
		if closeErr != nil {
			destroy()
			return sandbox.Workspace{}, closeErr
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

func firstArg(command sandbox.Command) string {
	if len(command.Args) == 0 {
		return ""
	}
	return command.Args[0]
}

func applySnapshotPatches(snapshot sandbox.Snapshot, patches []sandbox.Patch) sandbox.Snapshot {
	files := make(map[string]guest.File, len(snapshot.Files)+len(patches))
	for _, file := range snapshot.Files {
		files[file.Path] = file
	}
	for _, patch := range patches {
		if patch.Delete {
			delete(files, patch.Path)
		} else {
			files[patch.Path] = guest.File{Path: patch.Path, Content: patch.Content}
		}
	}
	out := snapshot
	out.Files = make([]guest.File, 0, len(files))
	for _, file := range files {
		out.Files = append(out.Files, file)
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	out.ManifestSHA256 = ""
	if digest, err := sandbox.SnapshotDigest(out.Files); err == nil {
		out.ManifestSHA256 = digest
	}
	return out
}

func npmLockfileDirectory(snapshot sandbox.Snapshot, patches []sandbox.Patch, commandDir string) (string, bool) {
	files := make(map[string]bool, len(snapshot.Files)+len(patches))
	for _, file := range snapshot.Files {
		files[path.Clean(file.Path)] = true
	}
	for _, patch := range patches {
		name := path.Clean(patch.Path)
		if patch.Delete {
			delete(files, name)
		} else {
			files[name] = true
		}
	}
	dir := strings.Trim(path.Clean(commandDir), "/")
	dir = strings.TrimPrefix(dir, "workspace/")
	if dir == "workspace" || dir == "." {
		dir = ""
	}
	for {
		if files[path.Join(dir, "package.json")] && files[path.Join(dir, "package-lock.json")] {
			return dir, true
		}
		if dir == "" {
			return "", false
		}
		dir = path.Dir(dir)
		if dir == "." {
			dir = ""
		}
	}
}
