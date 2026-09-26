package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reforge/internal/maintenance/repair"
	"reforge/internal/sandbox"
	"reforge/internal/sandbox/guest"
)

func TestSandboxedDependencyUpdate(t *testing.T) {
	if os.Getenv("REFORGE_SANDBOX_E2E") == "" {
		t.Skip("requires a runner host with gVisor")
	}
	digest := func(path string) string {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		return hex.EncodeToString(sum[:])
	}
	state, err := os.MkdirTemp("", "sandbox-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	cfg := sandbox.RuntimeConfig{Runsc: "/app/gvisor/runsc", Tool: "/app/reforge-sandbox-tool", StateRoot: filepath.Join(state, "sandbox"), CgroupRoot: "/sys/fs/cgroup/reforge", Images: map[string]string{}, Rootless: true, MemoryBytes: 2 << 30, DiskBytes: 1 << 30, CPUs: 1, MaxProcesses: 256, RunscSHA256: digest("/app/gvisor/runsc"), ToolSHA256: digest("/app/reforge-sandbox-tool")}
	for _, recipe := range []string{"javascript", "go"} {
		image, err := sandbox.ImageDigest("/app/images/" + recipe)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Images[image] = "/app/images/" + recipe
	}
	update := func(name, manifest string, d repair.DependencyUpdate) (map[string][]byte, error) {
		files := map[string][]byte{name: []byte(manifest)}
		entries := []guest.File{{Path: name, Content: files[name]}}
		sum, err := sandbox.SnapshotDigest(entries)
		if err != nil {
			t.Fatal(err)
		}
		commit := strings.Repeat("c", 40)
		cfg.Fetch = func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
			return sandbox.Snapshot{CommitSHA: commit, Complete: true, ManifestSHA256: sum, Files: entries}, nil
		}
		runtime, err := sandbox.NewRuntime(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		u := updater{cfg: cfg, runtime: runtime, request: sandbox.WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: commit, Trust: "untrusted"}, target: files}
		return u.update(context.Background(), files, d)
	}
	run := func(manifest string) (map[string][]byte, error) {
		return update("package.json", manifest, repair.DependencyUpdate{Ecosystem: "npm", Directory: ".", Package: "js-yaml", Version: "5.2.2"})
	}
	out, err := run(`{"name":"fixture","version":"1.0.0","dependencies":{"js-yaml":"^5.2.0"}}`)
	if err != nil || !strings.Contains(string(out["package-lock.json"]), `"version": "5.2.2"`) {
		t.Fatalf("registry update failed: %v %s", err, out["package-lock.json"])
	}
	lock := string(out["package-lock.json"])
	t.Logf("lock excerpt: %s", lock[strings.Index(lock, `"node_modules/js-yaml"`):][:160])
	_, err = run(`{"name":"fixture","version":"1.0.0","dependencies":{"internal":"http://127.0.0.1:8084/internal.tgz"}}`)
	if err == nil {
		t.Fatal("dependency tooling reached a loopback destination")
	}
	t.Logf("loopback refused: %v", err)
	out, err = update("go.mod", "module example.com/fixture\n\ngo 1.27\n", repair.DependencyUpdate{Ecosystem: "go", Directory: ".", Package: "golang.org/x/text", Version: "v0.41.0"})
	if err != nil || !strings.Contains(string(out["go.mod"]), "golang.org/x/text v0.41.0") || len(out["go.sum"]) == 0 {
		t.Fatalf("go update failed: %v %s", err, out["go.mod"])
	}
}
