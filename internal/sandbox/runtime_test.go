package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reforge/internal/sandbox/guest"
)

func TestRealGVisorBoundaryAndCancellation(t *testing.T) {
	runsc, tool, probe := os.Getenv("REFORGE_TEST_RUNSC"), os.Getenv("REFORGE_TEST_SANDBOX_TOOL"), os.Getenv("REFORGE_TEST_SANDBOX_PROBE")
	if runsc == "" || tool == "" || probe == "" {
		t.Skip("set explicit pinned local gVisor/tool/probe paths")
	}
	root := t.TempDir()
	for _, dir := range []string{"bin", "opt/reforge", "workspace", "tmp", "home", "proc", "dev"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(probe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "bin/probe"), data, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"bin", "opt", "opt/reforge"} {
		if err = os.Chmod(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(root, "opt/reforge/tool"), nil, 0555); err != nil {
		t.Fatal(err)
	}
	image, err := ImageDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(name string) string {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		return hex.EncodeToString(h[:])
	}
	state := t.TempDir()
	if err = os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := RuntimeConfig{Runsc: runsc, RunscSHA256: digest(runsc), Tool: tool, ToolSHA256: digest(tool), StateRoot: state, Images: map[string]string{image: root}, Development: true, Rootless: true, MemoryBytes: 512 << 20, DiskBytes: 16 << 20, CPUs: 1, MaxProcesses: 128, Fetch: func(ctx context.Context, in WorkspaceRequest) (Snapshot, error) {
		files := []guest.File{{Path: "source.txt", Content: []byte("baseline")}}
		digest, _ := SnapshotDigest(files)
		return Snapshot{CommitSHA: in.CommitSHA, Complete: true, ManifestSHA256: digest, Files: files}, nil
	}}
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	request := WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: strings.Repeat("a", 40), Image: image, Trust: "development-fixture", Timeout: 2 * time.Minute}
	bad := request
	bad.Trust = "untrusted"
	if _, err = runtime.PreparePinnedWorkspace(context.Background(), bad); !errors.Is(err, ErrUnavailable) {
		t.Fatal("development resource exception admitted untrusted code")
	}
	cfg.Development = false
	if _, err = NewRuntime(cfg); !errors.Is(err, ErrUnavailable) {
		t.Fatal("production admitted absent cgroup delegation")
	}
	originalFetch := runtime.config.Fetch
	for _, mode := range []string{"incomplete", "corrupt", "duplicate"} {
		runtime.config.Fetch = func(ctx context.Context, in WorkspaceRequest) (Snapshot, error) {
			snapshot, _ := originalFetch(ctx, in)
			switch mode {
			case "incomplete":
				snapshot.Complete = false
			case "corrupt":
				snapshot.Files[0].Content = []byte("altered")
			case "duplicate":
				snapshot.Files = append(snapshot.Files, snapshot.Files[0])
			}
			return snapshot, nil
		}
		if _, err := runtime.PreparePinnedWorkspace(context.Background(), request); !errors.Is(err, ErrBoundary) {
			t.Fatalf("snapshot %s admitted: %v", mode, err)
		}
	}
	runtime.config.Fetch = originalFetch
	if err = os.WriteFile(filepath.Join(root, "injected"), []byte("changed image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.PreparePinnedWorkspace(context.Background(), request); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mutable image admitted: %v", err)
	}
	if err = os.Remove(filepath.Join(root, "injected")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntime(runtime.config); !errors.Is(err, ErrUnavailable) {
		t.Fatal("concurrent supervisor admitted for same state root")
	}
	t.Setenv("REFORGE_SANDBOX_CANARY", "supervisor-only")
	w, err := runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if w.ID != "" {
			if err := runtime.Destroy(context.Background(), w); err != nil {
				t.Error(err)
			}
		}
	}()
	execute := func(arg string, limit int64, timeout time.Duration) CommandResult {
		t.Helper()
		result, err := runtime.ExecuteBoundedCommand(context.Background(), w, Command{Args: []string{"/bin/probe", arg}, Directory: "/workspace", Timeout: timeout, MaxOutputBytes: limit, NetworkProfile: "none"})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := execute("inspect", 4096, 10*time.Second); result.ExitCode != 0 || !strings.Contains(string(result.Output), "isolated") {
		t.Fatalf("boundary probe failed: %s", result.Output)
	}
	if err = runtime.ApplyPatch(context.Background(), w, []Patch{{Path: "../escape", Content: []byte("bad")}}); err == nil {
		t.Fatal("path traversal allowed")
	}
	if err = runtime.ApplyPatch(context.Background(), w, []Patch{{Path: "source.txt", Content: []byte("candidate")}}); err != nil {
		t.Fatal(err)
	}
	if result := execute("read", 4096, 10*time.Second); result.ExitCode != 0 || string(result.Output) != "candidate" {
		t.Fatalf("patch/read failed: %s", result.Output)
	}
	if artifact, err := runtime.CollectArtifact(context.Background(), w, "source.txt"); err != nil || string(artifact.Data) != "candidate" {
		t.Fatal("artifact did not preserve bounded source")
	}
	liveState, _ := runtime.state(w)
	stateResult, e := runtime.invoke(context.Background(), liveState, nil, 65536, false, "state", w.ID)
	if e != nil {
		t.Fatal(e)
	}
	var nativeState struct {
		PID    int    `json:"pid"`
		Status string `json:"status"`
	}
	if json.Unmarshal(stateResult.Output, &nativeState) != nil || nativeState.PID < 1 || nativeState.Status != "running" {
		t.Fatalf("missing live sandbox process: %s", stateResult.Output)
	}
	for _, operation := range []string{"read", "patch"} {
		if e := liveState.lock(context.Background()); e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		started := time.Now()
		if operation == "read" {
			_, e = runtime.CollectArtifact(ctx, w, "source.txt")
		} else {
			e = runtime.ApplyPatch(ctx, w, []Patch{{Path: "source.txt", Content: []byte("late")}})
		}
		cancel()
		liveState.unlock()
		if !errors.Is(e, context.DeadlineExceeded) || time.Since(started) > time.Second {
			t.Fatalf("%s ignored waiting deadline: %v", operation, e)
		}
	}
	other, e := runtime.PreparePinnedWorkspace(context.Background(), request)
	if e != nil {
		t.Fatal(e)
	}
	if artifact, e := runtime.CollectArtifact(context.Background(), other, "source.txt"); e != nil || string(artifact.Data) != "baseline" {
		t.Fatalf("cross-workspace source contamination: %v", e)
	}
	if e = runtime.Destroy(context.Background(), other); e != nil {
		t.Fatal(e)
	}
	if result := execute("readonly", 4096, 10*time.Second); result.ExitCode != 0 {
		t.Fatalf("readonly mounts failed: %s", result.Output)
	}
	if result := execute("fifo", 4096, 10*time.Second); result.ExitCode != 0 {
		t.Fatalf("fifo fixture failed: %s", result.Output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if _, e := runtime.CollectArtifact(ctx, w, "fifo"); e == nil {
		t.Fatal("FIFO artifact allowed")
	}
	cancel()
	if result := execute("symlink", 4096, 10*time.Second); result.ExitCode != 0 {
		t.Fatalf("symlink fixture failed: %s", result.Output)
	}
	if _, err = runtime.CollectArtifact(context.Background(), w, "escape"); err == nil {
		t.Fatal("artifact followed symlink outside workspace")
	}
	if result := execute("disk", 4096, 10*time.Second); result.ExitCode != 0 || !strings.Contains(string(result.Output), "enforced") {
		t.Fatalf("disk ceiling missing: %s", result.Output)
	}
	if result := execute("flood", 4096, 10*time.Second); !result.Truncated || len(result.Output) != 4096 {
		t.Fatal("output not bounded")
	}
	result := execute("hang", 4096, 1500*time.Millisecond)
	if !result.TimedOut || !strings.Contains(string(result.Output), "descendant-started") {
		t.Fatal("command timeout not reported")
	}
	processStopped := false
	for i := 0; i < 100; i++ {
		data, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", nativeState.PID))
		if errors.Is(e, os.ErrNotExist) || e == nil && strings.Contains(string(data), ") Z ") {
			processStopped = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !processStopped {
		t.Fatalf("sentry PID %d survived cancellation with setsid descendant", nativeState.PID)
	}
	select {
	case <-liveState.done:
	default:
		t.Fatal("managed runsc process survived cancellation")
	}
	if _, err = runtime.CollectArtifact(context.Background(), w, "alive"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancelled descendants retained usable workspace")
	}
	if err = runtime.Destroy(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	w = Workspace{}
	parent, cancelParent := context.WithCancel(context.Background())
	parentWorkspace, e := runtime.PreparePinnedWorkspace(parent, request)
	if e != nil {
		t.Fatal(e)
	}
	parentState, _ := runtime.state(parentWorkspace)
	cancelParent()
	select {
	case <-parentState.done:
	case <-time.After(5 * time.Second):
		t.Fatal("parent context did not terminate sandbox")
	}
	if _, e = runtime.CollectArtifact(context.Background(), parentWorkspace, "source.txt"); !errors.Is(e, ErrUnavailable) {
		t.Fatalf("file operation ignored expired parent: %v", e)
	}
	if e = runtime.Destroy(context.Background(), parentWorkspace); e != nil {
		t.Fatal(e)
	}
	orphan, err := runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	orphanState, _ := runtime.state(orphan)
	runtime.mu.Lock()
	runtime.closed = true
	delete(runtime.workspaces, orphan.ID)
	err = runtime.lockFile.Close()
	runtime.lockFile = nil
	runtime.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntime(runtime.config)
	if err != nil {
		orphanState.cancel()
		runtime.stop(orphanState)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	defer orphanState.cancel()
	select {
	case <-orphanState.done:
	case <-time.After(5 * time.Second):
		t.Fatal("restart did not terminate orphan sandbox")
	}
	if _, err = os.Stat(orphanState.bundle); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan bundle retained: %v", err)
	}
	afterRestart, err := restarted.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	failingState, _ := restarted.state(afterRestart)
	another, err := restarted.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	anotherState, _ := restarted.state(another)
	if anotherState.workspace.ID < failingState.workspace.ID {
		failingState, anotherState = anotherState, failingState
	}
	if err = os.Chmod(failingState.bundle, 0000); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Close(); err == nil {
		t.Fatal("cleanup failure was not returned")
	}
	select {
	case <-anotherState.done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close left later workspace alive after first cleanup error")
	}
	if _, err = NewRuntime(restarted.config); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unresolved cleanup released state lock")
	}
	if err = os.Chmod(failingState.bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Close(); err != nil {
		t.Fatal(err)
	}
	finalRuntime, err := NewRuntime(restarted.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := finalRuntime.Close(); err != nil {
			t.Error(err)
		}
	})
	registeredBundle, err := os.MkdirTemp(state, "bundle-")
	if err != nil {
		t.Fatal(err)
	}
	pendingCtx, pendingCancel := context.WithCancel(context.Background())
	defer pendingCancel()
	pending := &workspaceState{mu: make(chan struct{}, 1), workspace: Workspace{ID: "rf-00000000-0000-0000-0000-000000000001", Image: image, Root: "/workspace", CommitSHA: request.CommitSHA}, bundle: registeredBundle, ctx: pendingCtx, cancel: pendingCancel}
	finalRuntime.mu.Lock()
	finalRuntime.workspaces[pending.workspace.ID] = pending
	finalRuntime.mu.Unlock()
	if err = finalRuntime.Close(); err != nil {
		t.Fatal(err)
	}
	if err = finalRuntime.start(context.Background(), pending); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("registered workspace started after Close: %v", err)
	}
	if pending.process != nil {
		t.Fatal("Close/start race spawned a process")
	}

}
