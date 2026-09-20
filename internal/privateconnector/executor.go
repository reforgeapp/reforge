package privateconnector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"time"

	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/forge/gitea"
	"reforge/internal/network"
)

type Executor struct {
	Target      Target
	Development bool
}

func validateConnection(c Connection, target Target, development bool) error {
	if !auth.ValidID(c.ID) || c.OrgID != target.OrgID || c.Version < 1 || c.CredentialVersion < 1 || c.Provider != "gitea" || c.Secret == "" || len(c.Secret) > 32768 || len(c.CAPEM) > 128<<10 {
		return ErrInvalid
	}
	if c.Route.OrgID != target.OrgID || c.Route.RunnerID != target.RunnerID || c.Route.ConnectionID != c.ID {
		return ErrInvalid
	}
	u, e := url.Parse(c.Endpoint)
	if e != nil || len(c.Endpoint) > 2048 {
		return ErrInvalid
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if !development || u.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return ErrInvalid
		}
	}
	if _, e = network.ValidateEndpoint(c.Endpoint, network.Options{RunnerID: target.RunnerID, PrivateRoute: &c.Route, CAPEM: c.CAPEM, Development: development}); e != nil {
		return ErrInvalid
	}
	return nil
}
func (e Executor) Execute(ctx context.Context, grant Grant) Result {
	result := Result{OperationID: grant.Operation.ID}
	fail := func(code string, uncertain bool) Result {
		return Result{OperationID: grant.Operation.ID, Failure: &Failure{Code: code, Uncertain: uncertain}}
	}
	if grant.RunnerVersion < 1 || grant.Target != e.Target || !auth.ValidID(grant.ID) || !auth.ValidID(grant.AuthorityID) || len(grant.ResultCapability) != 43 || grant.Operation.validate() != nil || validateConnection(grant.Connection, e.Target, e.Development) != nil {
		return fail("invalid", false)
	}
	remaining := time.Until(grant.executionDeadline)
	if remaining <= 0 || remaining > MaxTTL || grant.TimeoutMS < 1 || grant.TimeoutMS > MaxTTL.Milliseconds() || !grant.ExpiresAt.After(time.Now()) {
		return fail("expired", false)
	}
	ctx, cancel := context.WithDeadline(ctx, grant.executionDeadline)
	defer cancel()
	client, err := network.NewClient(grant.Connection.Endpoint, network.Options{RunnerID: e.Target.RunnerID, PrivateRoute: &grant.Connection.Route, CAPEM: grant.Connection.CAPEM, Development: e.Development})
	if err != nil {
		return fail("invalid", false)
	}
	defer client.CloseIdleConnections()
	provider, err := gitea.New(forge.Config{OrgID: e.Target.OrgID, ConnectionID: grant.Connection.ID, BaseURL: grant.Connection.Endpoint, Token: grant.Connection.Secret, Client: client})
	if err != nil {
		return fail("invalid", false)
	}
	op := grant.Operation
	switch op.Kind {
	case GiteaProbe:
		v, err2 := provider.ProbeCapabilities(ctx)
		result.Capabilities = &v
		err = err2
	case GiteaInventory:
		v, err2 := provider.ListRepositories(ctx, forge.InventoryRequest{Namespace: op.Inventory.Namespace, Cursor: op.Inventory.Cursor, Limit: op.Inventory.Limit})
		result.Inventory = &v
		err = err2
	case GiteaRepository:
		v, err2 := provider.GetRepository(ctx, op.Repository.Repository)
		result.Repository = &v
		err = err2
	case GiteaResolveRef:
		result.SHA, err = provider.ResolveRef(ctx, op.Ref.Repository, op.Ref.Ref)
	case GiteaReadFile:
		v, err2 := provider.ReadFileAtRef(ctx, op.File.Repository, op.File.Path, op.File.CommitSHA)
		result.File = &v
		err = err2
	case GiteaReadChange:
		v, err2 := provider.ReadChange(ctx, op.Change.Repository, op.Change.ChangeID)
		result.Change = &v
		err = err2
	case GiteaChecks:
		result.Checks, err = provider.ListChecks(ctx, op.Checks.Repository, op.Checks.CommitSHA)
	case GiteaApprovals:
		result.Approvals, err = provider.ReadApprovals(ctx, op.Change.Repository, op.Change.ChangeID)
	default:
		return fail("unsupported", false)
	}
	if err != nil {
		if p, ok := err.(*domain.ProviderError); ok {
			return fail(p.Kind, p.Uncertain)
		}
		return fail("provider", false)
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > MaxResponse {
		return fail("response", false)
	}
	if containsSecret(raw, grant.Connection.Secret) || result.File != nil && bytes.Contains(result.File.Content, []byte(grant.Connection.Secret)) {
		return fail("credential_echo", false)
	}
	return result
}
func containsSecret(data []byte, secret string) bool {
	if secret == "" {
		return false
	}
	quoted, _ := json.Marshal(secret)
	for _, form := range []string{secret, string(quoted[1 : len(quoted)-1]), url.QueryEscape(secret), url.PathEscape(secret), base64.StdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte(secret))} {
		if form != "" && bytes.Contains(data, []byte(form)) {
			return true
		}
	}
	return false
}
func safeControllerURL(endpoint string, development bool) (string, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return "", ErrInvalid
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || !development || ip == nil || !ip.IsLoopback() {
			return "", ErrInvalid
		}
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}
