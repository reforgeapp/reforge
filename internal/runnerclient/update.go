package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"time"

	"reforge/internal/maintenance/repair"
	"reforge/internal/sandbox"
)

var npmCLI = "/app/npm/bin/npm-cli.js"

const npmRegistry = "https://registry.npmjs.org/"

func toolRoot(cfg sandbox.RuntimeConfig, binary string) string {
	for _, root := range cfg.Images {
		if _, err := os.Stat(filepath.Join(root, binary)); err == nil {
			return root
		}
	}
	return ""
}

func updateDependency(cfg sandbox.RuntimeConfig) func(context.Context, map[string][]byte, repair.DependencyUpdate) (map[string][]byte, error) {
	return func(ctx context.Context, files map[string][]byte, u repair.DependencyUpdate) (map[string][]byte, error) {
		if !u.Valid() {
			return nil, errors.New("invalid dependency update")
		}
		paths := u.Paths()
		if _, ok := files[paths[0]]; !ok {
			return nil, errors.New(paths[0] + " not found")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		work, err := os.MkdirTemp(cfg.DependencyRoot, "update-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(work)
		for _, name := range paths {
			if body, ok := files[name]; ok {
				if err = os.WriteFile(filepath.Join(work, path.Base(name)), body, 0644); err != nil {
					return nil, err
				}
			}
		}
		for _, dir := range []string{filepath.Join(work, "home"), filepath.Join(work, "cache")} {
			if err = os.Mkdir(dir, 0755); err != nil {
				return nil, err
			}
		}
		if os.Getuid() == 0 {
			err = filepath.Walk(work, func(name string, _ os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				return os.Chown(name, sandboxUser, sandboxUser)
			})
			if err != nil {
				return nil, err
			}
		}
		switch u.Ecosystem {
		case "npm":
			err = npmUpdate(ctx, cfg, work, u)
		case "go":
			err = goUpdate(ctx, cfg, work, u)
		}
		if err != nil {
			return nil, err
		}
		out := map[string][]byte{}
		for _, name := range paths {
			body, err := os.ReadFile(filepath.Join(work, path.Base(name)))
			if err == nil {
				out[name] = body
			}
		}
		return out, nil
	}
}

func npmUpdate(ctx context.Context, cfg sandbox.RuntimeConfig, work string, u repair.DependencyUpdate) error {
	root := toolRoot(cfg, "usr/local/bin/node")
	if _, err := os.Stat(npmCLI); root == "" || err != nil {
		return errors.New("npm is not available on this runner")
	}
	spec := u.Package
	if u.Version != "" {
		spec += "@" + u.Version
	}
	var args []string
	switch u.Strategy {
	case "update":
		args = []string{"update", u.Package}
	case "override":
		if u.Version == "" {
			return errors.New("override requires a version")
		}
		if err := setOverride(filepath.Join(work, "package.json"), u.Package, u.Version); err != nil {
			return err
		}
		args = []string{"install"}
	default:
		args = []string{"install", spec}
	}
	args = append(args, "--package-lock-only", "--ignore-scripts", "--no-audit", "--no-fund", "--registry", npmRegistry)
	command := append([]string{"--library-path", filepath.Join(root, "lib/x86_64-linux-gnu") + ":" + filepath.Join(root, "lib64"), filepath.Join(root, "usr/local/bin/node"), npmCLI}, args...)
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + filepath.Join(work, "home"), "npm_config_cache=" + filepath.Join(work, "cache"), "npm_config_userconfig=/dev/null", "npm_config_globalconfig=/dev/null", "npm_config_update_notifier=false"}
	return runTool(ctx, work, env, filepath.Join(root, "lib64/ld-linux-x86-64.so.2"), command...)
}

func setOverride(name, pkg, version string) error {
	body, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	var manifest map[string]json.RawMessage
	if err = json.Unmarshal(body, &manifest); err != nil {
		return err
	}
	overrides := map[string]json.RawMessage{}
	if raw, ok := manifest["overrides"]; ok {
		if err = json.Unmarshal(raw, &overrides); err != nil {
			return err
		}
	}
	overrides[pkg], _ = json.Marshal(version)
	manifest["overrides"], _ = json.Marshal(overrides)
	body, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(name, append(body, '\n'), 0644)
}

func goUpdate(ctx context.Context, cfg sandbox.RuntimeConfig, work string, u repair.DependencyUpdate) error {
	root := toolRoot(cfg, "usr/local/go/bin/go")
	if root == "" {
		return errors.New("go is not available on this runner")
	}
	version := u.Version
	if version == "" {
		version = "latest"
	}
	goBinary := filepath.Join(root, "usr/local/go/bin/go")
	home := filepath.Join(work, "home")
	env := []string{"PATH=" + filepath.Dir(goBinary) + ":/usr/bin:/bin", "HOME=" + home, "GOROOT=" + filepath.Join(root, "usr/local/go"), "GOMODCACHE=" + filepath.Join(work, "cache"), "GOCACHE=" + filepath.Join(home, "build"), "GOPATH=" + filepath.Join(home, "gopath"), "GOPROXY=https://proxy.golang.org", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local", "CGO_ENABLED=0"}
	return runTool(ctx, work, env, goBinary, "get", u.Package+"@"+version)
}
