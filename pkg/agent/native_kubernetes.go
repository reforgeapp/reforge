package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
	"time"

	"github.com/reforgeapp/reforge/pkg/sandbox/kubernetes"
)

type NativeKubernetesConfig struct {
	Client           kubernetes.PodClient
	Namespace        string
	Image            string
	RuntimeClassName string
	ImagePullSecrets []string
	StorageClass     string
	AccessMode       string
}

func NewNativeKubernetesOpen(cfg NativeKubernetesConfig) NativeOpen {
	if cfg.Client == nil || !pinnedImage(cfg.Image) || cfg.Namespace == "" || cfg.RuntimeClassName == "" {
		return nil
	}
	if cfg.AccessMode == "" {
		cfg.AccessMode = "ReadWriteOnce"
	}
	return func(ctx context.Context, scope NativeScope) (NativeHost, error) {
		if !scope.Valid() {
			return nil, ErrDenied
		}
		key := scope.Key()
		claim := kubernetes.Claim{Name: "rf-cache-" + key[:24], StorageClass: cfg.StorageClass, AccessMode: cfg.AccessMode, Bytes: 1 << 30}
		if err := cfg.Client.EnsureClaim(ctx, claim); err != nil {
			return nil, err
		}
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return nil, err
		}
		spec := kubernetes.PodSpec{
			Ref:    kubernetes.PodRef{Namespace: cfg.Namespace, Name: "rf-ws-agent-" + hex.EncodeToString(random)},
			Labels: map[string]string{"app.kubernetes.io/name": "reforge-agent", "reforge.io/runner-id": scope.RunnerID, "reforge.io/account-key": key[:32]},
			Image:  cfg.Image, RuntimeClassName: cfg.RuntimeClassName, ImagePullSecrets: cfg.ImagePullSecrets,
			ContainerName: "workspace", WorkingDirectory: "/workspace",
			Environment: []string{"HOME=/cache", "CODEX_HOME=/cache/.codex", "CLAUDE_CONFIG_DIR=/cache/.claude", "DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "CLAUDE_CODE_MAX_OUTPUT_TOKENS=8192", "PATH=/usr/local/bin:/usr/bin:/bin"},
			RunAsUser:   65532, RunAsGroup: 65532, FSGroup: 65532, RunAsNonRoot: true,
			ReadOnlyRootFilesystem: true, DropCapabilities: []string{"ALL"}, SeccompProfile: "RuntimeDefault",
			CacheClaim: claim.Name, WorkspaceEmptyDirBytes: 64 << 20, TempEmptyDirBytes: 256 << 20,
			NetworkProfile: "agent", Resources: kubernetes.Resources{MemoryBytes: 768 << 20, CPUs: 1, DiskBytes: 1 << 30},
			ActiveDeadlineSeconds: 1200, RestartPolicy: "Never",
		}
		pod, err := cfg.Client.Create(ctx, spec)
		if err != nil {
			return nil, err
		}
		host := &nativeKubernetesHost{client: cfg.Client, pod: pod.Ref, claim: claim.Name}
		ready, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		for {
			current, err := cfg.Client.Get(ready, pod.Ref)
			if err != nil || current.Ref.UID != pod.Ref.UID || current.Phase == "Failed" || current.Phase == "Succeeded" || current.Unschedulable {
				_ = host.Close()
				return nil, ErrDisabled
			}
			if current.Phase == "Running" {
				code, err := host.Exec(ready, []string{"/usr/bin/mkdir", "-p", "-m", "0700", "/cache/.codex", "/cache/.claude"}, nil, io.Discard, io.Discard)
				if err != nil || code != 0 {
					_ = host.Close()
					return nil, ErrDisabled
				}
				return host, nil
			}
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ready.Done():
				timer.Stop()
				_ = host.Close()
				return nil, ErrDisabled
			case <-timer.C:
			}
		}
	}
}

type nativeKubernetesHost struct {
	client kubernetes.PodClient
	pod    kubernetes.PodRef
	claim  string
	mu     sync.Mutex
	closed bool
}

func (h *nativeKubernetesHost) Exec(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	return h.client.Exec(ctx, h.pod, args, stdin, stdout, stderr)
}

func (h *nativeKubernetesHost) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	for range 2 {
		err = h.client.Delete(ctx, h.pod)
		if err == nil {
			h.closed = true
			return nil
		}
	}
	return err
}

func (h *nativeKubernetesHost) Forget(ctx context.Context) error {
	if err := h.Close(); err != nil {
		return err
	}
	client, ok := h.client.(interface {
		DeleteClaim(context.Context, string) error
	})
	if !ok {
		return ErrDisabled
	}
	return client.DeleteClaim(ctx, h.claim)
}
