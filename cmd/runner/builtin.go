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
	"syscall"
	"time"

	"reforge/internal/runnerclient"
	"reforge/internal/sandbox"
)

var recipes = []string{"go", "javascript", "python"}

type builtinFlags struct {
	dir, images, runsc, tool, state, cgroup, endpoint, publicURL string
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
	flags.StringVar(&f.endpoint, "endpoint", "http://127.0.0.1:8080", "loopback control-plane address")
	flags.StringVar(&f.publicURL, "public-url", os.Getenv("REFORGE_PUBLIC_URL"), "control-plane public URL")
	if err := flags.Parse(args); err != nil {
		return f, err
	}
	if flags.NArg() != 0 {
		return f, errors.New("unexpected built-in runner argument")
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
	digests, err := imageDigests(f.images)
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

func runBuiltin(args []string) error {
	f, err := parseBuiltin("builtin", args)
	if err != nil {
		return err
	}
	public, err := url.Parse(f.publicURL)
	if err != nil || public.Host == "" {
		return errors.New("built-in runner requires REFORGE_PUBLIC_URL")
	}
	secret, err := os.ReadFile(filepath.Join(f.dir, "token"))
	if err != nil {
		return errors.New("built-in runner token unavailable; run builtin-init")
	}
	token := strings.TrimSpace(string(secret))
	images, err := imageDigests(f.images)
	if err != nil {
		return err
	}
	config := sandbox.RuntimeConfig{Runsc: f.runsc, Tool: f.tool, StateRoot: filepath.Join(f.state, "sandbox"), DependencyRoot: "/var/cache/reforge-deps", CgroupRoot: f.cgroup, Images: map[string]string{}, Rootless: true, MemoryBytes: 2 << 30, DiskBytes: 512 << 20, CPUs: 2, MaxProcesses: 512}
	for recipe, digest := range images {
		config.Images[digest] = filepath.Join(f.images, recipe)
	}
	if config.RunscSHA256, err = fileSHA256(f.runsc); err != nil {
		return err
	}
	if config.ToolSHA256, err = fileSHA256(f.tool); err != nil {
		return err
	}
	if err = os.MkdirAll(f.state, 0700); err != nil {
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
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	httpClient := &http.Client{Transport: hostTransport{host: public.Host, base: transport}}
	newClient := func(name string) (*runnerclient.Client, error) {
		return runnerclient.New(runnerclient.Config{Endpoint: f.endpoint, Development: true, Name: "built-in", CredentialFile: filepath.Join(f.state, name+".json"), Client: httpClient})
	}
	control, err := newClient("control")
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	process := runnerclient.RepairProcessor(config)
	orgs := map[string]*builtinOrg{}
	var refreshed time.Time
	slog.Info("built-in runner started", "recipes", images)
	for ctx.Err() == nil {
		if time.Since(refreshed) > time.Minute {
			if ids, err := control.BuiltinOrgs(ctx, token); err == nil {
				refreshed = time.Now()
				current := map[string]bool{}
				for _, id := range ids {
					current[id] = true
					if orgs[id] == nil {
						client, err := newClient(id)
						if err != nil {
							return err
						}
						_ = client.Load()
						orgs[id] = &builtinOrg{client: client}
					}
				}
				for id := range orgs {
					if !current[id] {
						delete(orgs, id)
					}
				}
			} else if ctx.Err() == nil {
				slog.Warn("built-in runner cannot reach the control plane", "error", err)
			}
		}
		worked := false
		for id, org := range orgs {
			if time.Now().Before(org.retry) {
				continue
			}
			if _, credential := org.client.Supervisor(); credential == "" {
				if err := org.client.EnrollBuiltin(ctx, token, id); err != nil {
					org.retry = time.Now().Add(time.Minute)
					continue
				}
			}
			done, err := org.client.Step(ctx, process)
			if err != nil && ctx.Err() == nil {
				slog.Warn("built-in runner step failed", "org_id", id, "error", err)
				_ = os.Remove(filepath.Join(f.state, id+".json"))
				client, _ := newClient(id)
				orgs[id] = &builtinOrg{client: client, retry: time.Now().Add(10 * time.Second)}
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
	return nil
}
