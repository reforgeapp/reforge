package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"reforge/internal/maintenance/recipes"
	"reforge/internal/maintenance/repair"
	"reforge/internal/sandbox"
	"reforge/internal/sandbox/guest"
)

func TestToolchainImagesRunFrozenRecipes(t *testing.T) {
	if os.Getenv("REFORGE_RUN_TOOLCHAIN_INTEGRATION") != "1" {
		t.Skip("set REFORGE_RUN_TOOLCHAIN_INTEGRATION=1")
	}
	if runtime.GOOS != "linux" {
		t.Skip("gVisor fixture requires Linux")
	}
	runsc := "/tmp/reforge-gvisor/bin/runsc"
	tool := "/home/mnorris/repos/reforge/bin/reforge-sandbox-tool"
	if _, err := os.Stat(runsc); err != nil {
		t.Skip("missing pinned runsc")
	}
	if _, err := os.Stat(tool); err != nil {
		t.Skip("missing sandbox tool")
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	repo := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	cases := []struct {
		stack, imageName, source, patch string
		files                           map[string][]byte
	}{
		{"go", "go", "main.go", "package main\nfunc value() int { return 2 }\n", map[string][]byte{"go.mod": []byte("module example.test\n\ngo 1.27\n"), "main.go": []byte("package main\nfunc value() int { return 1 }\n"), "main_test.go": []byte("package main\nimport \"testing\"\nfunc TestValue(t *testing.T) { if value() != 2 { t.Fatalf(\"value=%d\", value()) } }\n")}},
		{"javascript", "javascript", "value.js", "export function value() { return 2; }\n", map[string][]byte{"value.js": []byte("export function value() { return 1; }\n"), "value.test.js": []byte("import { strict as assert } from 'node:assert';\nimport { value } from './value.js';\nimport test from 'node:test';\ntest('value', () => assert.equal(value(), 2));\n")}},
		{"python", "python", "value.py", "def value():\n    return 2\n", map[string][]byte{"value.py": []byte("def value():\n    return 1\n"), "test_value.py": []byte("import unittest\nfrom value import value\nclass ValueTest(unittest.TestCase):\n    def test_value(self): self.assertEqual(value(), 2)\n")}},
	}
	for _, tc := range cases {
		t.Run(tc.stack, func(t *testing.T) {
			runToolchainCase(t, repo, runsc, tool, tc.stack, tc.imageName, tc.source, tc.patch, tc.files)
		})
	}
}

func runToolchainCase(t *testing.T, repo, runsc, tool, stack, imageName, source, patch string, files map[string][]byte) {
	t.Helper()
	imageRoot := filepath.Join(t.TempDir(), "image")
	cmd := exec.Command("python3", filepath.Join(repo, "scripts/build-runner-images.py"), "--stack", stack, "--output", imageRoot)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("image build: %v: %s", err, output)
	}
	t.Cleanup(func() { os.RemoveAll(imageRoot) })
	image, err := sandbox.ImageDigest(imageRoot)
	if err != nil {
		t.Fatal(err)
	}
	stateRoot, err := os.MkdirTemp("/tmp", "rf-tc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateRoot) })
	if err := os.Chmod(stateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(stateRoot, "runsc"), 0700); err != nil {
		t.Fatal(err)
	}
	runtimeConfig := sandbox.RuntimeConfig{Runsc: runsc, RunscSHA256: digestFile(t, runsc), Tool: tool, ToolSHA256: digestFile(t, tool), StateRoot: stateRoot, Images: map[string]string{image: imageRoot}, Development: true, Rootless: true, MemoryBytes: 2 << 30, DiskBytes: 512 << 20, CPUs: 1, MaxProcesses: 128}
	runtimeConfig.Fetch = func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		snapshotFiles := snapshot(files)
		manifest, e := sandbox.SnapshotDigest(snapshotFiles)
		return sandbox.Snapshot{CommitSHA: strings.Repeat("a", 40), Complete: e == nil, ManifestSHA256: manifest, Files: snapshotFiles}, e
	}
	r, err := sandbox.NewRuntime(runtimeConfig)
	if err != nil {
		t.Fatalf("runtime init image=%s root=%s: %v", image, imageRoot, err)
	}
	t.Cleanup(func() {
		if e := r.Close(); e != nil {
			t.Error(e)
		}
	})
	plan, err := repair.Freeze(stack, image, strings.Repeat("a", 40), strings.Repeat("b", 40), files, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := sandbox.WorkspaceRequest{JobID: "toolchain-" + stack, AttemptID: "baseline", CommitSHA: strings.Repeat("a", 40), Image: image, Trust: "development-fixture", Timeout: 2 * time.Minute}
	w, err := r.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Destroy(context.Background(), w) })
	baseline := executeRecipe(t, r, w, plan.Recipe)
	if !repair.Reproduced(baseline) {
		t.Fatalf("baseline did not reproduce failure: %#v", baseline)
	}
	change := sandbox.Patch{Path: source, Content: []byte(patch)}
	if err := repair.CheckPatch(plan, files, []sandbox.Patch{change}); err != nil {
		t.Fatalf("source patch rejected: %v", err)
	}
	if err := r.ApplyPatch(context.Background(), w, []sandbox.Patch{change}); err != nil {
		t.Fatal(err)
	}
	candidate := executeRecipe(t, r, w, plan.Recipe)
	if !repair.Verified(plan, baseline, candidate) {
		t.Fatalf("candidate did not verify: baseline=%#v candidate=%#v", baseline, candidate)
	}
}

func executeRecipe(t *testing.T, r sandbox.SandboxRuntime, w sandbox.Workspace, recipe recipes.Recipe) []repair.CheckResult {
	t.Helper()
	results := make([]repair.CheckResult, 0, len(recipe.Commands))
	for _, command := range recipe.Commands {
		result, err := r.ExecuteBoundedCommand(context.Background(), w, sandbox.Command{Args: command.Args, Directory: command.Directory, Timeout: time.Duration(command.TimeoutSeconds) * time.Second, MaxOutputBytes: 4 << 20, NetworkProfile: "none"})
		if err != nil {
			t.Fatal(err)
		}
		if result.ExitCode != 0 {
			excerpt := result.Output
			if len(excerpt) > 512 {
				excerpt = excerpt[:512]
			}
			t.Logf("command %s exit=%d timeout=%t truncated=%t output=%q", command.ID, result.ExitCode, result.TimedOut, result.Truncated, excerpt)
		}
		results = append(results, repair.Interpret(command, result))
	}
	return results
}

func snapshot(files map[string][]byte) []guest.File {
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	out := make([]guest.File, 0, len(paths))
	for _, name := range paths {
		out = append(out, guest.File{Path: name, Content: files[name]})
	}
	return out
}
func digestFile(t *testing.T, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
