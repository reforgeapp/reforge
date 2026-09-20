package privateconnector

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"reforge/internal/auth"
)

type ClientConfig struct {
	Endpoint    string
	Credential  string `json:"-"`
	Target      Target
	Development bool
	CAPEM       []byte
}

func (c ClientConfig) String() string       { return "private supervisor client configuration [redacted]" }
func (c ClientConfig) GoString() string     { return c.String() }
func (c ClientConfig) LogValue() slog.Value { return slog.StringValue(c.String()) }

type Client struct {
	base       string
	credential string
	target     Target
	client     *http.Client
	executor   Executor
}

func NewClient(cfg ClientConfig) (*Client, error) {
	base, e := safeControllerURL(cfg.Endpoint, cfg.Development)
	if e != nil || cfg.Credential == "" || len(cfg.Credential) > 256 || !auth.ValidID(cfg.Target.OrgID) || !auth.ValidID(cfg.Target.RunnerID) {
		return nil, ErrInvalid
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(cfg.CAPEM) > 0 {
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(cfg.CAPEM) {
			return nil, ErrInvalid
		}
		tlsCfg.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = tlsCfg
	transport.ResponseHeaderTimeout = MaxTTL + 5*time.Second
	transport.MaxResponseHeaderBytes = 65536
	return &Client{base: base, credential: cfg.Credential, target: cfg.Target, client: &http.Client{Transport: transport, Timeout: MaxTTL + 5*time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, executor: Executor{Target: cfg.Target, Development: cfg.Development}}, nil
}
func (c *Client) String() string       { return "private supervisor client [redacted]" }
func (c *Client) GoString() string     { return c.String() }
func (c *Client) LogValue() slog.Value { return slog.StringValue(c.String()) }
func (c *Client) Close()               { c.client.CloseIdleConnections(); c.credential = "" }
func (c *Client) request(ctx context.Context, path string, body []byte, capability string) ([]byte, int, error) {
	req, e := http.NewRequestWithContext(ctx, "POST", c.base+path, bytes.NewReader(body))
	if e != nil {
		return nil, 0, ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+c.credential)
	req.Header.Set("Content-Type", "application/json")
	if capability != "" {
		req.Header.Set("X-Private-Result-Capability", capability)
	}
	response, e := c.client.Do(req)
	if e != nil {
		return nil, 0, ErrUnavailable
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, MaxGrant+1))
	if e != nil || len(raw) > MaxGrant {
		return nil, response.StatusCode, ErrInvalid
	}
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, response.StatusCode, auth.ErrUnauthenticated
	}
	if response.StatusCode == 409 {
		return nil, response.StatusCode, ErrConflict
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode, ErrUnavailable
	}
	return raw, response.StatusCode, nil
}
func (c *Client) Poll(ctx context.Context) (Grant, error) {
	started := time.Now()
	raw, status, e := c.request(ctx, "/runner/v1/private/poll", []byte("{}"), "")
	if e != nil {
		return Grant{}, e
	}
	if status == 204 {
		return Grant{}, ErrUnavailable
	}
	grant, e := DecodeGrant(raw)
	if e != nil || grant.Target != c.target {
		return Grant{}, ErrInvalid
	}
	if grant.TimeoutMS < 1 || grant.TimeoutMS > grant.Operation.MaximumTTL().Milliseconds() {
		return Grant{}, ErrInvalid
	}
	grant.executionDeadline = started.Add(time.Duration(grant.TimeoutMS) * time.Millisecond)
	return grant, nil
}
func (c *Client) Complete(ctx context.Context, grant Grant, result Result) error {
	raw, e := json.Marshal(Completion{GrantID: grant.ID, Result: result})
	if e != nil || len(raw) > MaxResponse {
		return ErrInvalid
	}
	_, _, e = c.request(ctx, "/runner/v1/private/results", raw, grant.ResultCapability)
	if e != nil {
		return ErrUncertain
	}
	return nil
}
func (c *Client) RunOnce(ctx context.Context) error {
	grant, e := c.Poll(ctx)
	if e != nil {
		return e
	}
	execution, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	if grant.Operation.Kind == ModelTurn {
		go func() {
			defer close(done)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-execution.Done():
					return
				case <-ticker.C:
					check, cancel := context.WithTimeout(execution, 3*time.Second)
					body, _ := json.Marshal(map[string]string{"grant_id": grant.ID})
					_, _, err := c.request(check, "/runner/v1/private/status", body, grant.ResultCapability)
					cancel()
					if err != nil {
						stop()
						return
					}
				}
			}
		}()
	} else {
		close(done)
	}
	executor := c.executor
	if grant.Operation.Mutation() {
		executor.Authorize = func(ctx context.Context) error {
			body, _ := json.Marshal(map[string]string{"grant_id": grant.ID})
			_, _, err := c.request(ctx, "/runner/v1/private/status", body, grant.ResultCapability)
			return err
		}
	}
	result := executor.Execute(execution, grant)
	stop()
	<-done
	defer func() { grant.Connection.Secret = ""; grant.ResultCapability = "" }()
	return c.Complete(ctx, grant, result)
}

type Completion struct {
	GrantID string `json:"grant_id"`
	Result  Result `json:"result"`
}
