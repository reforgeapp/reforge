package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"reforge/internal/artifact"
	"reforge/internal/domain"
	"reforge/internal/model"
	"reforge/internal/runner"
	"reforge/internal/workflow"
)

var ErrControlPlane = errors.New("runner control plane unavailable or rejected request")
var ErrUnauthorized = errors.New("runner credential rejected")

type Config struct {
	Endpoint       string       `json:"endpoint"`
	Development    bool         `json:"development"`
	Name           string       `json:"name"`
	Slots          int          `json:"slots"`
	Internal       bool         `json:"internal"`
	CredentialFile string       `json:"credential_file"`
	Client         *http.Client `json:"-"`
}

type Client struct {
	config     Config
	base       *url.URL
	http       *http.Client
	mu         sync.RWMutex
	credential credential
}

func (*Client) String() string   { return "[runner control client]" }
func (*Client) GoString() string { return "[runner control client]" }

type credential struct {
	Token     string        `json:"token"`
	ExpiresAt time.Time     `json:"expires_at"`
	Runner    runner.Runner `json:"runner"`
}

func (credential) String() string   { return "[runner credential]" }
func (credential) GoString() string { return "[runner credential]" }

type Job struct {
	Token     string         `json:"token"`
	ExpiresAt time.Time      `json:"expires_at"`
	Lease     workflow.Lease `json:"lease"`
	Task      workflow.Task  `json:"task"`
}

func (Job) String() string   { return "[runner job]" }
func (Job) GoString() string { return "[runner job]" }

type Processor func(context.Context, *Client, Job) (workflow.Completion, error)

func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "https" && !(cfg.Development && u.Scheme == "http" && (cfg.Internal || net.ParseIP(u.Hostname()).IsLoopback()))) || !filepath.IsAbs(cfg.CredentialFile) || len(cfg.Name) == 0 || len(cfg.Name) > 160 {
		return nil, errors.New("invalid runner configuration")
	}
	client := cfg.Client
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		client = &http.Client{Transport: transport}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	copyClient.Timeout = 35 * time.Second
	return &Client{config: cfg, base: u, http: &copyClient}, nil
}

func (c *Client) Enroll(ctx context.Context, token string) error {
	var next credential
	if _, err := c.call(ctx, "POST", "/runner/v1/enroll", token, map[string]string{"name": c.config.Name}, &next); err != nil {
		return err
	}
	return c.save(next)
}

func (c *Client) Load() error {
	info, err := os.Lstat(c.config.CredentialFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("runner credential file must be a private regular file")
	}
	file, err := os.OpenFile(c.config.CredentialFile, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return errors.New("runner credential file unavailable")
	}
	defer file.Close()
	var current credential
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&current) != nil || decoder.Decode(new(any)) != io.EOF || !validCredential(current) {
		return errors.New("runner credential invalid or expired; enroll again")
	}
	c.mu.Lock()
	c.credential = current
	c.mu.Unlock()
	return nil
}

func validCredential(v credential) bool {
	return strings.HasPrefix(v.Token, "sup/") && len(v.Token) < 256 && v.Runner.ID != "" && v.ExpiresAt.After(time.Now())
}

func (c *Client) save(next credential) error {
	if !validCredential(next) {
		return ErrControlPlane
	}
	directory := filepath.Dir(c.config.CredentialFile)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("runner state directory must be private")
	}
	file, err := os.CreateTemp(directory, ".credential-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = json.NewEncoder(file).Encode(next); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(file.Name(), c.config.CredentialFile); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.credential = next
	c.mu.Unlock()
	return nil
}

func (c *Client) Supervisor() (runner.Runner, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.credential.Runner, c.credential.Token
}

func (c *Client) Rotate(ctx context.Context) error {
	_, token := c.Supervisor()
	var next credential
	if _, err := c.call(ctx, "POST", "/runner/v1/rotate", token, nil, &next); err != nil {
		return err
	}
	return c.save(next)
}

func (c *Client) Claim(ctx context.Context) (Job, error) {
	_, token := c.Supervisor()
	var result Job
	status, err := c.call(ctx, "POST", "/runner/v1/claim", token, nil, &result)
	if status == 204 {
		return result, workflow.ErrNoWork
	}
	if err == nil && (result.Token == "" || result.Lease.OrgID == "" || result.Lease.TaskID != result.Task.ID || result.Lease.RepositoryID != result.Task.RepositoryID || !result.Lease.ExpiresAt.After(time.Now())) {
		err = ErrControlPlane
	}
	return result, err
}

func (c *Client) Heartbeat(ctx context.Context, j Job) (runner.Heartbeat, error) {
	var result runner.Heartbeat
	_, err := c.call(ctx, "POST", "/runner/v1/heartbeat", j.Token, nil, &result)
	return result, err
}

func (c *Client) Progress(ctx context.Context, j Job, state domain.TaskState) error {
	_, err := c.call(ctx, "POST", "/runner/v1/progress", j.Token, map[string]any{"state": state}, new(workflow.Task))
	return err
}

func (c *Client) Complete(ctx context.Context, j Job, result workflow.Completion) error {
	_, err := c.call(ctx, "POST", "/runner/v1/result", j.Token, result, new(workflow.Task))
	return err
}

func (c *Client) Upload(ctx context.Context, j Job, name, media string, data []byte) (artifact.Metadata, error) {
	var result artifact.Metadata
	if int64(len(data)) > artifact.MaxSize || strings.ContainsAny(name, "\r\n") {
		return result, errors.New("artifact exceeds runner limits")
	}
	u := *c.base
	u.Path = "/runner/v1/artifacts"
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(data))
	if err != nil {
		return result, ErrControlPlane
	}
	req.Header.Set("Authorization", "Bearer "+j.Token)
	req.Header.Set("Content-Type", media)
	req.Header.Set("X-Artifact-Name", name)
	_, err = c.response(req, &result)
	return result, err
}

func (c *Client) Broker(ctx context.Context, j Job, name, connection string, input any, result any) error {
	if strings.ContainsAny(name, "/?#%\\") || name == "" {
		return ErrControlPlane
	}
	_, err := c.call(ctx, "POST", "/runner/v1/operations/"+name, j.Token, map[string]any{"connection_id": connection, "input": input}, result)
	return err
}

func (c *Client) call(ctx context.Context, method, path, token string, input, output any) (int, error) {
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil || len(body) > 1<<20 {
			return 0, ErrControlPlane
		}
	}
	u := *c.base
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return 0, ErrControlPlane
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return c.response(req, output)
}

func (c *Client) response(req *http.Request, output any) (int, error) {
	return c.responseLimit(req, output, 2<<20)
}
func (c *Client) responseLimit(req *http.Request, output any, limit int64) (int, error) {
	response, err := c.http.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			return 0, req.Context().Err()
		}
		return 0, ErrControlPlane
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Message string `json:"message"`
		}
		base := ErrControlPlane
		if response.StatusCode == http.StatusUnauthorized {
			base = fmt.Errorf("%w: %w", ErrControlPlane, ErrUnauthorized)
		}
		if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure) == nil && failure.Message != "" && len(failure.Message) <= 300 {
			return response.StatusCode, fmt.Errorf("%w: %s", base, failure.Message)
		}
		return response.StatusCode, base
	}
	if response.StatusCode == 204 {
		return 204, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit || output == nil || json.Unmarshal(body, output) != nil {
		return response.StatusCode, ErrControlPlane
	}
	return response.StatusCode, nil
}

func (c *Client) Run(ctx context.Context, process Processor) error {
	if process == nil {
		return errors.New("runner processor is not configured")
	}
	for ctx.Err() == nil {
		worked, err := c.Step(ctx, process)
		if err != nil {
			return err
		}
		if !worked {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
	return ctx.Err()
}

func (c *Client) Step(ctx context.Context, process Processor) (bool, error) {
	c.mu.RLock()
	rotate := time.Until(c.credential.ExpiresAt) < time.Hour
	c.mu.RUnlock()
	if rotate {
		if err := c.Rotate(ctx); err != nil {
			return false, err
		}
	}
	job, err := c.Claim(ctx)
	if errors.Is(err, workflow.ErrNoWork) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, c.runJob(ctx, job, process)
}

func (c *Client) BuiltinOrgs(ctx context.Context, secret string) ([]string, error) {
	var out struct {
		OrgIDs []string `json:"org_ids"`
	}
	_, err := c.call(ctx, "GET", "/runner/v1/builtin/orgs", secret, nil, &out)
	return out.OrgIDs, err
}

func (c *Client) EnrollBuiltin(ctx context.Context, secret, org string) error {
	var next credential
	if _, err := c.call(ctx, "POST", "/runner/v1/builtin/enroll", secret, map[string]any{"org_id": org, "name": c.config.Name, "slots": max(c.config.Slots, 1)}, &next); err != nil {
		return err
	}
	return c.save(next)
}

func (c *Client) runJob(ctx context.Context, job Job, process Processor) error {
	jobctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopped := make(chan struct{})
	cancelRequested := false
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-jobctx.Done():
				return
			case <-ticker.C:
				heartbeatCtx, stop := context.WithTimeout(jobctx, 10*time.Second)
				value, err := c.Heartbeat(heartbeatCtx, job)
				stop()
				if err != nil || value.Stop {
					cancelRequested = value.Stop
					cancel()
					return
				}
			}
		}
	}()
	result, err := process(jobctx, c, job)
	if err != nil {
		slog.WarnContext(ctx, "runner job failed", "task_id", job.Lease.TaskID, "attempt_id", job.Lease.AttemptID, "outcome", result.Outcome, "error", err)
	}
	if err != nil && result.Outcome == "" {
		result = workflow.Completion{Outcome: "failed"}
	}
	cancel()
	<-stopped
	if cancelRequested && result.Outcome != "uncertain" {
		result.Outcome = "cancelled"
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	finish, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	return c.Complete(finish, job, result)
}

func (c *Client) ModelTurn(ctx context.Context, j Job, in model.Turn) (model.TurnResult, error) {
	var out model.TurnResult
	if !in.Valid() {
		return out, errors.New("invalid model turn")
	}
	copyClient := *c.http
	copyClient.Timeout = time.Duration(in.TimeoutMS)*time.Millisecond + 15*time.Second
	scoped := Client{config: c.config, base: c.base, http: &copyClient}
	_, err := scoped.call(ctx, "POST", "/runner/v1/model-turns", j.Token, in, &out)
	return out, err
}
