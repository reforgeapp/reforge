package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

type fakePodClient struct {
	spec       PodSpec
	pod        Pod
	deleted    []PodRef
	commands   [][]string
	getCount   int
	list       []Pod
	getHook    func(Pod) Pod
	execHook   func(context.Context, PodRef, []string, io.Reader, io.Writer, io.Writer) (int, error)
	failApply  bool
	commandOut []byte
	events     *[]string
}

func (f *fakePodClient) Create(_ context.Context, spec PodSpec) (Pod, error) {
	f.spec = spec
	f.pod = Pod{Ref: PodRef{Namespace: spec.Ref.Namespace, Name: spec.Ref.Name, UID: "pod-uid"}, Phase: "Running", CreatedAt: time.Now(), ActiveDeadlineSeconds: spec.ActiveDeadlineSeconds, Labels: copyLabels(spec.Labels)}
	return f.pod, nil
}

func (f *fakePodClient) List(_ context.Context, _ map[string]string) ([]Pod, error) {
	return append([]Pod(nil), f.list...), nil
}

func (f *fakePodClient) Get(_ context.Context, ref PodRef) (Pod, error) {
	f.getCount++
	if ref.Namespace != f.pod.Ref.Namespace || ref.Name != f.pod.Ref.Name || ref.UID != f.pod.Ref.UID {
		return Pod{}, ErrBoundary
	}
	pod := f.pod
	if f.getHook != nil {
		pod = f.getHook(pod)
	}
	return pod, nil
}

func (f *fakePodClient) Exec(ctx context.Context, ref PodRef, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	f.commands = append(f.commands, append([]string(nil), args...))
	if f.events != nil {
		*f.events = append(*f.events, "exec")
	}
	if f.execHook != nil {
		return f.execHook(ctx, ref, args, stdin, stdout, stderr)
	}
	if len(args) == 1 && args[0] == "/opt/reforge/tool" {
		var request guest.Request
		if err := json.NewDecoder(stdin).Decode(&request); err != nil {
			return 1, err
		}
		if request.Operation == "apply" && f.failApply {
			return 1, nil
		}
		if request.Operation == "read" {
			_, _ = io.WriteString(stdout, `{"content":"bG9nIG91dHB1dA=="}`)
		} else {
			_, _ = io.WriteString(stdout, "{}\n")
		}
		return 0, nil
	}
	_, _ = stdout.Write(f.commandOut)
	return 0, nil
}

func (f *fakePodClient) Delete(_ context.Context, ref PodRef) error {
	f.deleted = append(f.deleted, ref)
	return nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func copyLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func newTestRuntime(t *testing.T, client *fakePodClient, fetch func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error)) *Runtime {
	t.Helper()
	rootDigest := "sha256:" + strings.Repeat("a", 64)
	cfg := Config{
		Namespace:    "reforge",
		RunnerID:     "runner-1",
		Images:       map[string]string{rootDigest: "ghcr.io/reforge/workspace-javascript@sha256:" + strings.Repeat("a", 64)},
		MemoryBytes:  512 << 20,
		DiskBytes:    128 << 20,
		CPUs:         2,
		MaxProcesses: 128,
		Fetch:        fetch,
		Client:       client,
		PollInterval: time.Millisecond,
	}
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func testRequest() sandbox.WorkspaceRequest {
	return sandbox.WorkspaceRequest{
		JobID:     "job-1",
		AttemptID: "attempt-1",
		CommitSHA: strings.Repeat("c", 40),
		Image:     "sha256:" + strings.Repeat("a", 64),
		Timeout:   time.Minute,
	}
}

func testSnapshot(request sandbox.WorkspaceRequest) sandbox.Snapshot {
	files := []guest.File{{Path: "package.json", Content: []byte(`{"scripts":{"test":"node --test"}}`)}}
	digest, _ := sandbox.SnapshotDigest(files)
	return sandbox.Snapshot{CommitSHA: request.CommitSHA, Complete: true, ManifestSHA256: digest, Files: files}
}

func TestRuntimeCleansOnlyOwnedTerminalOrExpiredPods(t *testing.T) {
	now := time.Now()
	labels := func(runner string) map[string]string {
		return map[string]string{"app.kubernetes.io/name": "reforge-workspace", "reforge.io/runner-id": runner}
	}
	client := &fakePodClient{list: []Pod{
		{Ref: PodRef{Namespace: "reforge", Name: "rf-ws-terminal", UID: "terminal"}, Phase: "Failed", Labels: labels("runner-1")},
		{Ref: PodRef{Namespace: "reforge", Name: "rf-ws-stale", UID: "stale"}, Phase: "Running", CreatedAt: now.Add(-20 * time.Minute), ActiveDeadlineSeconds: 60, Labels: labels("runner-1")},
		{Ref: PodRef{Namespace: "reforge", Name: "rf-ws-active", UID: "active"}, Phase: "Running", CreatedAt: now, ActiveDeadlineSeconds: 60, Labels: labels("runner-1")},
		{Ref: PodRef{Namespace: "reforge", Name: "rf-ws-other", UID: "other"}, Phase: "Failed", Labels: labels("runner-2")},
		{Ref: PodRef{Namespace: "other", Name: "rf-ws-namespace", UID: "namespace"}, Phase: "Failed", Labels: labels("runner-1")},
	}}
	_ = newTestRuntime(t, client, func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return sandbox.Snapshot{}, nil
	})
	if len(client.deleted) != 2 || client.deleted[0].UID != "terminal" || client.deleted[1].UID != "stale" {
		t.Fatalf("orphan cleanup deleted wrong pods: %+v", client.deleted)
	}
}

func TestRuntimeCloseDestroysOwnedPodsAndClosesBootstrapper(t *testing.T) {
	client := &fakePodClient{}
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	request := testRequest()
	request.Egress = "registry"
	closed := 0
	lease := &fakeBootstrapLease{refs: BootstrapRefs{Egress: "registry"}, config: guest.TCPEgressRequest{Address: "10.0.0.2:8086", Token: strings.Repeat("A", 32)}}
	runtime.config.Bootstrapper = fakeBootstrapper{lease: lease, closed: &closed}
	workspace, err := runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ID == "" {
		t.Fatal("workspace ID missing")
	}
	if err = runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if len(client.deleted) != 1 || lease.closes != 1 || closed != 1 {
		t.Fatalf("shutdown deleted=%+v lease closes=%d bootstrap close=%d", client.deleted, lease.closes, closed)
	}
}

func TestRuntimeWorkspaceLifecycleAndPodSecurity(t *testing.T) {
	client := &fakePodClient{commandOut: []byte("bounded output")}
	request := testRequest()
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	workspace, err := runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if client.spec.RuntimeClassName != "" || client.spec.ContainerName != "workspace" || client.spec.WorkingDirectory != "/workspace" || client.spec.Image != "ghcr.io/reforge/workspace-javascript@sha256:"+strings.Repeat("a", 64) || client.spec.RunAsUser != 65532 || !client.spec.RunAsNonRoot || client.spec.RunAsGroup != 65532 || client.spec.FSGroup != 65532 || !client.spec.ReadOnlyRootFilesystem || client.spec.AllowPrivilegeEscalation || client.spec.AutomountServiceAccountToken || client.spec.HostNetwork || client.spec.HostPID || client.spec.HostIPC || len(client.spec.HostPaths) != 0 || !slicesEqual(client.spec.DropCapabilities, []string{"ALL"}) || client.spec.SeccompProfile != "RuntimeDefault" {
		t.Fatalf("unsafe or incomplete pod spec: %+v", client.spec)
	}
	if client.spec.WorkspaceEmptyDirBytes != runtime.config.DiskBytes/2 || client.spec.TempEmptyDirBytes != runtime.config.DiskBytes/2 || client.spec.ActiveDeadlineSeconds != 60 || client.spec.RestartPolicy != "Never" || client.spec.NetworkProfile != "none" || !contains(client.spec.Environment, "HOME=/tmp") || !contains(client.spec.Environment, "GOTOOLCHAIN=local") || !contains(client.spec.Environment, "GOFLAGS=-mod=readonly") || !contains(client.spec.Environment, "GOMODCACHE=/tmp/gomod") || !contains(client.spec.Environment, "GOPROXY=off") || !contains(client.spec.Environment, "GOSUMDB=off") {
		t.Fatalf("workspace bounds missing: %+v", client.spec)
	}
	if err = runtime.ApplyPatch(context.Background(), workspace, []sandbox.Patch{{Path: "value.js", Content: []byte("exports.ok = true")}}); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.ExecuteBoundedCommand(context.Background(), workspace, sandbox.Command{Args: []string{"node", "--test"}, Directory: "pkg", Timeout: time.Second, MaxOutputBytes: 5, NetworkProfile: "none"})
	if err != nil || result.ExitCode != 0 || !result.Truncated || string(result.Output) != "bound" {
		t.Fatalf("command result=%+v error=%v", result, err)
	}
	last := client.commands[len(client.commands)-1]
	if len(last) < 7 || last[0] != "/bin/sh" || last[4] != "pkg" || last[5] != "node" || last[6] != "--test" {
		t.Fatalf("command directory or args lost: %q", last)
	}
	artifact, err := runtime.CollectArtifact(context.Background(), workspace, "test.log")
	if err != nil || string(artifact.Data) != "log output" {
		t.Fatalf("artifact=%+v error=%v", artifact, err)
	}
	if err = runtime.Destroy(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	if len(client.deleted) != 1 || client.deleted[0].UID != "pod-uid" {
		t.Fatalf("pod delete refs=%+v", client.deleted)
	}
}

func TestRuntimeFailsClosedForBadSnapshotAndUnbootstrappedInputs(t *testing.T) {
	request := testRequest()
	client := &fakePodClient{}
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		snapshot := testSnapshot(got)
		snapshot.ManifestSHA256 = strings.Repeat("0", 64)
		return snapshot, nil
	})
	if _, err := runtime.PreparePinnedWorkspace(context.Background(), request); !errors.Is(err, ErrBoundary) || len(client.deleted) != 0 {
		t.Fatalf("bad snapshot accepted or pod created: %v", err)
	}
	runtime = newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	request.Dependencies = "/var/cache/dependencies"
	if _, err := runtime.PreparePinnedWorkspace(context.Background(), request); !errors.Is(err, ErrBoundary) || client.spec.Ref.Name != "" {
		t.Fatalf("dependencies silently accepted: %v", err)
	}
	request = testRequest()
	request.Egress = "/tmp/egress.sock"
	if _, err := runtime.PreparePinnedWorkspace(context.Background(), request); !errors.Is(err, ErrBoundary) || client.spec.Ref.Name != "" {
		t.Fatalf("host egress silently accepted: %v", err)
	}
}

func TestRuntimeDeletesPodOnPartialSetupFailure(t *testing.T) {
	client := &fakePodClient{failApply: true}
	request := testRequest()
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	if _, err := runtime.PreparePinnedWorkspace(context.Background(), request); err == nil {
		t.Fatal("failed initial snapshot transfer accepted")
	}
	if len(client.deleted) != 1 || client.deleted[0].UID != "pod-uid" {
		t.Fatalf("partial pod was not deleted: %+v", client.deleted)
	}
}

func TestRuntimeTimeoutDeletesPodAndRejectsChangedUID(t *testing.T) {
	client := &fakePodClient{}
	request := testRequest()
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	workspace, err := runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	client.execHook = func(ctx context.Context, _ PodRef, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	result, err := runtime.ExecuteBoundedCommand(context.Background(), workspace, sandbox.Command{Args: []string{"sleep", "1"}, Timeout: 10 * time.Millisecond, MaxOutputBytes: 1024, NetworkProfile: "none"})
	if err != nil || !result.TimedOut || len(client.deleted) != 1 {
		t.Fatalf("timeout result=%+v err=%v deletes=%+v", result, err, client.deleted)
	}
	if _, err = runtime.CollectArtifact(context.Background(), workspace, "x.log"); !errors.Is(err, ErrBoundary) {
		t.Fatalf("timed-out workspace still available: %v", err)
	}

	client = &fakePodClient{}
	runtime = newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	workspace, err = runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	client.getHook = func(pod Pod) Pod { pod.Ref.UID = "replaced-uid"; return pod }
	_, err = runtime.ExecuteBoundedCommand(context.Background(), workspace, sandbox.Command{Args: []string{"true"}, Timeout: time.Second, MaxOutputBytes: 1024, NetworkProfile: "none"})
	if !errors.Is(err, ErrBoundary) {
		t.Fatalf("replacement pod UID accepted: %v", err)
	}
}

func TestBoundedOutputCapsStream(t *testing.T) {
	var output boundedOutput
	output.limit = 4
	_, _ = io.Copy(&output, bytes.NewBufferString("abcdefgh"))
	if string(output.data) != "abcd" || !output.truncated {
		t.Fatalf("output=%q truncated=%t", output.data, output.truncated)
	}
}

type fakeBootstrapLease struct {
	refs       BootstrapRefs
	egressEnds int
	closes     int
	config     guest.TCPEgressRequest
	events     *[]string
}

func (l *fakeBootstrapLease) Refs() BootstrapRefs                  { return l.refs }
func (l *fakeBootstrapLease) EgressConfig() guest.TCPEgressRequest { return l.config }
func (l *fakeBootstrapLease) EndEgress(context.Context) error {
	l.egressEnds++
	if l.events != nil {
		*l.events = append(*l.events, "revoke")
	}
	return nil
}
func (l *fakeBootstrapLease) Close(context.Context) error {
	l.closes++
	return nil
}

type fakeBootstrapper struct {
	lease  *fakeBootstrapLease
	closed *int
}

func (b fakeBootstrapper) Close() error {
	if b.closed != nil {
		*b.closed++
	}
	return nil
}

func (b fakeBootstrapper) Prepare(context.Context, sandbox.WorkspaceRequest, string) (BootstrapLease, error) {
	return b.lease, nil
}

func TestRuntimeUsesBootstrapRefsAndRevokesEgressAtCommandBoundaries(t *testing.T) {
	events := []string{}
	client := &fakePodClient{events: &events}
	request := testRequest()
	request.Egress = "registry"
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	runtime.config.RuntimeClassName = "gvisor"
	token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	lease := &fakeBootstrapLease{refs: BootstrapRefs{Egress: "registry"}, config: guest.TCPEgressRequest{Address: "10.0.0.2:8443", Token: token}, events: &events}
	runtime.config.Bootstrapper = fakeBootstrapper{lease: lease}
	workspace, err := runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if client.spec.RuntimeClassName != "gvisor" || client.spec.Bootstrap != lease.refs {
		t.Fatalf("runtime class/bootstrap refs missing: %+v", client.spec)
	}
	encodedSpec, _ := json.Marshal(client.spec)
	if bytes.Contains(encodedSpec, []byte(token)) {
		t.Fatal("egress token appeared in pod spec")
	}
	events = nil
	client.execHook = func(_ context.Context, _ PodRef, args []string, stdin io.Reader, _ io.Writer, _ io.Writer) (int, error) {
		if strings.Contains(strings.Join(args, " "), token) || len(args) < 2 || args[len(args)-2] != "/opt/reforge/tool" || args[len(args)-1] != "egress-tcp" {
			t.Fatalf("unexpected egress command args: %v", args)
		}
		var config guest.TCPEgressRequest
		if err := json.NewDecoder(stdin).Decode(&config); err != nil || config.Token != token || strings.Join(config.Command, " ") != "npm ci" {
			t.Fatalf("egress config=%+v error=%v", config, err)
		}
		return 0, nil
	}
	if _, err = runtime.ExecuteBoundedCommand(context.Background(), workspace, sandbox.Command{Args: []string{"npm", "ci"}, Timeout: time.Second, MaxOutputBytes: 1024, NetworkProfile: "egress"}); err != nil {
		t.Fatal(err)
	}
	if lease.egressEnds != 1 || len(events) != 2 || events[0] != "exec" || events[1] != "revoke" {
		t.Fatalf("egress command/revoke order=%v", events)
	}
	if _, err = runtime.ExecuteBoundedCommand(context.Background(), workspace, sandbox.Command{Args: []string{"npm", "test"}, Timeout: time.Second, MaxOutputBytes: 1024, NetworkProfile: "egress"}); !errors.Is(err, ErrBoundary) {
		t.Fatalf("egress remained available: %v", err)
	}
	if err = runtime.Destroy(context.Background(), workspace); err != nil || lease.closes != 1 {
		t.Fatalf("bootstrap cleanup error=%v closes=%d", err, lease.closes)
	}

	events = nil
	client = &fakePodClient{events: &events}
	runtime = newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	runtime.config.Bootstrapper = fakeBootstrapper{lease: &fakeBootstrapLease{refs: BootstrapRefs{Egress: "registry"}, config: guest.TCPEgressRequest{Address: "10.0.0.2:8443", Token: token}, events: &events}}
	workspace, err = runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	events = nil
	if _, err = runtime.ExecuteBoundedCommand(context.Background(), workspace, sandbox.Command{Args: []string{"go", "test"}, Timeout: time.Second, MaxOutputBytes: 1024, NetworkProfile: "none"}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != "revoke" || events[1] != "exec" {
		t.Fatalf("offline command was not preceded by revocation: %v", events)
	}
}

func TestValidateConfigRequiresOCIImageDigestToMatchApprovalKey(t *testing.T) {
	rootDigest := "sha256:" + strings.Repeat("a", 64)
	config := Config{
		Namespace:   "reforge",
		Images:      map[string]string{rootDigest: "ghcr.io/reforge/workspace-go@sha256:" + strings.Repeat("b", 64)},
		MemoryBytes: 512 << 20, DiskBytes: 128 << 20, CPUs: 2,
	}
	if err := ValidateConfig(config); !errors.Is(err, ErrBoundary) {
		t.Fatalf("digest mismatch accepted: %v", err)
	}
	config.Images[rootDigest] = "ghcr.io/reforge/workspace-go@" + rootDigest
	if err := ValidateConfig(config); err != nil {
		t.Fatalf("matching digest rejected: %v", err)
	}
}

func TestRuntimeRemoteExecErrorDeletesPodAndRevokesEgress(t *testing.T) {
	client := &fakePodClient{}
	request := testRequest()
	request.Egress = "registry"
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	lease := &fakeBootstrapLease{refs: BootstrapRefs{Egress: "registry"}, config: guest.TCPEgressRequest{Address: "10.0.0.2:8443", Token: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}
	runtime.config.Bootstrapper = fakeBootstrapper{lease: lease}
	workspace, err := runtime.PreparePinnedWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	client.execHook = func(context.Context, PodRef, []string, io.Reader, io.Writer, io.Writer) (int, error) {
		return 0, errors.New("exec stream interrupted")
	}
	_, err = runtime.ExecuteBoundedCommand(context.Background(), workspace, sandbox.Command{Args: []string{"npm", "ci"}, Timeout: time.Second, MaxOutputBytes: 1024, NetworkProfile: "egress"})
	if err == nil || lease.egressEnds != 1 || lease.closes != 1 || len(client.deleted) != 1 {
		t.Fatalf("remote exec failure cleanup: err=%v revoke=%d close=%d deleted=%d", err, lease.egressEnds, lease.closes, len(client.deleted))
	}
	if _, err = runtime.state(workspace); !errors.Is(err, ErrBoundary) {
		t.Fatalf("workspace remained usable after uncertain exec: %v", err)
	}
}

func TestValidateConfigBoundsImagePullSecretNames(t *testing.T) {
	base := Config{Namespace: "reforge", Images: map[string]string{"sha256:" + strings.Repeat("a", 64): "ghcr.io/reforge/workspace-go@sha256:" + strings.Repeat("a", 64)}, MemoryBytes: 512 << 20, DiskBytes: 128 << 20, CPUs: 2}
	valid := base
	valid.ImagePullSecrets = []string{"registry-creds", "shared.pull-auth"}
	if err := ValidateConfig(valid); err != nil {
		t.Fatalf("valid image pull secret names rejected: %v", err)
	}
	tooMany := make([]string, 17)
	for i := range tooMany {
		tooMany[i] = "registry-secret-" + strconv.Itoa(i)
	}
	for _, names := range [][]string{{"Registry-creds"}, {"bad..name"}, {"duplicate", "duplicate"}, tooMany} {
		config := base
		config.ImagePullSecrets = names
		if err := ValidateConfig(config); err == nil {
			t.Fatalf("invalid image pull secrets accepted: %#v", names)
		}
	}
}

func TestRuntimeWaitsForCapacityWhileUnschedulable(t *testing.T) {
	gets := 0
	client := &fakePodClient{getHook: func(pod Pod) Pod {
		gets++
		if gets <= 8 {
			pod.Phase, pod.Unschedulable = "Pending", true
		}
		return pod
	}}
	runtime := newTestRuntime(t, client, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	runtime.config.ReadyTimeout = 20 * time.Millisecond
	if _, err := runtime.PreparePinnedWorkspace(context.Background(), testRequest()); err != nil {
		t.Fatalf("workspace failed while waiting for capacity: %v", err)
	}
	if gets <= 8 {
		t.Fatalf("stopped waiting after %d polls", gets)
	}

	full := &fakePodClient{getHook: func(pod Pod) Pod {
		pod.Phase, pod.Unschedulable = "Pending", true
		return pod
	}}
	runtime = newTestRuntime(t, full, func(_ context.Context, got sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
		return testSnapshot(got), nil
	})
	runtime.config.ReadyTimeout = 20 * time.Millisecond
	request := testRequest()
	request.Timeout = time.Second
	if _, err := runtime.PreparePinnedWorkspace(context.Background(), request); !errors.Is(err, sandbox.ErrResourceLimit) {
		t.Fatalf("full cluster error = %v", err)
	}
}
