package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/sandbox"
)

func builtinKubernetesFixture(t *testing.T) (builtinFlags, sandbox.RuntimeConfig) {
	t.Helper()
	t.Setenv("POD_IP", "10.42.0.9")
	t.Setenv("KUBERNETES_POD_IP", "")
	root := t.TempDir()
	config := sandbox.RuntimeConfig{
		Backend: "kubernetes",
		Kubernetes: &sandbox.KubernetesRuntimeConfig{
			Namespace: "reforge-workspaces", RuntimeClassName: "gvisor",
			Images: map[string]string{}, Toolchains: map[string]string{},
		},
		MemoryBytes: 6 << 30, DiskBytes: 3 << 30, CPUs: 2, MaxProcesses: 512,
	}
	for i, recipe := range recipes {
		digest := "sha256:" + strings.Repeat(string(rune('a'+i)), 64)
		config.Kubernetes.Images[digest] = "ghcr.io/reforgeapp/reforge-workspace-" + recipe + "@" + digest
		config.Kubernetes.Toolchains[recipe] = digest
	}
	flags, err := parseBuiltin("builtin", []string{
		"--dir", root, "--runtime-config", filepath.Join(root, "runtime.json"),
		"--state", filepath.Join(root, "state"), "--images", filepath.Join(root, "missing-images"),
		"--runsc", filepath.Join(root, "missing-runsc"), "--tool", filepath.Join(root, "missing-tool"),
		"--cgroup-root", filepath.Join(root, "missing-cgroup"), "--public-url", "https://app.reforgeapp.dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	writeBuiltinRuntimeFixture(t, flags.runtimeConfig, config)
	return flags, config
}

func writeBuiltinRuntimeFixture(t *testing.T, name string, config sandbox.RuntimeConfig) {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinInitUsesKubernetesCatalogWithoutLocalAssets(t *testing.T) {
	flags, config := builtinKubernetesFixture(t)
	args := []string{"--dir", flags.dir, "--runtime-config", flags.runtimeConfig, "--images", flags.images, "--runsc", flags.runsc, "--tool", flags.tool, "--cgroup-root", flags.cgroup}
	if err := builtinInit(args); err != nil {
		t.Fatalf("Kubernetes initialization required local sandbox assets: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(flags.dir, "repair-images.json"))
	if err != nil {
		t.Fatal(err)
	}
	var images map[string]string
	if err = json.Unmarshal(raw, &images); err != nil || !reflect.DeepEqual(images, config.Kubernetes.Toolchains) {
		t.Fatalf("catalog does not contain configured OCI digests: images=%v err=%v", images, err)
	}
	token, err := os.ReadFile(filepath.Join(flags.dir, "token"))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base64.RawURLEncoding.DecodeString(string(token))
	if err != nil || len(secret) != 32 {
		t.Fatalf("invalid generated token: bytes=%d err=%v", len(secret), err)
	}
	if err = builtinInit(args); err != nil {
		t.Fatal(err)
	}
	preserved, err := os.ReadFile(filepath.Join(flags.dir, "token"))
	if err != nil || string(preserved) != string(token) {
		t.Fatalf("initialization replaced existing enrollment token: %v", err)
	}
	for _, name := range []string{"repair-images.json", "token"} {
		info, err := os.Stat(filepath.Join(flags.dir, name))
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("shared %s requires mode 0640: %v", name, err)
		}
	}
	loaded, runtimeImages, err := builtinRuntimeConfig(flags)
	if err != nil || !reflect.DeepEqual(loaded, config) || !reflect.DeepEqual(runtimeImages, images) {
		t.Fatalf("runtime configuration and control catalog differ: err=%v", err)
	}
	for _, path := range []string{flags.images, flags.runsc, flags.tool, flags.cgroup, flags.state} {
		if _, err = os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("Kubernetes metadata initialization touched local asset %q: %v", path, err)
		}
	}
}

func TestBuiltinRejectsInvalidKubernetesConfigBeforeStartup(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *builtinFlags, *sandbox.RuntimeConfig)
	}{
		{name: "missing toolchain", change: func(_ *testing.T, _ *builtinFlags, config *sandbox.RuntimeConfig) {
			delete(config.Kubernetes.Toolchains, "python")
		}},
		{name: "mismatched image", change: func(_ *testing.T, _ *builtinFlags, config *sandbox.RuntimeConfig) {
			digest := config.Kubernetes.Toolchains["go"]
			config.Kubernetes.Images[digest] = "ghcr.io/reforgeapp/reforge-workspace-go@sha256:" + strings.Repeat("d", 64)
		}},
		{name: "missing pod IP", change: func(t *testing.T, _ *builtinFlags, _ *sandbox.RuntimeConfig) {
			t.Setenv("POD_IP", "")
		}},
		{name: "development mode", change: func(_ *testing.T, _ *builtinFlags, config *sandbox.RuntimeConfig) {
			config.Development = true
		}},
		{name: "local backend", change: func(_ *testing.T, _ *builtinFlags, config *sandbox.RuntimeConfig) {
			*config = sandbox.RuntimeConfig{}
		}},
		{name: "relative path", change: func(_ *testing.T, flags *builtinFlags, _ *sandbox.RuntimeConfig) {
			flags.runtimeConfig = "runtime.json"
		}},
		{name: "symlink", change: func(t *testing.T, flags *builtinFlags, _ *sandbox.RuntimeConfig) {
			link := filepath.Join(flags.dir, "runtime-link.json")
			if err := os.Symlink(flags.runtimeConfig, link); err != nil {
				t.Fatal(err)
			}
			flags.runtimeConfig = link
		}},
		{name: "writable by others", change: func(t *testing.T, flags *builtinFlags, _ *sandbox.RuntimeConfig) {
			if err := os.Chmod(flags.runtimeConfig, 0666); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			flags, config := builtinKubernetesFixture(t)
			originalPath := flags.runtimeConfig
			test.change(t, &flags, &config)
			writeBuiltinRuntimeFixture(t, originalPath, config)
			args := []string{"--dir", flags.dir, "--runtime-config", flags.runtimeConfig, "--state", flags.state, "--public-url", flags.publicURL}
			if err := builtinInit(args); err == nil {
				t.Fatal("invalid runtime accepted during initialization")
			}
			if err := runBuiltin(args); err == nil || strings.Contains(err.Error(), "token unavailable") {
				t.Fatalf("runtime was not rejected before enrollment setup: %v", err)
			}
			for _, path := range []string{filepath.Join(flags.dir, "repair-images.json"), filepath.Join(flags.dir, "token"), flags.state} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("invalid configuration changed %q: %v", path, err)
				}
			}
		})
	}
}

func TestBuiltinLocalCatalogRemainsDefault(t *testing.T) {
	root := t.TempDir()
	images := filepath.Join(root, "images")
	for _, recipe := range recipes {
		path := filepath.Join(images, recipe)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "recipe"), []byte(recipe), 0600); err != nil {
			t.Fatal(err)
		}
	}
	flags, err := parseBuiltin("builtin", []string{"--dir", root, "--images", images})
	if err != nil {
		t.Fatal(err)
	}
	config, catalog, err := builtinRuntimeConfig(flags)
	if err != nil {
		t.Fatal(err)
	}
	if config.Backend != "" || config.Kubernetes != nil || !config.Rootless || config.CgroupRoot != "/sys/fs/cgroup/reforge" || config.Runsc != "/app/gvisor/runsc" || config.Tool != "/app/reforge-sandbox-tool" {
		t.Fatalf("local runtime defaults changed: %+v", config)
	}
	if err = builtinInit([]string{"--dir", root, "--images", images}); err != nil {
		t.Fatal(err)
	}
	for _, recipe := range recipes {
		digest, err := sandbox.ImageDigest(filepath.Join(images, recipe))
		if err != nil || catalog[recipe] != digest || config.Images[digest] != filepath.Join(images, recipe) {
			t.Fatalf("local %s mapping changed: %v", recipe, err)
		}
	}
}
