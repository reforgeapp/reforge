package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/maintenance/repair"
	"reforge/internal/model"
	"reforge/internal/model/compatible"
	"reforge/internal/network"
	"reforge/internal/sandbox"
	"reforge/internal/sandbox/guest"
)

func TestRepairEngineRealModelAndGVisor(t *testing.T) {
	if os.Getenv("REFORGE_REPAIR_ENGINE_TEST") != "1" {
		t.Skip("set REFORGE_REPAIR_ENGINE_TEST=1 for real repair-engine acceptance")
	}
	if runtime.GOOS != "linux" {
		t.Skip("gVisor fixture requires Linux")
	}
	if _, err := os.Stat("/tmp/reforge-gvisor/bin/runsc"); err != nil {
		t.Skip("missing pinned runsc")
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	repo := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	tool := filepath.Join(repo, "bin/reforge-sandbox-tool")
	if _, err := os.Stat(tool); err != nil {
		t.Skip("missing sandbox helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	imageRoot := filepath.Join(t.TempDir(), "image")
	command := exec.CommandContext(ctx, "python3", filepath.Join(repo, "scripts/build-runner-images.py"), "--stack", "javascript", "--output", imageRoot)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("toolchain image build failed: %v: %s", err, output)
	}
	image, err := sandbox.ImageDigest(imageRoot)
	if err != nil {
		t.Fatal(err)
	}
	stateRoot, err := os.MkdirTemp("/tmp", "rf-eng-")
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
	runsc := "/tmp/reforge-gvisor/bin/runsc"
	baselineSHA := strings.Repeat("a", 40)
	targetSHA := strings.Repeat("b", 40)
	candidateSHA := strings.Repeat("c", 40)
	baseFiles := map[string][]byte{
		"value.js":      []byte("export function value() { return 1; }\n"),
		"value.test.js": []byte("import { strict as assert } from 'node:assert';\nimport { value } from './value.js';\nimport test from 'node:test';\ntest('value', () => assert.equal(value(), 2));\n"),
	}
	targetFiles := map[string][]byte{"value.js": baseFiles["value.js"], "value.test.js": baseFiles["value.test.js"]}
	base := makeSnapshot(t, baselineSHA, baseFiles)
	target := makeSnapshot(t, targetSHA, targetFiles)
	var candidate sandbox.Snapshot
	runtimeConfig := sandbox.RuntimeConfig{Runsc: runsc, RunscSHA256: fileDigest(t, runsc), Tool: tool, ToolSHA256: fileDigest(t, tool), StateRoot: stateRoot, Images: map[string]string{image: imageRoot}, Development: true, Rootless: true, MemoryBytes: 2 << 30, DiskBytes: 512 << 20, CPUs: 1, MaxProcesses: 128}
	runtimeConfig.Fetch = func(_ context.Context, request sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		switch request.CommitSHA {
		case baselineSHA:
			return base, nil
		case targetSHA:
			return target, nil
		case candidateSHA:
			return candidate, nil
		default:
			return sandbox.Snapshot{}, sandbox.ErrBoundary
		}
	}
	r, err := sandbox.NewRuntime(runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	plan, err := repair.Freeze("javascript", image, baselineSHA, targetSHA, baseFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	runnerID := domain.NewID()
	modelClient, err := network.NewClient("http://127.0.0.1:55435", network.Options{Development: true, RunnerID: runnerID, PrivateRoute: &network.PrivateRoute{OrgID: domain.NewID(), ConnectionID: domain.NewID(), RunnerID: runnerID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}})
	if err != nil {
		t.Fatal(err)
	}
	modelClient.Timeout = 5*time.Minute + 15*time.Second
	defer modelClient.CloseIdleConnections()
	modelName := os.Getenv("REFORGE_REPAIR_TEST_MODEL")
	if modelName == "" {
		modelName = "qwen3:1.7b"
	}
	provider, err := compatible.New(model.Config{Endpoint: "http://127.0.0.1:55435", Model: modelName, APIKey: "local-ollama", Profile: "ollama", Client: modelClient})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := make([][]byte, 0)
	turnSummary := []string{}
	engine := repair.Engine{Runtime: r, Model: modelName, JobID: "engine-integration", AttemptID: "attempt-1", Trust: "development-fixture", MaxOutputTokens: 1024, TurnTimeout: 5 * time.Minute, Turn: func(turnCtx context.Context, turn model.Turn) (model.TurnResult, error) {
		if len(turn.Messages) > 0 && turn.Messages[0].Role == "user" && len(turn.Continuation) == 0 {
			turn.Messages[0].Text += " Read value.js and value.test.js, repair the failing arithmetic while preserving the test, then run_checks. /no_think"
		}
		result, turnErr := model.CollectTurn(turnCtx, provider, turn)
		for _, call := range result.ToolCalls {
			var input struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			_ = json.Unmarshal(call.Arguments, &input)
			detail := fmt.Sprintf("%s(%s,%d)", call.Name, input.Path, len(input.Content))
			if call.Name == "apply_patch" {
				content := input.Content
				if len(content) > 160 {
					content = content[:160]
				}
				detail += fmt.Sprintf("=%q", content)
			}
			turnSummary = append(turnSummary, detail)
		}
		if turnErr != nil {
			turnSummary = append(turnSummary, "error")
		}
		return result, turnErr
	}, Artifact: func(_ context.Context, _ string, data []byte) (string, error) {
		artifacts = append(artifacts, append([]byte(nil), data...))
		return "artifact-" + string(rune('0'+len(artifacts))), nil
	}}
	report, err := engine.Run(ctx, plan, base, target)
	if err != nil {
		t.Fatalf("real engine failed: %v model=%s turns=%d calls=%v baseline=%v candidate=%v candidate_detail=%s state=%s reason=%s", err, modelName, report.Turns, turnSummary, exitCodes(report.Baseline), exitCodes(report.Candidate), checkSummary(report.Candidate), report.State, report.Reason)
	}
	if report.State != "validated" || len(report.Patches) != 1 || report.Patches[0].Path != "value.js" || len(artifacts) < 3 {
		t.Fatalf("incomplete engine evidence state=%s patches=%+v artifacts=%d", report.State, report.Patches, len(artifacts))
	}
	if string(baseFiles["value.test.js"]) != string(targetFiles["value.test.js"]) {
		t.Fatal("engine changed unexpected source or protected test")
	}
	candidate = makeSnapshot(t, candidateSHA, map[string][]byte{"value.js": append([]byte(nil), report.Patches[0].Content...), "value.test.js": append([]byte(nil), targetFiles["value.test.js"]...)})
	publication, err := engine.ValidateNative(ctx, plan, candidateSHA, target, report)
	if err != nil || publication.HeadSHA != candidateSHA || len(publication.Checks) == 0 {
		t.Fatalf("native validation failed: %+v error=%v", publication, err)
	}
}

func exitCodes(results []repair.CheckResult) []int {
	codes := make([]int, 0, len(results))
	for _, result := range results {
		codes = append(codes, result.ExitCode)
	}
	return codes
}

func checkSummary(results []repair.CheckResult) string {
	if len(results) == 0 {
		return "none"
	}
	result := results[0]
	excerpt := result.Excerpt
	if len(excerpt) > 200 {
		excerpt = excerpt[:200]
	}
	return fmt.Sprintf("%s exit=%d reason=%q excerpt=%q", result.CommandID, result.ExitCode, result.Reason, excerpt)
}

func makeSnapshot(t *testing.T, commit string, files map[string][]byte) sandbox.Snapshot {
	t.Helper()
	entries := make([]guest.File, 0, len(files))
	for name, data := range files {
		entries = append(entries, guest.File{Path: name, Content: append([]byte(nil), data...)})
	}
	digest, err := sandbox.SnapshotDigest(entries)
	if err != nil {
		t.Fatal(err)
	}
	return sandbox.Snapshot{CommitSHA: commit, Complete: true, ManifestSHA256: digest, Files: entries}
}

func fileDigest(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
