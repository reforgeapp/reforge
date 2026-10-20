package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/reforgeapp/reforge/pkg/egress"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
)

const (
	npmCLI      = "/usr/local/lib/node_modules/npm/bin/npm-cli.js"
	npmRegistry = "https://registry.npmjs.org/"
)

type updater struct {
	cfg     sandbox.RuntimeConfig
	runtime sandbox.SandboxRuntime
	request sandbox.WorkspaceRequest
	target  map[string][]byte
}

func (u updater) image(binary string) string {
	if u.cfg.Backend == "kubernetes" && u.cfg.Kubernetes != nil {
		toolchain := map[string]string{"usr/local/go/bin/go": "go", "usr/local/bin/node": "javascript"}[binary]
		return u.cfg.Kubernetes.Toolchains[toolchain]
	}
	for digest, root := range u.cfg.Images {
		if _, err := os.Stat(filepath.Join(root, binary)); err == nil {
			return digest
		}
	}
	return ""
}

func (u updater) update(ctx context.Context, files map[string][]byte, d repair.DependencyUpdate) (map[string][]byte, error) {
	if !d.Valid() {
		return nil, errors.New("invalid dependency update")
	}
	paths := d.Paths()
	if u.cfg.Backend == "kubernetes" {
		return u.updateKubernetes(ctx, files, d, paths)
	}
	if _, ok := files[paths[0]]; !ok {
		return nil, errors.New(paths[0] + " not found")
	}
	binary, args := "usr/local/go/bin/go", []string{"GOPROXY=https://proxy.golang.org", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local", "GOMODCACHE=/tmp/gomod", "/usr/local/go/bin/go", "get", d.Package + "@" + goVersion(d.Version)}
	if d.Ecosystem == "npm" {
		binary = "usr/local/bin/node"
		switch d.Strategy {
		case "update":
			args = []string{"update", d.Package}
		case "override":
			manifest, err := withOverride(files[paths[0]], d.Package, d.Version)
			if err != nil {
				return nil, err
			}
			files = copyFiles(files)
			files[paths[0]] = manifest
			args = []string{"install"}
		default:
			args = []string{"install", d.Package + "@" + d.Version}
		}
		args = append([]string{"/usr/local/bin/node", npmCLI}, append(args, "--package-lock-only", "--ignore-scripts", "--no-audit", "--no-fund", "--registry", npmRegistry)...)
	}
	image := u.image(binary)
	if image == "" {
		return nil, errors.New(d.Ecosystem + " tooling is not available on this runner")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	root := filepath.Join(u.cfg.StateRoot, "egress")
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, "egress-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err = os.Chmod(dir, 0755); err != nil {
		return nil, err
	}
	proxy, err := egress.Listen(filepath.Join(dir, "egress.sock"), egress.Registries)
	if err != nil {
		return nil, err
	}
	defer proxy.Close()
	request := u.request
	request.Image, request.Egress, request.Timeout, request.Dependencies = image, dir, 10*time.Minute, ""
	workspace, err := u.runtime.PreparePinnedWorkspace(ctx, request)
	if err != nil {
		return nil, err
	}
	defer u.runtime.Destroy(context.WithoutCancel(ctx), workspace)
	if patches := changed(u.target, files); len(patches) > 0 {
		if err = u.runtime.ApplyPatch(ctx, workspace, patches); err != nil {
			return nil, err
		}
	}
	result, err := u.runtime.ExecuteBoundedCommand(ctx, workspace, sandbox.Command{Args: append([]string{"/opt/reforge/tool", "egress"}, args...), Directory: path.Clean(d.Directory), Timeout: 5 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "egress"})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		tail := result.Output
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		return nil, fmt.Errorf("%s exited %d: %s", d.Ecosystem, result.ExitCode, tail)
	}
	out := map[string][]byte{}
	for _, name := range paths {
		if artifact, err := u.runtime.CollectArtifact(ctx, workspace, name); err == nil {
			out[name] = artifact.Data
		}
	}
	return out, nil
}

func goVersion(version string) string {
	if version == "" {
		return "latest"
	}
	return version
}

func copyFiles(files map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(files))
	for name, body := range files {
		out[name] = body
	}
	return out
}

func changed(base, files map[string][]byte) []sandbox.Patch {
	names := make([]string, 0, len(files))
	for name, body := range files {
		if old, ok := base[name]; !ok || !bytes.Equal(old, body) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]sandbox.Patch, 0, len(names))
	for _, name := range names {
		out = append(out, sandbox.Patch{Path: name, Content: files[name]})
	}
	return out
}

func withOverride(body []byte, pkg, version string) ([]byte, error) {
	if version == "" {
		return nil, errors.New("override requires a version")
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, err
	}
	overrides := map[string]json.RawMessage{}
	if raw, ok := manifest["overrides"]; ok {
		if err := json.Unmarshal(raw, &overrides); err != nil {
			return nil, err
		}
	}
	overrides[pkg], _ = json.Marshal(version)
	manifest["overrides"], _ = json.Marshal(overrides)
	out, err := json.MarshalIndent(manifest, "", "  ")
	return append(out, '\n'), err
}
