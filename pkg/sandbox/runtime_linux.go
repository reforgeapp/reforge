package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

var ErrUnavailable = errors.New("sandbox isolation or resource controls unavailable")
var ErrBoundary = errors.New("sandbox request violates execution boundary")

type Snapshot struct {
	CommitSHA      string       `json:"commit_sha"`
	Complete       bool         `json:"complete"`
	ManifestSHA256 string       `json:"manifest_sha256"`
	Files          []guest.File `json:"files"`
}

type KubernetesRuntimeConfig struct {
	Namespace              string            `json:"namespace"`
	RunnerID               string            `json:"-"`
	RuntimeClassName       string            `json:"runtime_class_name,omitempty"`
	ImagePullSecrets       []string          `json:"image_pull_secrets,omitempty"`
	Images                 map[string]string `json:"images"`
	Toolchains             map[string]string `json:"toolchains"`
	BrokerListenAddress    string            `json:"broker_listen_address,omitempty"`
	BrokerAdvertiseAddress string            `json:"broker_advertise_address,omitempty"`
	CacheBytes             int64             `json:"cache_bytes,omitempty"`
	CacheStorageClass      string            `json:"cache_storage_class,omitempty"`
	CacheAccessMode        string            `json:"cache_access_mode,omitempty"`
}

type RuntimeConfig struct {
	Backend        string                                                    `json:"backend,omitempty"`
	Kubernetes     *KubernetesRuntimeConfig                                  `json:"kubernetes,omitempty"`
	Runsc          string                                                    `json:"runsc"`
	RunscSHA256    string                                                    `json:"runsc_sha256"`
	Tool           string                                                    `json:"tool"`
	ToolSHA256     string                                                    `json:"tool_sha256"`
	StateRoot      string                                                    `json:"state_root"`
	DependencyRoot string                                                    `json:"dependency_root,omitempty"`
	CgroupRoot     string                                                    `json:"cgroup_root"`
	Images         map[string]string                                         `json:"images"`
	Development    bool                                                      `json:"development"`
	Rootless       bool                                                      `json:"rootless"`
	MemoryBytes    int64                                                     `json:"memory_bytes"`
	DiskBytes      int64                                                     `json:"disk_bytes"`
	CPUs           int64                                                     `json:"cpus"`
	MaxProcesses   int64                                                     `json:"max_processes"`
	Fetch          func(context.Context, WorkspaceRequest) (Snapshot, error) `json:"-"`
}

type Runtime struct {
	config     RuntimeConfig
	lockFile   *os.File
	closed     bool
	mu         sync.Mutex
	workspaces map[string]*workspaceState
}

type workspaceState struct {
	mu           chan struct{}
	workspace    Workspace
	bundle       string
	dependencies string
	egress       string
	cgroup       *os.File
	cgroupPath   string
	ctx          context.Context
	cancel       context.CancelFunc
	process      *exec.Cmd
	done         chan struct{}
	output       *boundedOutput
	stopOnce     sync.Once
	processMu    sync.Mutex
	stopped      bool
}

func NewRuntime(cfg RuntimeConfig) (*Runtime, error) {
	if !filepath.IsAbs(cfg.StateRoot) || cfg.StateRoot == "/" || len(cfg.Images) == 0 || cfg.Fetch == nil {
		return nil, ErrBoundary
	}
	if cfg.MemoryBytes < 256<<20 || cfg.MemoryBytes > 64<<30 || cfg.DiskBytes < 16<<20 || cfg.DiskBytes > cfg.MemoryBytes/2 || cfg.CPUs < 1 || cfg.CPUs > 32 || cfg.MaxProcesses < 32 || cfg.MaxProcesses > 4096 {
		return nil, ErrBoundary
	}
	for _, asset := range []struct{ path, digest string }{{cfg.Runsc, cfg.RunscSHA256}, {cfg.Tool, cfg.ToolSHA256}} {
		if !filepath.IsAbs(asset.path) || len(asset.digest) != 64 {
			return nil, ErrBoundary
		}
		info, err := os.Lstat(asset.path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 || info.Mode()&0022 != 0 {
			return nil, ErrUnavailable
		}
		file, err := os.Open(asset.path)
		if err != nil {
			return nil, ErrUnavailable
		}
		h := sha256.New()
		_, err = io.Copy(h, file)
		file.Close()
		if err != nil || hex.EncodeToString(h.Sum(nil)) != asset.digest {
			return nil, ErrUnavailable
		}
	}
	images := map[string]string{}
	for digest, root := range cfg.Images {
		if !filepath.IsAbs(root) || root == "/" || len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") {
			return nil, ErrBoundary
		}
		if err := imageLayout(root); err != nil {
			return nil, err
		}
		actual, err := ImageDigest(root)
		if err != nil || actual != digest {
			return nil, ErrUnavailable
		}
		images[digest] = root
	}
	cfg.Images = images
	if err := os.MkdirAll(cfg.StateRoot, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(cfg.StateRoot)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, ErrBoundary
	}
	if !cfg.Development && (!filepath.IsAbs(cfg.CgroupRoot) || filepath.Clean(cfg.CgroupRoot) != cfg.CgroupRoot || !strings.HasPrefix(cfg.CgroupRoot, "/sys/fs/cgroup/")) {
		return nil, ErrUnavailable
	}
	lock, e := os.OpenFile(filepath.Join(cfg.StateRoot, ".runtime.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, ErrUnavailable
	}
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		lock.Close()
		return nil, ErrUnavailable
	}
	runtime := &Runtime{config: cfg, lockFile: lock, workspaces: map[string]*workspaceState{}}
	if e = runtime.cleanOrphans(); e != nil {
		lock.Close()
		return nil, e
	}
	return runtime, nil
}

func ImageDigest(root string) (string, error) { return imageDigest(context.Background(), root) }
func imageDigest(ctx context.Context, root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return "", ErrBoundary
	}
	h := sha256.New()
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 && info.Mode()&0022 != 0 {
			return ErrBoundary
		}
		fmt.Fprintf(h, "%s\x00%d\x00", rel, info.Mode())
		switch {
		case info.Mode().IsRegular():
			fmt.Fprintf(h, "%d\x00", info.Size())
			f, err := os.Open(name)
			if err != nil {
				return err
			}
			_, err = io.Copy(h, contextReader{ctx: ctx, reader: f})
			f.Close()
			if err != nil {
				return err
			}
		case info.IsDir():
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(name)
			if err != nil {
				return err
			}
			io.WriteString(h, target)
		default:
			return ErrBoundary
		}
		io.WriteString(h, "\x00")
		return nil
	})
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), err
}

func validSHA(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (r *Runtime) PreparePinnedWorkspace(ctx context.Context, in WorkspaceRequest) (Workspace, error) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return Workspace{}, ErrUnavailable
	}
	if in.JobID == "" || in.AttemptID == "" || !validSHA(in.CommitSHA) || in.Timeout < time.Second || in.Timeout > 2*time.Hour {
		return Workspace{}, ErrBoundary
	}
	image, ok := r.config.Images[in.Image]
	if !ok || (r.config.Development && in.Trust != "development-fixture") {
		return Workspace{}, ErrUnavailable
	}
	if in.Dependencies != "" {
		info, err := os.Lstat(in.Dependencies)
		if filepath.Clean(in.Dependencies) != in.Dependencies || r.config.DependencyRoot == "" || !strings.HasPrefix(in.Dependencies, r.config.DependencyRoot+"/") || err != nil || !info.IsDir() {
			return Workspace{}, ErrBoundary
		}
	}
	if in.Egress != "" {
		info, err := os.Lstat(in.Egress)
		if filepath.Clean(in.Egress) != in.Egress || !strings.HasPrefix(in.Egress, filepath.Join(r.config.StateRoot, "egress")+"/") || err != nil || !info.IsDir() {
			return Workspace{}, ErrBoundary
		}
	}
	jobctx, cancel := context.WithTimeout(ctx, in.Timeout)
	keep := false
	defer func() {
		if !keep {
			cancel()
		}
	}()
	snapshot, err := r.config.Fetch(jobctx, in)
	if err != nil {
		return Workspace{}, err
	}
	if snapshot.CommitSHA != in.CommitSHA || !snapshot.Complete || len(snapshot.Files) == 0 || len(snapshot.Files) > 20000 {
		return Workspace{}, ErrBoundary
	}
	digest, err := SnapshotDigest(snapshot.Files)
	if err != nil || digest != snapshot.ManifestSHA256 {
		return Workspace{}, ErrBoundary
	}
	if err = r.verifyAssets(jobctx, in.Image); err != nil {
		return Workspace{}, err
	}
	id := "rf-" + domain.NewID()
	bundle, err := os.MkdirTemp(r.config.StateRoot, "bundle-")
	if err != nil {
		return Workspace{}, err
	}
	w := &workspaceState{mu: make(chan struct{}, 1), workspace: Workspace{ID: id, CommitSHA: in.CommitSHA, Root: "/workspace", Image: in.Image}, bundle: bundle, dependencies: in.Dependencies, egress: in.Egress, ctx: jobctx, cancel: cancel}
	if err = r.setupCgroup(w); err != nil {
		cancel()
		os.RemoveAll(bundle)
		return Workspace{}, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		if w.cgroup != nil {
			w.cgroup.Close()
			_ = os.Remove(w.cgroupPath)
		}
		_ = os.RemoveAll(bundle)
		return Workspace{}, ErrUnavailable
	}
	r.workspaces[id] = w
	r.mu.Unlock()
	succeeded := false
	defer func() {
		if !succeeded {
			_ = r.Destroy(context.Background(), w.workspace)
		}
	}()
	data, err := json.Marshal(r.spec(w, image))
	if err != nil {
		return Workspace{}, err
	}
	if err = os.WriteFile(filepath.Join(bundle, "config.json"), data, 0600); err != nil {
		return Workspace{}, err
	}
	if err = r.start(jobctx, w); err != nil {
		return Workspace{}, err
	}
	if _, err = r.fileOperation(jobctx, w, guest.Request{Operation: "apply", Files: snapshot.Files}); err != nil {
		return Workspace{}, err
	}
	go func() { <-jobctx.Done(); r.stop(w) }()
	succeeded = true
	keep = true
	return w.workspace, nil
}

func (r *Runtime) state(workspace Workspace) (*workspaceState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := r.workspaces[workspace.ID]
	if w == nil || w.workspace != workspace {
		return nil, ErrBoundary
	}
	return w, nil
}

func (r *Runtime) ExecuteBoundedCommand(ctx context.Context, workspace Workspace, command Command) (CommandResult, error) {
	w, err := r.state(workspace)
	if err != nil {
		return CommandResult{}, err
	}
	if len(command.Args) == 0 || len(command.Args) > 128 || command.Timeout < time.Millisecond || command.Timeout > time.Hour || command.MaxOutputBytes < 1 || command.MaxOutputBytes > 4<<20 || command.NetworkProfile != "none" && (command.NetworkProfile != "egress" || w.egress == "") || len(command.Stdin) > 1<<20 {
		return CommandResult{}, ErrBoundary
	}
	for _, arg := range command.Args {
		if len(arg) > 16384 || strings.ContainsRune(arg, 0) {
			return CommandResult{}, ErrBoundary
		}
	}
	directory := strings.TrimPrefix(command.Directory, "/workspace/")
	if command.Directory == "" || command.Directory == "/workspace" {
		directory = "."
	}
	if directory != "." && !guest.ValidPath(directory) {
		return CommandResult{}, ErrBoundary
	}
	if err = w.lock(ctx); err != nil {
		return CommandResult{}, err
	}
	defer w.unlock()
	if w.ctx.Err() != nil {
		return CommandResult{}, ErrUnavailable
	}
	callctx, cancel := context.WithTimeout(ctx, command.Timeout)
	defer cancel()
	stop := context.AfterFunc(w.ctx, cancel)
	defer stop()
	if err = r.verifyAssets(callctx, w.workspace.Image); err != nil {
		return CommandResult{}, err
	}
	args := []string{"exec", "--user=65532:65532", "--cwd=" + path.Join("/workspace", directory), workspace.ID}
	args = append(args, command.Args...)
	start := time.Now()
	result, err := r.invoke(callctx, w, command.Stdin, command.MaxOutputBytes, true, args...)
	result.Duration = time.Since(start)
	result.TimedOut = errors.Is(callctx.Err(), context.DeadlineExceeded)
	if callctx.Err() != nil {
		w.cancel()
		r.stop(w)
	}
	return result, err
}

func (r *Runtime) fileOperation(ctx context.Context, w *workspaceState, request guest.Request) ([]byte, error) {
	callctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(w.ctx, cancel)
	defer stop()
	if callctx.Err() != nil || w.ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	if err := r.verifyAssets(callctx, w.workspace.Image); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(raw) > 96<<20 {
		return nil, ErrBoundary
	}
	result, err := r.invoke(callctx, w, raw, 6<<20, true, "exec", "--user=65532:65532", w.workspace.ID, "/opt/reforge/tool")
	if callctx.Err() != nil {
		w.cancel()
		r.stop(w)
		return nil, callctx.Err()
	}
	if err != nil || result.ExitCode != 0 || result.Truncated {
		return nil, ErrBoundary
	}
	return result.Output, nil
}

func (r *Runtime) ApplyPatch(ctx context.Context, workspace Workspace, patches []Patch) error {
	w, err := r.state(workspace)
	if err != nil {
		return err
	}
	if err = w.lock(ctx); err != nil {
		return err
	}
	defer w.unlock()
	if w.ctx.Err() != nil {
		return ErrUnavailable
	}
	if len(patches) == 0 || len(patches) > 20000 {
		return ErrBoundary
	}
	seen := map[string]bool{}
	total := 0
	files := make([]guest.File, 0, len(patches))
	for _, patch := range patches {
		total += len(patch.Content)
		if !guest.ValidPath(patch.Path) || seen[patch.Path] || len(patch.Content) > 4<<20 || total > 64<<20 {
			return ErrBoundary
		}
		seen[patch.Path] = true
		files = append(files, guest.File{Path: patch.Path, Content: patch.Content, Delete: patch.Delete})
	}
	_, err = r.fileOperation(ctx, w, guest.Request{Operation: "apply", Files: files})
	return err
}

func (r *Runtime) CollectArtifact(ctx context.Context, workspace Workspace, name string) (Artifact, error) {
	w, err := r.state(workspace)
	if err != nil {
		return Artifact{}, err
	}
	if err = w.lock(ctx); err != nil {
		return Artifact{}, err
	}
	defer w.unlock()
	if w.ctx.Err() != nil {
		return Artifact{}, ErrUnavailable
	}
	data, err := r.fileOperation(ctx, w, guest.Request{Operation: "read", Path: name, Limit: 4 << 20})
	if err != nil {
		return Artifact{}, err
	}
	var response struct {
		Content []byte `json:"content"`
	}
	if json.Unmarshal(data, &response) != nil {
		return Artifact{}, ErrBoundary
	}
	h := sha256.Sum256(response.Content)
	return Artifact{Name: name, MediaType: "text/plain", Data: response.Content, SHA256: hex.EncodeToString(h[:])}, nil
}

func (r *Runtime) Destroy(ctx context.Context, workspace Workspace) error {
	w, err := r.state(workspace)
	if err != nil {
		return err
	}
	w.cancel()
	r.stop(w)
	cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = w.lock(cleanup); err != nil {
		return err
	}
	defer w.unlock()
	result, err := r.invoke(cleanup, w, nil, 65536, false, "delete", "--force", workspace.ID)
	if err != nil || result.ExitCode != 0 {
		return ErrUnavailable
	}
	if w.cgroup != nil {
		w.cgroup.Close()
		if err = os.Remove(w.cgroupPath); err != nil {
			return err
		}
	}
	if err = os.RemoveAll(w.bundle); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.workspaces, workspace.ID)
	r.mu.Unlock()
	return nil
}

func (r *Runtime) spec(w *workspaceState, image string) any {
	mounts := []any{
		map[string]any{"destination": "/proc", "type": "proc", "source": "proc", "options": []string{"nosuid", "nodev", "noexec"}},
		map[string]any{"destination": "/dev", "type": "tmpfs", "source": "tmpfs", "options": []string{"nosuid", "strictatime", "mode=755", "size=4m"}},
		map[string]any{"destination": "/tmp", "type": "tmpfs", "source": "tmpfs", "options": []string{"nosuid", "nodev", "mode=1777", fmt.Sprintf("size=%d", r.config.DiskBytes), "nr_inodes=100000"}},
		map[string]any{"destination": "/workspace", "type": "tmpfs", "source": "tmpfs", "options": []string{"nosuid", "nodev", "mode=700", "uid=65532", "gid=65532", fmt.Sprintf("size=%d", r.config.DiskBytes), "nr_inodes=100000"}},
		map[string]any{"destination": "/opt/reforge/tool", "type": "bind", "source": r.config.Tool, "options": []string{"ro", "rbind", "nosuid", "nodev"}},
	}
	env := []string{"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin", "HOME=/tmp", "TMPDIR=/tmp", "GOCACHE=/tmp/go-build", "GOPATH=/workspace/.reforge/gopath", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "CI=true", "GOFLAGS=-mod=readonly"}
	if w.egress != "" {
		mounts = append(mounts, map[string]any{"destination": "/run/reforge", "type": "bind", "source": w.egress, "options": []string{"rbind", "nosuid", "nodev", "noexec"}})
	}
	if w.dependencies != "" {
		mounts = append(mounts, map[string]any{"destination": "/opt/deps", "type": "bind", "source": w.dependencies, "options": []string{"ro", "rbind", "nosuid", "nodev", "noexec"}})
		env = append(env, "GOMODCACHE=/opt/deps/go", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	}
	empty := []string{}
	return map[string]any{"ociVersion": "1.0.2", "hostname": "reforge", "root": map[string]any{"path": image, "readonly": true}, "process": map[string]any{"terminal": false, "user": map[string]int{"uid": 65532, "gid": 65532}, "args": []string{"/opt/reforge/tool", "hold"}, "cwd": "/workspace", "env": env, "noNewPrivileges": true, "capabilities": map[string]any{"bounding": empty, "effective": empty, "inheritable": empty, "permitted": empty, "ambient": empty}, "rlimits": []any{map[string]any{"type": "RLIMIT_NOFILE", "hard": 1024, "soft": 1024}, map[string]any{"type": "RLIMIT_FSIZE", "hard": r.config.DiskBytes, "soft": r.config.DiskBytes}}}, "mounts": mounts, "linux": map[string]any{"namespaces": []any{map[string]string{"type": "pid"}, map[string]string{"type": "network"}, map[string]string{"type": "ipc"}, map[string]string{"type": "uts"}, map[string]string{"type": "mount"}}, "maskedPaths": []string{"/proc/acpi", "/proc/kcore", "/proc/keys", "/proc/timer_list", "/proc/scsi", "/sys/firmware"}, "readonlyPaths": []string{"/proc/sys", "/proc/sysrq-trigger", "/proc/irq", "/proc/bus"}}}
}

var _ SandboxRuntime = (*Runtime)(nil)

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}
func (w *workspaceState) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case w.mu <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (w *workspaceState) unlock() { <-w.mu }
func SnapshotDigest(files []guest.File) (string, error) {
	if len(files) == 0 || len(files) > 20000 {
		return "", ErrBoundary
	}
	copyFiles := append([]guest.File(nil), files...)
	sort.Slice(copyFiles, func(i, j int) bool { return copyFiles[i].Path < copyFiles[j].Path })
	h := sha256.New()
	total := 0
	for i, f := range copyFiles {
		total += len(f.Content)
		if f.Delete || !guest.ValidPath(f.Path) || len(f.Content) > 64<<20 || total > 64<<20 || i > 0 && copyFiles[i-1].Path == f.Path {
			return "", ErrBoundary
		}
		fmt.Fprintf(h, "%s\x00%t\x00%d\x00", f.Path, f.Executable, len(f.Content))
		h.Write(f.Content)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (r *Runtime) verifyAssets(ctx context.Context, image string) error {
	for _, asset := range []struct{ path, digest string }{{r.config.Runsc, r.config.RunscSHA256}, {r.config.Tool, r.config.ToolSHA256}} {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Lstat(asset.path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0022 != 0 {
			return ErrUnavailable
		}
		f, err := os.Open(asset.path)
		if err != nil {
			return ErrUnavailable
		}
		h := sha256.New()
		_, err = io.Copy(h, contextReader{ctx: ctx, reader: f})
		f.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != asset.digest {
			return ErrUnavailable
		}
	}
	root, ok := r.config.Images[image]
	if !ok {
		return ErrUnavailable
	}
	digest, err := imageDigest(ctx, root)
	if err != nil {
		return err
	}
	if digest != image {
		return ErrUnavailable
	}
	return nil
}

func imageLayout(root string) error {
	for _, name := range []string{"proc", "dev", "tmp", "home", "workspace", "opt", "opt/reforge", "opt/reforge/tool"} {
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil {
			return ErrBoundary
		}
		if name == "opt/reforge/tool" {
			if !info.Mode().IsRegular() {
				return ErrBoundary
			}
		} else if !info.IsDir() {
			return ErrBoundary
		}
	}
	return nil
}
func (r *Runtime) Close() error {
	r.mu.Lock()
	r.closed = true
	workspaces := make([]Workspace, 0, len(r.workspaces))
	for _, w := range r.workspaces {
		workspaces = append(workspaces, w.workspace)
	}
	r.mu.Unlock()
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].ID < workspaces[j].ID })
	var failures []error
	for _, w := range workspaces {
		if err := r.Destroy(context.Background(), w); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lockFile != nil {
		err := r.lockFile.Close()
		r.lockFile = nil
		return err
	}
	return nil
}
