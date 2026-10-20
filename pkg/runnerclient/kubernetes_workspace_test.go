package runnerclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

type recordingSandboxRuntime struct {
	requests  []sandbox.WorkspaceRequest
	patches   [][]sandbox.Patch
	commands  []sandbox.Command
	artifacts map[string][]byte
	result    sandbox.CommandResult
	execErr   error
	destroyed []sandbox.Workspace
	events    []string
}

func (r *recordingSandboxRuntime) PreparePinnedWorkspace(_ context.Context, request sandbox.WorkspaceRequest) (sandbox.Workspace, error) {
	r.requests = append(r.requests, request)
	r.events = append(r.events, "prepare")
	return sandbox.Workspace{ID: "workspace", CommitSHA: request.CommitSHA, Root: "/workspace", Image: request.Image}, nil
}

func (r *recordingSandboxRuntime) ExecuteBoundedCommand(_ context.Context, _ sandbox.Workspace, command sandbox.Command) (sandbox.CommandResult, error) {
	r.commands = append(r.commands, command)
	r.events = append(r.events, "execute")
	return r.result, r.execErr
}

func (r *recordingSandboxRuntime) ApplyPatch(_ context.Context, _ sandbox.Workspace, patches []sandbox.Patch) error {
	r.patches = append(r.patches, append([]sandbox.Patch(nil), patches...))
	r.events = append(r.events, "patch")
	return nil
}

func (r *recordingSandboxRuntime) CollectArtifact(_ context.Context, _ sandbox.Workspace, name string) (sandbox.Artifact, error) {
	body, ok := r.artifacts[name]
	if !ok {
		return sandbox.Artifact{}, errors.New("artifact missing")
	}
	return sandbox.Artifact{Name: name, Data: body}, nil
}

func (r *recordingSandboxRuntime) Destroy(_ context.Context, workspace sandbox.Workspace) error {
	r.destroyed = append(r.destroyed, workspace)
	r.events = append(r.events, "destroy")
	return nil
}

func kubernetesRuntimeConfig() sandbox.RuntimeConfig {
	return sandbox.RuntimeConfig{Backend: "kubernetes", Kubernetes: &sandbox.KubernetesRuntimeConfig{Toolchains: map[string]string{"go": "sha256:go-image", "javascript": "sha256:js-image", "python": "sha256:py-image"}}}
}

func TestKubernetesWorkspaceBootstrapsPatchedGoModulesOnce(t *testing.T) {
	files := []guest.File{{Path: "go.mod", Content: []byte("module example.test/root\n")}, {Path: "go.sum", Content: []byte("example.test/root v1.0.0 h1:old\n")}, {Path: "go.work", Content: []byte("go 1.20\n\nuse .\nuse ./nested\n")}, {Path: "nested/go.mod", Content: []byte("module example.test/nested\n")}, {Path: "nested/go.sum", Content: []byte("example.test/nested v1.0.0 h1:old\n")}, {Path: "go.work.sum", Content: []byte("example.test/work v1.0.0 h1:old\n")}}
	snapshot := sandbox.Snapshot{CommitSHA: strings.Repeat("a", 40), Files: files}
	runtime := &recordingSandboxRuntime{}
	request := sandbox.WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: snapshot.CommitSHA, Image: "sha256:requested", Timeout: time.Minute}
	patch := sandbox.Patch{Path: "go.mod", Content: []byte("module example.test/root\n\nrequire example.test/lib v1.2.3\n")}
	workspace, err := (commandPreparer{cfg: kubernetesRuntimeConfig(), runtime: runtime, fetch: func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) { return snapshot, nil }}).prepare(context.Background(), request, []sandbox.Patch{patch}, sandbox.Command{Args: []string{"go", "test", "./..."}, Directory: "."})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Image != "sha256:go-image" || len(runtime.requests) != 1 || runtime.requests[0].Image != "sha256:go-image" || runtime.requests[0].Dependencies != "" || runtime.requests[0].Egress != "registry" {
		t.Fatalf("prepared request=%+v workspace=%+v", runtime.requests, workspace)
	}
	if len(runtime.patches) != 1 || len(runtime.patches[0]) != 1 || string(runtime.patches[0][0].Content) != string(patch.Content) {
		t.Fatalf("staged patch missing: %+v", runtime.patches)
	}
	if len(runtime.commands) != 1 || runtime.commands[0].NetworkProfile != "egress" || runtime.commands[0].Timeout != 10*time.Minute {
		t.Fatalf("Go bootstrap commands=%+v", runtime.commands)
	}
	bootstrap := strings.Join(runtime.commands[0].Args, " ")
	for _, want := range []string{"go mod download", "root=/tmp/reforge-manifests", "GOMODCACHE=/tmp/gomod", "GOFLAGS=-mod=mod", "go.mod go.sum go.work go.work.sum nested/go.mod nested/go.sum -- . nested"} {
		if !strings.Contains(bootstrap, want) {
			t.Fatalf("Go bootstrap must resolve copied manifests in scratch: missing %q in %s", want, bootstrap)
		}
	}
	if !strings.Contains(bootstrap, `cp "/workspace/$file" "$root/$file"`) {
		t.Fatalf("Go bootstrap can write into the source tree: %s", bootstrap)
	}
	if strings.Contains(strings.Join(runtime.events, ","), "destroy") || strings.Join(runtime.events, ",") != "prepare,patch,execute" {
		t.Fatalf("setup ordering=%v", runtime.events)
	}
}

func TestKubernetesWorkspaceSelectsStackAndKeepsPythonOffline(t *testing.T) {
	request := sandbox.WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: strings.Repeat("b", 40), Timeout: time.Minute}
	t.Run("npm", func(t *testing.T) {
		snapshot := sandbox.Snapshot{CommitSHA: request.CommitSHA, Files: []guest.File{{Path: "web/package.json", Content: []byte(`{"name":"app"}`)}, {Path: "web/package-lock.json", Content: []byte(`{"lockfileVersion":3}`)}}}
		runtime := &recordingSandboxRuntime{}
		request.Image = "sha256:requested"
		_, err := (commandPreparer{cfg: kubernetesRuntimeConfig(), runtime: runtime, fetch: func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) { return snapshot, nil }}).prepare(context.Background(), request, nil, sandbox.Command{Args: []string{"npm", "test"}, Directory: "web"})
		if err != nil {
			t.Fatal(err)
		}
		if runtime.requests[0].Image != "sha256:js-image" || runtime.requests[0].Egress != "registry" || runtime.requests[0].Dependencies != "" {
			t.Fatalf("npm request=%+v", runtime.requests[0])
		}
		if len(runtime.commands) != 1 || runtime.commands[0].Directory != "web" || strings.Join(runtime.commands[0].Args[1:], " ") != npmCLI+" ci --ignore-scripts --no-audit --no-fund --registry "+npmRegistry {
			t.Fatalf("npm bootstrap=%+v", runtime.commands)
		}
	})
	t.Run("python", func(t *testing.T) {
		snapshot := sandbox.Snapshot{CommitSHA: request.CommitSHA, Files: []guest.File{{Path: "test_app.py", Content: []byte("pass\n")}}}
		runtime := &recordingSandboxRuntime{}
		request.Image = "sha256:requested"
		_, err := (commandPreparer{cfg: kubernetesRuntimeConfig(), runtime: runtime, fetch: func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) { return snapshot, nil }}).prepare(context.Background(), request, nil, sandbox.Command{Args: []string{"python3", "-m", "unittest"}})
		if err != nil {
			t.Fatal(err)
		}
		if runtime.requests[0].Image != "sha256:py-image" || runtime.requests[0].Egress != "" || len(runtime.commands) != 0 {
			t.Fatalf("Python should prepare offline: request=%+v commands=%+v", runtime.requests, runtime.commands)
		}
	})
}

func TestKubernetesWorkspaceDestroysOnBootstrapFailure(t *testing.T) {
	snapshot := sandbox.Snapshot{CommitSHA: strings.Repeat("c", 40), Files: []guest.File{{Path: "go.mod", Content: []byte("module example.test/app\n")}}}
	runtime := &recordingSandboxRuntime{result: sandbox.CommandResult{ExitCode: 1, Output: []byte("download failed")}}
	request := sandbox.WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: snapshot.CommitSHA, Timeout: time.Minute}
	_, err := (commandPreparer{cfg: kubernetesRuntimeConfig(), runtime: runtime, fetch: func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) { return snapshot, nil }}).prepare(context.Background(), request, nil, sandbox.Command{Args: []string{"go", "test"}})
	var setupErr *sandbox.CommandSetupError
	if !errors.As(err, &setupErr) || len(runtime.destroyed) != 1 || strings.Join(runtime.events, ",") != "prepare,execute,destroy" {
		t.Fatalf("bootstrap failure cleanup: err=%v destroyed=%d events=%v", err, len(runtime.destroyed), runtime.events)
	}
}

func TestKubernetesDependencyUpdaterUsesPinnedToolchainAndRegistryLease(t *testing.T) {
	files := map[string][]byte{"go.mod": []byte("module example.test/app\n"), "go.sum": []byte("")}
	runtime := &recordingSandboxRuntime{artifacts: map[string][]byte{"go.mod": []byte("module example.test/app\n\nrequire example.test/lib v1.2.3\n"), "go.sum": []byte("example.test/lib v1.2.3 h1:abc\n")}}
	request := sandbox.WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: strings.Repeat("d", 40), Timeout: time.Minute}
	updated, err := (updater{cfg: kubernetesRuntimeConfig(), runtime: runtime, request: request, target: files}).update(context.Background(), files, repair.DependencyUpdate{Ecosystem: "go", Directory: ".", Package: "example.test/lib", Version: "v1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated["go.mod"]) == string(files["go.mod"]) || runtime.requests[0].Image != "sha256:go-image" || runtime.requests[0].Egress != "registry" || runtime.requests[0].Dependencies != "" {
		t.Fatalf("updated=%q request=%+v", updated["go.mod"], runtime.requests)
	}
	if len(runtime.commands) != 1 || runtime.commands[0].NetworkProfile != "egress" || strings.Contains(strings.Join(runtime.commands[0].Args, " "), "/opt/reforge/tool egress") || !strings.Contains(strings.Join(runtime.commands[0].Args, " "), "go get example.test/lib@v1.2.3") {
		t.Fatalf("Kubernetes update command=%+v", runtime.commands)
	}
	if len(runtime.destroyed) != 1 {
		t.Fatalf("workspace cleanup count=%d", len(runtime.destroyed))
	}
}

func TestKubernetesNPMUpdaterUsesJavascriptImageAndLockfileOnly(t *testing.T) {
	files := map[string][]byte{"web/package.json": []byte(`{"name":"web","dependencies":{"pkg":"^1.0.0"}}`), "web/package-lock.json": []byte(`{"lockfileVersion":3}`)}
	updatedFiles := map[string][]byte{"web/package.json": []byte(`{"name":"web","dependencies":{"pkg":"^1.1.0"}}`), "web/package-lock.json": []byte(`{"lockfileVersion":3,"updated":true}`)}
	runtime := &recordingSandboxRuntime{artifacts: updatedFiles}
	request := sandbox.WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: strings.Repeat("e", 40), Timeout: time.Minute}
	updated, err := (updater{cfg: kubernetesRuntimeConfig(), runtime: runtime, request: request, target: files}).update(context.Background(), files, repair.DependencyUpdate{Ecosystem: "npm", Directory: "web", Package: "pkg", Version: "1.1.0", Strategy: "update"})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.requests[0].Image != "sha256:js-image" || runtime.requests[0].Egress != "registry" || runtime.requests[0].Dependencies != "" || string(updated["web/package-lock.json"]) != string(updatedFiles["web/package-lock.json"]) {
		t.Fatalf("updated=%v request=%+v", updated, runtime.requests)
	}
	args := strings.Join(runtime.commands[0].Args, " ")
	if runtime.commands[0].NetworkProfile != "egress" || runtime.commands[0].Directory != "web" || !strings.Contains(args, npmCLI+" update pkg --package-lock-only --ignore-scripts --no-audit --no-fund --registry "+npmRegistry) {
		t.Fatalf("npm update command=%+v", runtime.commands[0])
	}
}

func TestKubernetesMixedStackSelectsCommandToolchain(t *testing.T) {
	snapshot := sandbox.Snapshot{CommitSHA: strings.Repeat("f", 40), Files: []guest.File{
		{Path: "go.mod", Content: []byte("module example.test/app\n")},
		{Path: "web/package.json", Content: []byte(`{"name":"web"}`)},
		{Path: "web/package-lock.json", Content: []byte(`{"lockfileVersion":3}`)},
	}}
	request := sandbox.WorkspaceRequest{JobID: "job", AttemptID: "attempt", CommitSHA: snapshot.CommitSHA, Timeout: time.Minute}
	for _, test := range []struct {
		name, executable, wantImage, wantBootstrap string
	}{
		{name: "go", executable: "go", wantImage: "sha256:go-image", wantBootstrap: "go mod download"},
		{name: "javascript", executable: "node", wantImage: "sha256:js-image", wantBootstrap: "npm-cli.js ci"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &recordingSandboxRuntime{}
			request.Image = "sha256:requested"
			_, err := (commandPreparer{cfg: kubernetesRuntimeConfig(), runtime: runtime, fetch: func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) { return snapshot, nil }}).prepare(context.Background(), request, nil, sandbox.Command{Args: []string{test.executable, "test"}, Directory: "web"})
			if err != nil {
				t.Fatal(err)
			}
			if runtime.requests[0].Image != test.wantImage || len(runtime.commands) != 1 || !strings.Contains(strings.Join(runtime.commands[0].Args, " "), test.wantBootstrap) {
				t.Fatalf("image=%q commands=%+v", runtime.requests[0].Image, runtime.commands)
			}
		})
	}
}
