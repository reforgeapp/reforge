package kubernetes

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

var ErrUnavailable = errors.New("Kubernetes sandbox unavailable")
var ErrBoundary = errors.New("Kubernetes workspace violates execution boundary")

var labelValue = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_.-]{0,61}[A-Za-z0-9])?$`)
var namespaceValue = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var secretNameValue = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?)*$`)
var shaValue = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var imageDigest = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]+)?/[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$`)

type PodRef struct {
	Namespace string
	Name      string
	UID       string
}

type Pod struct {
	Ref                   PodRef
	Phase                 string
	CreatedAt             time.Time
	ActiveDeadlineSeconds int64
	Labels                map[string]string
}

type Resources struct {
	MemoryBytes int64
	CPUs        int64
	DiskBytes   int64
	PidsLimit   int64
}

type BootstrapRefs struct {
	Dependencies string
	Egress       string
}

type PodSpec struct {
	Ref                          PodRef
	Labels                       map[string]string
	Image                        string
	RuntimeClassName             string
	ImagePullSecrets             []string
	ContainerName                string
	WorkingDirectory             string
	Environment                  []string
	RunAsUser                    int64
	RunAsNonRoot                 bool
	RunAsGroup                   int64
	FSGroup                      int64
	AllowPrivilegeEscalation     bool
	ReadOnlyRootFilesystem       bool
	DropCapabilities             []string
	SeccompProfile               string
	AutomountServiceAccountToken bool
	HostNetwork                  bool
	HostPID                      bool
	HostIPC                      bool
	HostPaths                    []string
	WorkspaceEmptyDirBytes       int64
	TempEmptyDirBytes            int64
	NetworkProfile               string
	Resources                    Resources
	ActiveDeadlineSeconds        int64
	RestartPolicy                string
	Bootstrap                    BootstrapRefs
}

type PodClient interface {
	Create(context.Context, PodSpec) (Pod, error)
	Get(context.Context, PodRef) (Pod, error)
	List(context.Context, map[string]string) ([]Pod, error)
	Exec(context.Context, PodRef, []string, io.Reader, io.Writer, io.Writer) (int, error)
	Delete(context.Context, PodRef) error
}

type BootstrapLease interface {
	Refs() BootstrapRefs
	EndEgress(context.Context) error
	Close(context.Context) error
}

type egressConfigProvider interface {
	EgressConfig() guest.TCPEgressRequest
}

type Bootstrapper interface {
	Prepare(context.Context, sandbox.WorkspaceRequest, string) (BootstrapLease, error)
}

type Config struct {
	Namespace        string
	RunnerID         string
	RuntimeClassName string
	ImagePullSecrets []string
	Images           map[string]string
	MemoryBytes      int64
	DiskBytes        int64
	CPUs             int64
	MaxProcesses     int64
	Fetch            func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error)
	Client           PodClient
	Bootstrapper     Bootstrapper
	ReadyTimeout     time.Duration
	PollInterval     time.Duration
}

type Runtime struct {
	config     Config
	mu         sync.Mutex
	workspaces map[string]*workspaceState
	gcMu       sync.Mutex
	lastSweep  time.Time
}

type workspaceState struct {
	mu        sync.Mutex
	workspace sandbox.Workspace
	pod       Pod
	lease     BootstrapLease
	egress    bool
}

func NewRuntime(cfg Config) (*Runtime, error) {
	if cfg.Client == nil || cfg.Fetch == nil || !labelValue.MatchString(cfg.RunnerID) {
		return nil, ErrBoundary
	}
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	images := make(map[string]string, len(cfg.Images))
	for digest, image := range cfg.Images {
		images[digest] = image
	}
	cfg.Images = images
	cfg.ImagePullSecrets = append([]string(nil), cfg.ImagePullSecrets...)
	if cfg.ReadyTimeout < 0 || cfg.ReadyTimeout > 5*time.Minute || cfg.PollInterval < 0 || cfg.PollInterval > time.Minute {
		return nil, ErrBoundary
	}
	if cfg.ReadyTimeout == 0 {
		cfg.ReadyTimeout = 3 * time.Minute
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	runtime := &Runtime{config: cfg, workspaces: map[string]*workspaceState{}}
	cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := runtime.sweepOrphans(cleanup); err != nil {
		return nil, err
	}
	return runtime, nil
}

func ValidateConfig(cfg Config) error {
	if !namespaceValue.MatchString(cfg.Namespace) || len(cfg.Images) == 0 || !validPullSecrets(cfg.ImagePullSecrets) {
		return ErrBoundary
	}
	if cfg.RuntimeClassName != "" && !labelValue.MatchString(cfg.RuntimeClassName) {
		return ErrBoundary
	}
	if cfg.MemoryBytes < 256<<20 || cfg.MemoryBytes > 64<<30 || cfg.DiskBytes < 16<<20 || cfg.DiskBytes > 1<<40 || cfg.CPUs < 1 || cfg.CPUs > 32 || cfg.MaxProcesses != 0 && (cfg.MaxProcesses < 32 || cfg.MaxProcesses > 4096) {
		return ErrBoundary
	}
	for digest, image := range cfg.Images {
		if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 || !shaValue.MatchString(digest[7:]) || !validImageRef(image) || image[strings.Index(image, "@sha256:")+1:] != digest {
			return ErrBoundary
		}
	}
	if cfg.ReadyTimeout < 0 || cfg.ReadyTimeout > 5*time.Minute || cfg.PollInterval < 0 || cfg.PollInterval > time.Minute {
		return ErrBoundary
	}
	return nil
}

func (r *Runtime) PreparePinnedWorkspace(ctx context.Context, in sandbox.WorkspaceRequest) (sandbox.Workspace, error) {
	if in.JobID == "" || in.AttemptID == "" || !labelValue.MatchString(in.JobID) || !labelValue.MatchString(in.AttemptID) || !shaValue.MatchString(in.CommitSHA) || in.Timeout < time.Second || in.Timeout > 2*time.Hour {
		return sandbox.Workspace{}, ErrBoundary
	}
	if in.Dependencies != "" || in.Egress != "" && in.Egress != "registry" {
		return sandbox.Workspace{}, ErrBoundary
	}
	image, ok := r.config.Images[in.Image]
	if !ok {
		return sandbox.Workspace{}, ErrUnavailable
	}
	if err := r.sweepOrphans(ctx); err != nil {
		return sandbox.Workspace{}, err
	}
	prepareCtx, prepareCancel := context.WithTimeout(ctx, in.Timeout)
	defer prepareCancel()
	snapshot, err := r.config.Fetch(prepareCtx, in)
	if err != nil {
		return sandbox.Workspace{}, err
	}
	if snapshot.CommitSHA != in.CommitSHA || !snapshot.Complete || len(snapshot.Files) == 0 || len(snapshot.Files) > 20000 {
		return sandbox.Workspace{}, ErrBoundary
	}
	digest, err := sandbox.SnapshotDigest(snapshot.Files)
	if err != nil || digest != snapshot.ManifestSHA256 {
		return sandbox.Workspace{}, ErrBoundary
	}
	id, err := randomID()
	if err != nil {
		return sandbox.Workspace{}, err
	}
	name := "rf-ws-" + id
	var lease BootstrapLease
	if in.Dependencies != "" || in.Egress != "" {
		if r.config.Bootstrapper == nil {
			return sandbox.Workspace{}, ErrUnavailable
		}
		lease, err = r.config.Bootstrapper.Prepare(prepareCtx, in, name)
		if err != nil {
			return sandbox.Workspace{}, err
		}
	}
	cleanupLease := func() error {
		if lease == nil {
			return nil
		}
		return runCleanup(ctx, lease.Close)
	}
	refs := BootstrapRefs{}
	if lease != nil {
		refs = lease.Refs()
		if refs.Dependencies != "" && !validOpaque(refs.Dependencies) || refs.Egress != "" && !validOpaque(refs.Egress) || (in.Dependencies != "") != (refs.Dependencies != "") || (in.Egress != "") != (refs.Egress != "") {
			return sandbox.Workspace{}, errors.Join(ErrBoundary, cleanupLease())
		}
	}
	labels := map[string]string{
		"app.kubernetes.io/name": "reforge-workspace",
		"reforge.io/workspace":   name,
		"reforge.io/runner-id":   r.config.RunnerID,
		"reforge.io/job-id":      in.JobID,
		"reforge.io/attempt-id":  in.AttemptID,
		"reforge.io/commit":      in.CommitSHA,
	}
	moduleCache := "/tmp/gomod"
	if refs.Dependencies != "" {
		moduleCache = "/opt/deps/go"
	}
	spec := PodSpec{
		Ref:              PodRef{Namespace: r.config.Namespace, Name: name},
		Labels:           labels,
		Image:            image,
		RuntimeClassName: r.config.RuntimeClassName,
		ImagePullSecrets: append([]string(nil), r.config.ImagePullSecrets...),
		ContainerName:    "workspace",
		WorkingDirectory: "/workspace",
		Environment: []string{
			"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
			"HOME=/tmp",
			"TMPDIR=/tmp",
			"GOCACHE=/tmp/go-build",
			"GOMODCACHE=" + moduleCache,
			"GOPROXY=off",
			"GOSUMDB=off",
			"GOPATH=/workspace/.reforge/gopath",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_TERMINAL_PROMPT=0",
			"GOTOOLCHAIN=local",
			"GOFLAGS=-mod=readonly",
			"CI=true",
		},
		RunAsUser:              65532,
		RunAsNonRoot:           true,
		RunAsGroup:             65532,
		FSGroup:                65532,
		ReadOnlyRootFilesystem: true,
		DropCapabilities:       []string{"ALL"},
		SeccompProfile:         "RuntimeDefault",
		WorkspaceEmptyDirBytes: r.config.DiskBytes / 2,
		TempEmptyDirBytes:      r.config.DiskBytes / 2,
		NetworkProfile:         "none",
		Resources:              Resources{MemoryBytes: r.config.MemoryBytes, CPUs: r.config.CPUs, DiskBytes: r.config.DiskBytes, PidsLimit: r.config.MaxProcesses},
		ActiveDeadlineSeconds:  int64((in.Timeout + time.Second - 1) / time.Second),
		RestartPolicy:          "Never",
		Bootstrap:              refs,
	}
	if err = validPodSpec(spec); err != nil {
		return sandbox.Workspace{}, errors.Join(err, cleanupLease())
	}
	pod, err := r.config.Client.Create(prepareCtx, spec)
	if err != nil {
		var existing Pod
		getErr := runCleanup(ctx, func(clean context.Context) error {
			var err error
			existing, err = r.config.Client.Get(clean, spec.Ref)
			return err
		})
		if getErr == nil && existing.Ref.Namespace == spec.Ref.Namespace && existing.Ref.Name == spec.Ref.Name && existing.Ref.UID != "" && sameLabels(labels, existing.Labels) {
			err = errors.Join(err, r.deletePod(ctx, existing.Ref))
		}
		return sandbox.Workspace{}, errors.Join(err, cleanupLease())
	}
	createdRef := pod.Ref
	if pod.Ref.Namespace != spec.Ref.Namespace || pod.Ref.Name != spec.Ref.Name || pod.Ref.UID == "" || !sameLabels(labels, pod.Labels) {
		deleteErr := r.deletePod(ctx, safeDeleteRef(spec.Ref, createdRef))
		return sandbox.Workspace{}, errors.Join(ErrBoundary, deleteErr, cleanupLease())
	}
	pod, err = r.awaitRunning(prepareCtx, pod)
	if err != nil {
		deleteErr := r.deletePod(ctx, createdRef)
		return sandbox.Workspace{}, errors.Join(err, deleteErr, cleanupLease())
	}
	workspace := sandbox.Workspace{ID: name, CommitSHA: in.CommitSHA, Root: "/workspace", Image: in.Image}
	state := &workspaceState{workspace: workspace, pod: pod, lease: lease, egress: refs.Egress != ""}
	if err = r.apply(prepareCtx, state, snapshot.Files); err != nil {
		deleteErr := r.deletePod(ctx, createdRef)
		return sandbox.Workspace{}, errors.Join(err, deleteErr, cleanupLease())
	}
	r.mu.Lock()
	if _, exists := r.workspaces[name]; exists {
		r.mu.Unlock()
		deleteErr := r.deletePod(ctx, createdRef)
		return sandbox.Workspace{}, errors.Join(ErrBoundary, deleteErr, cleanupLease())
	}
	r.workspaces[name] = state
	r.mu.Unlock()
	return workspace, nil
}

func (r *Runtime) sweepOrphans(ctx context.Context) error {
	r.gcMu.Lock()
	defer r.gcMu.Unlock()
	if !r.lastSweep.IsZero() && time.Since(r.lastSweep) < time.Minute {
		return nil
	}
	if err := r.cleanOrphans(ctx); err != nil {
		return err
	}
	r.lastSweep = time.Now()
	return nil
}

func (r *Runtime) cleanOrphans(ctx context.Context) error {
	selector := map[string]string{"app.kubernetes.io/name": "reforge-workspace", "reforge.io/runner-id": r.config.RunnerID}
	pods, err := r.config.Client.List(ctx, selector)
	if err != nil {
		return err
	}
	now := time.Now()
	r.mu.Lock()
	active := make(map[string]bool, len(r.workspaces))
	for _, state := range r.workspaces {
		active[state.pod.Ref.UID] = true
	}
	r.mu.Unlock()
	for _, pod := range pods {
		if pod.Ref.Namespace != r.config.Namespace || pod.Ref.UID == "" || active[pod.Ref.UID] || !strings.HasPrefix(pod.Ref.Name, "rf-ws-") || !sameLabels(selector, pod.Labels) {
			continue
		}
		terminal := pod.Phase == "Succeeded" || pod.Phase == "Failed"
		stale := (pod.Phase == "Pending" || pod.Phase == "Running" || pod.Phase == "Unknown") && !pod.CreatedAt.IsZero() && pod.ActiveDeadlineSeconds > 0 && pod.ActiveDeadlineSeconds <= int64((2*time.Hour)/time.Second) && now.After(pod.CreatedAt.Add(time.Duration(pod.ActiveDeadlineSeconds)*time.Second+15*time.Minute))
		if terminal || stale {
			if err := r.deletePod(ctx, pod.Ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runtime) Close() error {
	r.mu.Lock()
	states := make([]*workspaceState, 0, len(r.workspaces))
	for _, state := range r.workspaces {
		states = append(states, state)
	}
	r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var result error
	for _, state := range states {
		if err := r.Destroy(ctx, state.workspace); err != nil {
			result = errors.Join(result, err)
		}
	}
	if closer, ok := r.config.Bootstrapper.(interface{ Close() error }); ok {
		result = errors.Join(result, closer.Close())
	}
	return result
}

func (r *Runtime) awaitRunning(ctx context.Context, pod Pod) (Pod, error) {
	ready, cancel := context.WithTimeout(ctx, r.config.ReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		next, err := r.config.Client.Get(ready, pod.Ref)
		if err != nil {
			return pod, err
		}
		if err = validPodIdentity(pod, next); err != nil {
			return next, err
		}
		switch next.Phase {
		case "Running":
			return next, nil
		case "Failed", "Succeeded", "Unknown":
			return next, ErrUnavailable
		}
		select {
		case <-ready.Done():
			return next, ErrUnavailable
		case <-ticker.C:
		}
	}
}

func (r *Runtime) ApplyPatch(ctx context.Context, workspace sandbox.Workspace, patches []sandbox.Patch) error {
	state, err := r.state(workspace)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(patches) == 0 || len(patches) > 20000 {
		return ErrBoundary
	}
	files := make([]guest.File, 0, len(patches))
	seen := map[string]bool{}
	total := 0
	for _, patch := range patches {
		total += len(patch.Content)
		if !guest.ValidPath(patch.Path) || seen[patch.Path] || len(patch.Content) > 4<<20 || total > 64<<20 {
			return ErrBoundary
		}
		seen[patch.Path] = true
		files = append(files, guest.File{Path: patch.Path, Content: patch.Content, Delete: patch.Delete})
	}
	return r.apply(ctx, state, files)
}

func (r *Runtime) ExecuteBoundedCommand(ctx context.Context, workspace sandbox.Workspace, command sandbox.Command) (sandbox.CommandResult, error) {
	state, err := r.state(workspace)
	if err != nil {
		return sandbox.CommandResult{}, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(command.Args) == 0 || len(command.Args) > 128 || command.Timeout < time.Millisecond || command.Timeout > 30*time.Minute || command.MaxOutputBytes < 1 || command.MaxOutputBytes > 4<<20 || len(command.Stdin) > 1<<20 || command.NetworkProfile != "none" && command.NetworkProfile != "egress" || command.NetworkProfile == "egress" && !state.egress {
		return sandbox.CommandResult{}, ErrBoundary
	}
	for _, arg := range command.Args {
		if len(arg) > 16384 || strings.ContainsRune(arg, 0) {
			return sandbox.CommandResult{}, ErrBoundary
		}
	}
	directory, err := cleanDirectory(command.Directory)
	if err != nil {
		return sandbox.CommandResult{}, err
	}
	stdin := command.Stdin
	commandArgs := command.Args
	if command.NetworkProfile == "egress" {
		if len(command.Stdin) != 0 || state.lease == nil {
			return sandbox.CommandResult{}, ErrBoundary
		}
		provider, ok := state.lease.(egressConfigProvider)
		if !ok {
			return sandbox.CommandResult{}, ErrBoundary
		}
		config := provider.EgressConfig()
		config.Command = append([]string(nil), command.Args...)
		stdin, err = json.Marshal(config)
		if err != nil || len(stdin) > 1<<20 {
			return sandbox.CommandResult{}, ErrBoundary
		}
		commandArgs = []string{"/opt/reforge/tool", "egress-tcp"}
	} else if state.egress {
		if err = r.endEgress(ctx, state); err != nil {
			return sandbox.CommandResult{}, err
		}
	}
	args := []string{"/bin/sh", "-c", "cd -- \"$1\" && shift && exec \"$@\"", "reforge-command", directory}
	args = append(args, commandArgs...)
	if err = r.verify(ctx, state); err != nil {
		return sandbox.CommandResult{}, err
	}
	callctx, cancel := context.WithTimeout(ctx, command.Timeout)
	start := time.Now()
	result, execErr := r.exec(callctx, state, args, stdin, command.MaxOutputBytes)
	result.Duration = time.Since(start)
	result.TimedOut = errors.Is(callctx.Err(), context.DeadlineExceeded)
	if result.TimedOut {
		execErr = nil
	}
	cancel()
	if execErr != nil || result.TimedOut || ctx.Err() != nil {
		if state.egress {
			execErr = errors.Join(execErr, r.endEgress(ctx, state))
		}
		deleteErr := r.deletePod(ctx, state.pod.Ref)
		leaseErr := runCleanup(ctx, func(clean context.Context) error { return r.closeLease(clean, state) })
		execErr = errors.Join(execErr, deleteErr, leaseErr)
		if deleteErr == nil && leaseErr == nil {
			r.removeState(state)
		}
	}
	if state.egress {
		if closeErr := r.endEgress(ctx, state); closeErr != nil {
			execErr = errors.Join(execErr, closeErr)
		}
	}
	return result, execErr
}

func (r *Runtime) CollectArtifact(ctx context.Context, workspace sandbox.Workspace, name string) (sandbox.Artifact, error) {
	if !guest.ValidPath(name) {
		return sandbox.Artifact{}, ErrBoundary
	}
	state, err := r.state(workspace)
	if err != nil {
		return sandbox.Artifact{}, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err = r.verify(ctx, state); err != nil {
		return sandbox.Artifact{}, err
	}
	request, _ := json.Marshal(guest.Request{Operation: "read", Path: name, Limit: 4 << 20})
	var stdout boundedOutput
	stdout.limit = 6 << 20
	code, err := r.config.Client.Exec(ctx, state.pod.Ref, []string{"/opt/reforge/tool"}, bytes.NewReader(request), &stdout, io.Discard)
	if err != nil || code != 0 || stdout.truncated {
		return sandbox.Artifact{}, errors.Join(err, ErrBoundary)
	}
	var response struct {
		Content []byte `json:"content"`
	}
	if json.Unmarshal(stdout.data, &response) != nil || len(response.Content) > 4<<20 {
		return sandbox.Artifact{}, ErrBoundary
	}
	return sandbox.Artifact{Name: name, MediaType: "text/plain", Data: response.Content}, nil
}

func (r *Runtime) Destroy(ctx context.Context, workspace sandbox.Workspace) error {
	state, err := r.state(workspace)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	deleteErr := r.deletePod(ctx, state.pod.Ref)
	leaseErr := runCleanup(ctx, func(clean context.Context) error { return r.closeLease(clean, state) })
	if deleteErr == nil && leaseErr == nil {
		r.removeState(state)
	}
	return errors.Join(deleteErr, leaseErr)
}

func (r *Runtime) apply(ctx context.Context, state *workspaceState, files []guest.File) error {
	if len(files) == 0 || len(files) > 20000 {
		return ErrBoundary
	}
	var total int
	for _, file := range files {
		total += len(file.Content)
		if !guest.ValidPath(file.Path) || len(file.Content) > 4<<20 || total > 64<<20 {
			return ErrBoundary
		}
	}
	request, err := json.Marshal(guest.Request{Operation: "apply", Files: files})
	if err != nil || len(request) > 96<<20 {
		return ErrBoundary
	}
	if err = r.verify(ctx, state); err != nil {
		return err
	}
	var stdout boundedOutput
	stdout.limit = 64 << 10
	code, err := r.config.Client.Exec(ctx, state.pod.Ref, []string{"/opt/reforge/tool"}, bytes.NewReader(request), &stdout, io.Discard)
	if err != nil || code != 0 || stdout.truncated {
		return errors.Join(err, ErrBoundary)
	}
	return nil
}

func (r *Runtime) exec(ctx context.Context, state *workspaceState, args []string, stdin []byte, maxOutput int64) (sandbox.CommandResult, error) {
	var output boundedOutput
	output.limit = maxOutput
	code, err := r.config.Client.Exec(ctx, state.pod.Ref, args, bytes.NewReader(stdin), &output, &output)
	return sandbox.CommandResult{ExitCode: code, Output: output.data, Truncated: output.truncated}, err
}

func runCleanup(ctx context.Context, fn func(context.Context) error) error {
	clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return fn(clean)
}

func safeDeleteRef(expected, actual PodRef) PodRef {
	if actual.Namespace == expected.Namespace && actual.Name == expected.Name {
		return PodRef{Namespace: expected.Namespace, Name: expected.Name, UID: actual.UID}
	}
	return expected
}

func (r *Runtime) deletePod(ctx context.Context, pod PodRef) error {
	return runCleanup(ctx, func(clean context.Context) error { return r.config.Client.Delete(clean, pod) })
}

func (r *Runtime) verify(ctx context.Context, state *workspaceState) error {
	pod, err := r.config.Client.Get(ctx, state.pod.Ref)
	if err != nil {
		return err
	}
	if err = validPodIdentity(state.pod, pod); err != nil || pod.Phase != "Running" {
		return ErrBoundary
	}
	return nil
}

func (r *Runtime) state(workspace sandbox.Workspace) (*workspaceState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.workspaces[workspace.ID]
	if state == nil || state.workspace != workspace {
		return nil, ErrBoundary
	}
	return state, nil
}

func (r *Runtime) removeState(state *workspaceState) {
	r.mu.Lock()
	delete(r.workspaces, state.workspace.ID)
	r.mu.Unlock()
}

func (r *Runtime) closeLease(ctx context.Context, state *workspaceState) error {
	if state.lease == nil {
		return nil
	}
	err := state.lease.Close(ctx)
	if err == nil {
		state.lease = nil
		state.egress = false
	}
	return err
}

func (r *Runtime) endEgress(ctx context.Context, state *workspaceState) error {
	if !state.egress || state.lease == nil {
		return nil
	}
	err := runCleanup(ctx, state.lease.EndEgress)
	if err == nil {
		state.egress = false
	}
	return err
}

func validPodSpec(spec PodSpec) error {
	if !namespaceValue.MatchString(spec.Ref.Namespace) || len(spec.Ref.Name) > 63 || !strings.HasPrefix(spec.Ref.Name, "rf-ws-") || !validImageRef(spec.Image) || !validPullSecrets(spec.ImagePullSecrets) || spec.RunAsUser != 65532 || !spec.RunAsNonRoot || spec.RunAsGroup != 65532 || spec.FSGroup != 65532 || spec.AllowPrivilegeEscalation || !spec.ReadOnlyRootFilesystem || !slicesEqual(spec.DropCapabilities, []string{"ALL"}) || spec.SeccompProfile != "RuntimeDefault" || spec.AutomountServiceAccountToken || spec.HostNetwork || spec.HostPID || spec.HostIPC || len(spec.HostPaths) != 0 || spec.WorkspaceEmptyDirBytes <= 0 || spec.TempEmptyDirBytes <= 0 || spec.ActiveDeadlineSeconds <= 0 || spec.RestartPolicy != "Never" || spec.NetworkProfile != "none" || spec.Resources.MemoryBytes <= 0 || spec.Resources.CPUs <= 0 || spec.Resources.DiskBytes <= 0 {
		return ErrBoundary
	}
	for _, value := range spec.Labels {
		if !labelValue.MatchString(value) {
			return ErrBoundary
		}
	}
	return nil
}

func validPodIdentity(want, got Pod) error {
	if got.Ref.Namespace != want.Ref.Namespace || got.Ref.Name != want.Ref.Name || got.Ref.UID == "" || got.Ref.UID != want.Ref.UID || !sameLabels(want.Labels, got.Labels) {
		return ErrBoundary
	}
	return nil
}

func sameLabels(want, got map[string]string) bool {
	for name, value := range want {
		if got[name] != value {
			return false
		}
	}
	return true
}

func validOpaque(value string) bool {
	return labelValue.MatchString(value)
}

func validImageRef(value string) bool {
	if !imageDigest.MatchString(value) {
		return false
	}
	return !strings.HasSuffix(value[:strings.Index(value, "@sha256:")], "/")
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func randomID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

type boundedOutput struct {
	data      []byte
	limit     int64
	truncated bool
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	count := len(data)
	left := int(b.limit) - len(b.data)
	if left <= 0 {
		b.truncated = b.truncated || count > 0
		return count, nil
	}
	if len(data) > left {
		data = data[:left]
		b.truncated = true
	}
	b.data = append(b.data, data...)
	return count, nil
}

func cleanDirectory(directory string) (string, error) {
	if directory == "" || directory == "/workspace" || directory == "." {
		return ".", nil
	}
	directory = strings.TrimPrefix(directory, "/workspace/")
	if strings.HasPrefix(directory, "/") || directory == "." || !guest.ValidPath(directory) {
		return "", ErrBoundary
	}
	return directory, nil
}

func validPullSecrets(names []string) bool {
	if len(names) > 16 {
		return false
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if len(name) > 253 || !secretNameValue.MatchString(name) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}
