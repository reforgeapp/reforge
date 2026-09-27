package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"reforge/internal/runnerclient"
	"reforge/internal/sandbox"
)

var recipes = []string{"go", "javascript", "python"}

type builtinFlags struct {
	dir, images, runsc, tool, state, cgroup, endpoint, publicURL string
	runtimeConfig                                                string
	slots                                                        int
}

func parseBuiltin(name string, args []string) (builtinFlags, error) {
	var f builtinFlags
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.StringVar(&f.dir, "dir", "/builtin", "directory shared with the control plane")
	flags.StringVar(&f.images, "images", "/app/images", "recipe image directory")
	flags.StringVar(&f.runsc, "runsc", "/app/gvisor/runsc", "gVisor runsc binary")
	flags.StringVar(&f.tool, "tool", "/app/reforge-sandbox-tool", "sandbox tool binary")
	flags.StringVar(&f.state, "state", "/var/lib/reforge-runner", "private runner state directory")
	flags.StringVar(&f.cgroup, "cgroup-root", "/sys/fs/cgroup/reforge", "delegated cgroup v2 root")
	flags.StringVar(&f.endpoint, "endpoint", "http://127.0.0.1:8080", "control-plane address on the host or its private container network")
	flags.StringVar(&f.publicURL, "public-url", os.Getenv("REFORGE_PUBLIC_URL"), "control-plane public URL")
	flags.StringVar(&f.runtimeConfig, "runtime-config", "", "strict Kubernetes runtime configuration JSON")
	flags.IntVar(&f.slots, "slots", 2, "jobs run at once")
	if err := flags.Parse(args); err != nil {
		return f, err
	}
	if flags.NArg() != 0 {
		return f, errors.New("unexpected built-in runner argument")
	}
	if f.slots < 1 || f.slots > 16 {
		return f, errors.New("slots must be between 1 and 16")
	}
	return f, nil
}

func imageDigests(root string) (map[string]string, error) {
	out := map[string]string{}
	for _, recipe := range recipes {
		digest, err := sandbox.ImageDigest(filepath.Join(root, recipe))
		if err != nil {
			return nil, err
		}
		out[recipe] = digest
	}
	return out, nil
}

func builtinRuntimeConfig(f builtinFlags) (sandbox.RuntimeConfig, map[string]string, error) {
	if f.runtimeConfig != "" {
		config, err := loadRuntimeConfig(f.runtimeConfig)
		if err != nil {
			return sandbox.RuntimeConfig{}, nil, err
		}
		if config.Backend != "kubernetes" || config.Development {
			return sandbox.RuntimeConfig{}, nil, errors.New("built-in runtime config requires the Kubernetes backend without development mode")
		}
		images := make(map[string]string, len(config.Kubernetes.Toolchains))
		for recipe, digest := range config.Kubernetes.Toolchains {
			images[recipe] = digest
		}
		return config, images, nil
	}
	images, err := imageDigests(f.images)
	if err != nil {
		return sandbox.RuntimeConfig{}, nil, err
	}
	config := sandbox.RuntimeConfig{Runsc: f.runsc, Tool: f.tool, StateRoot: filepath.Join(f.state, "sandbox"), DependencyRoot: "/var/cache/reforge-deps", CgroupRoot: f.cgroup, Images: map[string]string{}, Rootless: true, MemoryBytes: 6 << 30, DiskBytes: 3 << 30, CPUs: 2, MaxProcesses: 512}
	for recipe, digest := range images {
		config.Images[digest] = filepath.Join(f.images, recipe)
	}
	return config, images, nil
}

func writeShared(path string, data []byte) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0640); err != nil {
		return err
	}
	if err := os.Chmod(temp, 0640); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func builtinInit(args []string) error {
	f, err := parseBuiltin("builtin-init", args)
	if err != nil {
		return err
	}
	_, digests, err := builtinRuntimeConfig(f)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(digests)
	if err = writeShared(filepath.Join(f.dir, "repair-images.json"), raw); err != nil {
		return err
	}
	if _, err = os.Lstat(filepath.Join(f.dir, "token")); err == nil {
		return nil
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return err
	}
	return writeShared(filepath.Join(f.dir, "token"), []byte(base64.RawURLEncoding.EncodeToString(secret)))
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err = io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func delegateCgroup(root string) error {
	if _, err := os.Stat(filepath.Join(root, "cgroup.subtree_control")); err == nil {
		return nil
	}
	parent := filepath.Dir(root)
	leaf := filepath.Join(parent, "supervisor")
	if err := os.MkdirAll(leaf, 0700); err != nil {
		return err
	}
	procs, err := os.ReadFile(filepath.Join(parent, "cgroup.procs"))
	if err != nil {
		return err
	}
	for _, pid := range strings.Fields(string(procs)) {
		_ = os.WriteFile(filepath.Join(leaf, "cgroup.procs"), []byte(pid), 0600)
	}
	controllers := []byte("+memory +pids +cpu")
	if err = os.WriteFile(filepath.Join(parent, "cgroup.subtree_control"), controllers, 0600); err != nil {
		return err
	}
	if err = os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.WriteFile(filepath.Join(root, "cgroup.subtree_control"), controllers, 0600)
}

type hostTransport struct {
	host string
	base http.RoundTripper
}

func (t hostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Host = t.host
	return t.base.RoundTrip(req)
}

type builtinOrg struct {
	client *runnerclient.Client
	retry  time.Time
}

func runBuiltin(args []string) (retErr error) {
	f, err := parseBuiltin("builtin", args)
	if err != nil {
		return err
	}
	public, err := url.Parse(f.publicURL)
	if err != nil || public.Host == "" {
		return errors.New("built-in runner requires REFORGE_PUBLIC_URL")
	}
	config, images, err := builtinRuntimeConfig(f)
	if err != nil {
		return err
	}
	secret, err := os.ReadFile(filepath.Join(f.dir, "token"))
	if err != nil {
		return errors.New("built-in runner token unavailable; run builtin-init")
	}
	token := strings.TrimSpace(string(secret))
	if err = os.MkdirAll(f.state, 0700); err != nil {
		return err
	}
	state, err := os.Lstat(f.state)
	if err != nil || !state.IsDir() || state.Mode().Perm()&0077 != 0 {
		return errors.New("built-in runner state requires a private regular directory")
	}
	if config.Backend != "kubernetes" {
		if config.RunscSHA256, err = fileSHA256(f.runsc); err != nil {
			return err
		}
		if config.ToolSHA256, err = fileSHA256(f.tool); err != nil {
			return err
		}
		if err = delegateCgroup(f.cgroup); err != nil {
			return errors.New("built-in runner needs a privileged container with cgroup v2 delegation")
		}
		check := config
		check.Fetch = func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error) {
			return sandbox.Snapshot{}, sandbox.ErrBoundary
		}
		runtime, err := sandbox.NewRuntime(check)
		if err != nil {
			return err
		}
		if err = runtime.Close(); err != nil {
			return err
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: hostTransport{host: public.Host, base: transport}}
	newClient := func(name string) (*runnerclient.Client, error) {
		return runnerclient.New(runnerclient.Config{Endpoint: f.endpoint, Development: true, Name: "built-in", Slots: f.slots, Internal: true, CredentialFile: filepath.Join(f.state, name+".json"), Client: httpClient})
	}
	control, err := newClient("control")
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	process, closeProcessor := runnerclient.RepairProcessorWithCloser(config)
	var mu sync.Mutex
	orgs := map[string]*builtinOrg{}
	fresh := func(id string) (*builtinOrg, error) {
		client, err := newClient(id)
		if err != nil {
			return nil, err
		}
		_ = client.Load()
		return &builtinOrg{client: client}, nil
	}
	ready := func(id string, org *builtinOrg) bool {
		mu.Lock()
		defer mu.Unlock()
		if time.Now().Before(org.retry) {
			return false
		}
		if _, credential := org.client.Supervisor(); credential == "" {
			if err := org.client.EnrollBuiltin(ctx, token, id); err != nil {
				org.retry = time.Now().Add(time.Minute)
				return false
			}
		}
		return true
	}
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
		retErr = errors.Join(retErr, closeProcessor())
	}()
	for n := 0; n < f.slots; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				mu.Lock()
				current := map[string]*builtinOrg{}
				for id, org := range orgs {
					current[id] = org
				}
				mu.Unlock()
				worked := false
				for id, org := range current {
					if !ready(id, org) {
						continue
					}
					done, err := org.client.Step(ctx, process)
					if err != nil && ctx.Err() == nil {
						slog.Warn("built-in runner step failed", "org_id", id, "error", err)
						mu.Lock()
						org.retry = time.Now().Add(10 * time.Second)
						if !done && errors.Is(err, runnerclient.ErrUnauthorized) && orgs[id] == org {
							_ = os.Remove(filepath.Join(f.state, id+".json"))
							if next, err := fresh(id); err == nil {
								next.retry = org.retry
								orgs[id] = next
							}
						}
						mu.Unlock()
					}
					worked = worked || done
				}
				if !worked {
					select {
					case <-ctx.Done():
					case <-time.After(2 * time.Second):
					}
				}
			}
		}()
	}
	slog.Info("built-in runner started", "recipes", images, "slots", f.slots)
	for ctx.Err() == nil {
		if ids, err := control.BuiltinOrgs(ctx, token); err == nil {
			current := map[string]bool{}
			for _, id := range ids {
				current[id] = true
				mu.Lock()
				known := orgs[id] != nil
				mu.Unlock()
				if known {
					continue
				}
				org, err := fresh(id)
				if err != nil {
					return err
				}
				mu.Lock()
				orgs[id] = org
				mu.Unlock()
			}
			mu.Lock()
			for id := range orgs {
				if !current[id] {
					delete(orgs, id)
				}
			}
			mu.Unlock()
		} else if ctx.Err() == nil {
			slog.Warn("built-in runner cannot reach the control plane", "error", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Minute):
		}
	}
	return nil
}
