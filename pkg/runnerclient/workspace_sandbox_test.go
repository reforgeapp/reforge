package runnerclient

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

func TestSandboxedPreparedCommands(t *testing.T) {
	if os.Getenv("REFORGE_SANDBOX_E2E") == "" {
		t.Skip("requires a runner host with gVisor")
	}
	state, err := os.MkdirTemp("", "sandbox-prepared-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(state) })
	cfg := sandbox.RuntimeConfig{
		Runsc:          "/app/gvisor/runsc",
		Tool:           "/app/reforge-sandbox-tool",
		StateRoot:      filepath.Join(state, "sandbox"),
		DependencyRoot: filepath.Join(state, "dependencies"),
		CgroupRoot:     "/sys/fs/cgroup/reforge",
		Images:         map[string]string{},
		Rootless:       true,
		MemoryBytes:    2 << 30,
		DiskBytes:      1 << 30,
		CPUs:           1,
		MaxProcesses:   256,
	}
	for _, recipe := range []string{"go", "javascript", "python"} {
		image, err := sandbox.ImageDigest("/app/images/" + recipe)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Images[image] = "/app/images/" + recipe
	}
	fileDigest := func(name string) string {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		return hex.EncodeToString(sum[:])
	}
	cfg.RunscSHA256 = fileDigest(cfg.Runsc)
	cfg.ToolSHA256 = fileDigest(cfg.Tool)
	snapshots := map[string]sandbox.Snapshot{}
	cfg.Fetch = func(_ context.Context, request sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		snapshot, ok := snapshots[request.CommitSHA]
		if !ok {
			return sandbox.Snapshot{}, fmt.Errorf("missing test snapshot %s", request.CommitSHA)
		}
		return snapshot, nil
	}
	runtime, err := sandbox.NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("close sandbox runtime: %v", err)
		}
	})
	ctx := context.Background()
	commit := func(name string) string {
		sum := sha1.Sum([]byte(name))
		return hex.EncodeToString(sum[:])
	}
	addSnapshot := func(name string, files []guest.File) sandbox.Snapshot {
		t.Helper()
		sha, err := sandbox.SnapshotDigest(files)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := sandbox.Snapshot{CommitSHA: commit(name), Complete: true, ManifestSHA256: sha, Files: files}
		snapshots[snapshot.CommitSHA] = snapshot
		return snapshot
	}
	request := func(snapshot sandbox.Snapshot) sandbox.WorkspaceRequest {
		return sandbox.WorkspaceRequest{JobID: "prepared-e2e", AttemptID: "attempt", CommitSHA: snapshot.CommitSHA, Trust: "untrusted", Timeout: 10 * time.Minute}
	}
	prepare := func(snapshot sandbox.Snapshot, patches []sandbox.Patch, command sandbox.Command) (sandbox.Workspace, error) {
		return (commandPreparer{cfg: cfg, runtime: runtime, org: "prepared-e2e", fetch: cfg.Fetch}).prepare(ctx, request(snapshot), patches, command)
	}
	run := func(workspace sandbox.Workspace, command sandbox.Command) sandbox.CommandResult {
		t.Helper()
		result, err := runtime.ExecuteBoundedCommand(ctx, workspace, command)
		if err != nil {
			t.Fatal(err)
		}
		if result.ExitCode != 0 || result.TimedOut || result.Truncated {
			t.Fatalf("command failed: code=%d timeout=%t truncated=%t output=%s", result.ExitCode, result.TimedOut, result.Truncated, result.Output)
		}
		return result
	}
	check := func(name string, snapshot sandbox.Snapshot, patches []sandbox.Patch, command sandbox.Command) sandbox.Workspace {
		t.Helper()
		workspace, err := prepare(snapshot, patches, command)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := runtime.Destroy(cleanup, workspace); err != nil {
				t.Errorf("destroy %s workspace: %v", name, err)
			}
		})
		return workspace
	}

	goSnapshot := addSnapshot("go", []guest.File{
		{Path: "go.mod", Content: []byte("module example.test/fixture\n\ngo 1.22\n")},
		{Path: "fixture_test.go", Content: []byte("package fixture\n\nimport \"testing\"\n\nfunc TestStdlib(t *testing.T) { if got := len([]byte(\"ok\")); got != 2 { t.Fatal(got) } }\n")},
	})
	goWorkspace := check("go", goSnapshot, nil, sandbox.Command{Args: []string{"/usr/local/go/bin/go", "test", "./..."}, Directory: ".", Timeout: 2 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"})
	run(goWorkspace, sandbox.Command{Args: []string{"/usr/local/go/bin/go", "test", "./..."}, Directory: ".", Timeout: 2 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"})

	nodeFiles := map[string][]byte{"package.json": []byte("{\"name\":\"prepared-fixture\",\"version\":\"1.0.0\",\"scripts\":{\"preinstall\":\"echo ran > repository-script-ran\",\"build\":\"vite build\",\"test:unit\":\"vitest run\"},\"dependencies\":{\"js-yaml\":\"^5.2.0\",\"vue\":\"^3.5.13\"},\"devDependencies\":{\"vite\":\"^8.1.4\",\"vitest\":\"^4.1.10\",\"@vitejs/plugin-vue\":\"^6.0.1\"}}\n")}
	nodeSnapshot := addSnapshot("node", []guest.File{
		{Path: "package.json", Content: nodeFiles["package.json"]},
		{Path: "index.html", Content: []byte("<div id=\"app\"></div><script type=\"module\" src=\"/src/main.js\"></script>\n")},
		{Path: "vite.config.js", Content: []byte("import { defineConfig } from 'vite'; import vue from '@vitejs/plugin-vue'; export default defineConfig({ plugins: [vue()] });\n")},
		{Path: "src/App.vue", Content: []byte("<template><p>rolldown native smoke</p></template>\n")},
		{Path: "src/main.js", Content: []byte("import { createApp } from 'vue'; import App from './App.vue'; createApp(App).mount('#app');\n")},
		{Path: "src/basic.test.ts", Content: []byte("import { expect, it } from 'vitest'; import { createSSRApp } from 'vue'; import { renderToString } from 'vue/server-renderer'; import App from './App.vue'; it('renders compiled Vue component', async () => { const html = await renderToString(createSSRApp(App)); expect(html).toContain('<p>rolldown native smoke</p>'); });\n")},
	})
	updateOutput, err := (updater{cfg: cfg, runtime: runtime, request: request(nodeSnapshot), target: nodeFiles}).update(ctx, nodeFiles, repair.DependencyUpdate{Ecosystem: "npm", Directory: ".", Package: "js-yaml", Version: "5.2.2"})
	if err != nil {
		t.Fatalf("prepare npm lockfile: %v", err)
	}
	nodePatches := changed(nodeFiles, updateOutput)
	nodeWorkspace := check("node", nodeSnapshot, nodePatches, sandbox.Command{Args: []string{"node", "-e", "const yaml=require('js-yaml'); if (yaml.load('answer: 42').answer !== 42) process.exit(1); console.log('js-yaml ok')"}, Directory: ".", Timeout: 2 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"})
	if result := run(nodeWorkspace, sandbox.Command{Args: []string{"node", "-e", "const yaml=require('js-yaml'); if (yaml.load('answer: 42').answer !== 42) process.exit(1); console.log('js-yaml ok')"}, Directory: ".", Timeout: 2 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"}); !strings.Contains(string(result.Output), "js-yaml ok") {
		t.Fatalf("npm dependency unavailable: %s", result.Output)
	}
	buildResult := run(nodeWorkspace, sandbox.Command{Args: []string{"node", "/usr/local/lib/node_modules/npm/bin/npm-cli.js", "run", "build"}, Directory: ".", Timeout: 3 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"})
	if !strings.Contains(string(buildResult.Output), "built in") {
		t.Fatalf("Vite/Rolldown build did not complete: %s", buildResult.Output)
	}
	unitResult := run(nodeWorkspace, sandbox.Command{Args: []string{"node", "/usr/local/lib/node_modules/npm/bin/npm-cli.js", "run", "test:unit"}, Directory: ".", Timeout: 3 * time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"})
	if !strings.Contains(string(unitResult.Output), "1 passed") {
		t.Fatalf("Vitest native smoke failed: %s", unitResult.Output)
	}
	if len(nodeSnapshot.Files) != 6 || string(nodeSnapshot.Files[0].Content) != string(nodeFiles["package.json"]) {
		t.Fatal("npm preparation mutated source snapshot")
	}
	if _, err := runtime.CollectArtifact(ctx, nodeWorkspace, "repository-script-ran"); err == nil {
		t.Fatal("npm preparation ran repository lifecycle script")
	}
	registryProbe := sandbox.Command{Args: []string{"/opt/reforge/tool", "egress", "/usr/local/bin/node", "-e", "fetch('https://registry.npmjs.org/js-yaml').then(() => process.exit(1), () => { console.log('registry unavailable'); process.exit(0) })"}, Directory: ".", Timeout: 10 * time.Second, MaxOutputBytes: 64 << 10, NetworkProfile: "egress"}
	if result := run(nodeWorkspace, registryProbe); !strings.Contains(string(result.Output), "registry unavailable") {
		t.Fatalf("registry probe did not report blocked access: %s", result.Output)
	}

	pythonSnapshot := addSnapshot("python", []guest.File{{Path: "test_fixture.py", Content: []byte("import unittest\n\nclass FixtureTest(unittest.TestCase):\n    def test_stdlib(self):\n        self.assertEqual(len('ok'), 2)\n\nif __name__ == '__main__':\n    unittest.main()\n")}})
	pythonWorkspace := check("python", pythonSnapshot, nil, sandbox.Command{Args: []string{"python3", "-m", "unittest", "-v"}, Directory: ".", Timeout: time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"})
	if result := run(pythonWorkspace, sandbox.Command{Args: []string{"python3", "-m", "unittest", "-v"}, Directory: ".", Timeout: time.Minute, MaxOutputBytes: 64 << 10, NetworkProfile: "none"}); !strings.Contains(string(result.Output), "OK") {
		t.Fatalf("python stdlib test failed: %s", result.Output)
	}
}
