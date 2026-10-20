package runnerclient

import (
	"context"
	"encoding/json"
	"github.com/reforgeapp/reforge/pkg/customcmd"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/source"
	"net/http"
	"time"
)

func (c *Client) RepairContext(ctx context.Context, j Job) (repair.ExecutionContext, error) {
	var out repair.ExecutionContext
	_, err := c.call(ctx, "GET", "/runner/v1/repair/context", j.Token, nil, &out)
	return out, err
}
func (c *Client) RepairSnapshot(ctx context.Context, j Job, sha string) (sandbox.Snapshot, error) {
	var out sandbox.Snapshot
	if !source.ValidSHA(sha, "sha1") {
		return out, ErrControlPlane
	}
	u := *c.base
	u.Path = "/runner/v1/repair/source/" + sha
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+j.Token)
	transport := *c.http
	transport.Timeout = 3 * time.Minute
	scoped := Client{config: c.config, base: c.base, http: &transport}
	_, err = scoped.responseLimit(req, &out, 90<<20)
	return out, err
}
func (c *Client) RepairLogs(ctx context.Context, j Job, entries []repair.LogEntry) error {
	_, err := c.call(ctx, "POST", "/runner/v1/repair/logs", j.Token, entries, nil)
	return err
}

func (c *Client) RepairCheckpoint(ctx context.Context, j Job, in repair.Checkpoint) error {
	body, err := json.Marshal(in)
	if err != nil || len(body) > 1<<20 {
		return ErrControlPlane
	}
	for attempt := 0; attempt < 3; attempt++ {
		status, callErr := c.call(ctx, "POST", "/runner/v1/repair/checkpoint", j.Token, in, nil)
		if callErr == nil || ctx.Err() != nil {
			return callErr
		}
		retry := status == 0 || status == http.StatusInternalServerError || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
		if !retry || attempt == 2 {
			return callErr
		}
		timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func (c *Client) RepairReport(ctx context.Context, j Job, in repair.Report) (repair.Run, error) {
	var out repair.Run
	transport := *c.http
	transport.Timeout = 3 * time.Minute
	scoped := Client{config: c.config, base: c.base, http: &transport}
	_, err := scoped.call(ctx, "POST", "/runner/v1/repair/report", j.Token, in, &out)
	return out, err
}

func (c *Client) RepairStage(ctx context.Context, j Job) (repair.Run, error) {
	var out repair.Run
	_, err := c.call(ctx, "POST", "/runner/v1/repair/stage", j.Token, map[string]any{}, &out)
	return out, err
}
func (c *Client) RepairPublish(ctx context.Context, j Job, in repair.Publication) (repair.Run, error) {
	var out repair.Run
	_, err := c.call(ctx, "POST", "/runner/v1/repair/publish", j.Token, in, &out)
	return out, err
}
func (c *Client) RepairNativeChecks(ctx context.Context, j Job, in repair.Publication) error {
	_, err := c.call(ctx, "POST", "/runner/v1/repair/native-checks", j.Token, in, new(repair.Run))
	return err
}
func (c *Client) RepairRun(ctx context.Context, j Job) (repair.Run, error) {
	var out repair.Run
	_, err := c.call(ctx, "GET", "/runner/v1/repair/run", j.Token, nil, &out)
	return out, err
}
func (c *Client) CustomAuthorize(ctx context.Context, j Job) (customcmd.Authorized, error) {
	var out customcmd.Authorized
	_, err := c.call(ctx, "POST", "/runner/v1/repair/custom/authorize", j.Token, map[string]any{}, &out)
	return out, err
}
func (c *Client) CustomReport(ctx context.Context, j Job, in customcmd.ReportInput) (customcmd.Authorized, error) {
	var out customcmd.Authorized
	_, err := c.call(ctx, "POST", "/runner/v1/repair/custom/report", j.Token, in, &out)
	return out, err
}
