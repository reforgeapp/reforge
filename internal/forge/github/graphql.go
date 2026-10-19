package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

type MergeGuard func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error

func (p *Provider) WithBranchAuthorizer(fn func(context.Context, forge.UpdateBranchRequest) error) *Provider {
	q := *p
	q.authorizeBranch = fn
	return &q
}
func (p *Provider) WithBranchRefreshAuthorizer(fn func(context.Context, forge.RefreshBranchRequest) error) *Provider {
	q := *p
	q.authorizeRefreshBranch = fn
	return &q
}
func (p *Provider) WithChangeAuthorizer(fn func(context.Context, forge.CreateChangeRequest) error) *Provider {
	q := *p
	q.authorizeChange = fn
	return &q
}
func (p *Provider) WithMergeGuard(fn MergeGuard) *Provider { q := *p; q.mergeGuard = fn; return &q }
func (p *Provider) WithGraphQL(endpoint string, client forge.HTTPClient) (*Provider, error) {
	u, err := url.Parse(endpoint)
	if err != nil || client == nil || u.Scheme != p.base.Scheme || !strings.EqualFold(u.Host, p.base.Host) || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || (u.Path != "/graphql" && u.Path != "/api/graphql") {
		return nil, failure("configuration", "A separate same-origin GitHub GraphQL transport is required")
	}
	if c, ok := client.(*http.Client); ok {
		clone := *c
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		if clone.Timeout == 0 || clone.Timeout > 15*time.Second {
			clone.Timeout = 15 * time.Second
		}
		client = &clone
	}
	q := *p
	q.graphURL = u
	q.graphClient = client
	return &q, nil
}
func (p *Provider) graphql(ctx context.Context, query string, variables any, out any, mutation bool) error {
	if p.graphClient == nil || p.graphURL == nil {
		return failure("unsupported", "GitHub GraphQL transport is not configured")
	}
	token := p.config.Token
	var err error
	if p.app != nil {
		token, err = p.installationToken(ctx)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return failure("invalid", "Invalid GraphQL input")
	}
	if len(data) > maxResponseBytes {
		return failure("invalid", "GraphQL request exceeds limit")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.graphURL.String(), bytes.NewReader(data))
	if err != nil {
		return failure("configuration", "Invalid GraphQL endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	result, err := p.graphClient.Do(req)
	method := "GET"
	if mutation {
		method = "POST"
	}
	if err != nil {
		return transportFailure(method, "GitHub GraphQL request failed")
	}
	if result == nil || result.Body == nil {
		return transportFailure(method, "GitHub GraphQL response missing")
	}
	defer result.Body.Close()
	if result.StatusCode == 401 {
		p.invalidateToken(token)
	}
	raw, err := io.ReadAll(io.LimitReader(result.Body, maxResponseBytes+1))
	if err != nil {
		return transportFailure(method, "GitHub GraphQL response could not be read")
	}
	if len(raw) > maxResponseBytes {
		return transportFailure(method, "GitHub GraphQL response exceeded limit")
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		if mutation && (result.StatusCode >= 500 || result.StatusCode == http.StatusRequestTimeout) {
			return transportFailure(method, "GitHub mutation outcome is unknown")
		}
		return responseError(result.StatusCode, result.Header)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Type string `json:"type"`
		} `json:"errors"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return transportFailure(method, "GitHub GraphQL response was invalid")
	}
	if len(envelope.Errors) > 0 {
		if mutation {
			return &domain.ProviderError{Kind: "uncertain", Message: "GitHub mutation was rejected or its outcome requires reconciliation", Uncertain: true}
		}
		return failure("unsupported", "GitHub GraphQL field is unavailable or inaccessible")
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return transportFailure(method, "GitHub GraphQL result missing")
	}
	if json.Unmarshal(envelope.Data, out) != nil {
		return transportFailure(method, "GitHub GraphQL result was invalid")
	}
	return nil
}
func validSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func validBranch(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") || strings.HasSuffix(value, ".") || strings.ContainsAny(value, " ~^:?*[\\\x00\r\n") || strings.Contains(value, "..") || strings.Contains(value, "@{") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
func (p *Provider) UpdateAppBranch(ctx context.Context, in forge.UpdateBranchRequest) (string, error) {
	if p.authorizeBranch == nil || p.graphClient == nil {
		return "", failure("unsupported", "Persisted branch authorization and GraphQL exact-head support are required")
	}
	if !positive(in.Repository.NativeID) || !validSHA(in.BaseSHA) || !validBranch(in.Branch) || !strings.HasPrefix(in.Branch, "reforge/") || len(in.Edits) == 0 || len(in.Edits) > 100 || in.Message == "" || len(in.Message) > 4096 || in.ExpectedOldSHA != "" && in.ExpectedOldSHA != in.BaseSHA {
		return "", failure("invalid", "Invalid app-owned branch update")
	}
	if err := validOperationID(in.OperationID); err != nil {
		return "", err
	}
	if _, err := p.authenticatedBot(ctx); err != nil {
		return "", err
	}
	route, err := repositoryPath(in.Repository)
	if err != nil {
		return "", err
	}
	var observed struct {
		Repository struct {
			ID         string    `json:"id"`
			DatabaseID int64     `json:"databaseId"`
			Ref        *graphRef `json:"ref"`
		} `json:"repository"`
	}
	if err = p.graphql(ctx, `query($owner:String!,$name:String!,$ref:String!){repository(owner:$owner,name:$name){id databaseId ref(qualifiedName:$ref){id target{oid}}}}`, map[string]any{"owner": route[1], "name": route[2], "ref": "refs/heads/" + in.Branch}, &observed, false); err != nil {
		return "", err
	}
	if observed.Repository.ID == "" || strconv.FormatInt(observed.Repository.DatabaseID, 10) != in.Repository.NativeID {
		return "", failure("identity", "GitHub branch repository identity changed")
	}
	additions, deletions := []map[string]string{}, []map[string]string{}
	seen := map[string]bool{}
	total := 0
	for _, edit := range in.Edits {
		if _, err := safePathSegments(edit.Path); err != nil {
			return "", err
		}
		if seen[edit.Path] || strings.EqualFold(edit.Path, ".git") || strings.HasPrefix(strings.ToLower(edit.Path), ".git/") {
			return "", failure("invalid", "Invalid or duplicate file edit")
		}
		seen[edit.Path] = true
		total += len(edit.Content)
		if total > 2<<20 {
			return "", failure("invalid", "Branch edit limit exceeded")
		}
		if edit.Delete {
			deletions = append(deletions, map[string]string{"path": edit.Path})
		} else {
			additions = append(additions, map[string]string{"path": edit.Path, "contents": base64.StdEncoding.EncodeToString(edit.Content)})
		}
	}
	if err := p.authorizeBranch(ctx, in); err != nil {
		return "", err
	}
	branch := observed.Repository.Ref
	if in.ExpectedOldSHA == "" {
		if branch != nil {
			return "", failure("conflict", "Application branch already exists")
		}
		var created struct {
			Created struct {
				Ref *graphRef `json:"ref"`
			} `json:"createRef"`
		}
		input := map[string]any{"repositoryId": observed.Repository.ID, "name": "refs/heads/" + in.Branch, "oid": in.BaseSHA, "clientMutationId": in.OperationID}
		if err := p.graphql(ctx, `mutation($input:CreateRefInput!){createRef(input:$input){ref{id target{oid}}}}`, map[string]any{"input": input}, &created, true); err != nil {
			return "", err
		}
		branch = created.Created.Ref
		if branch == nil || branch.ID == "" || branch.Target.OID != in.BaseSHA {
			return "", transportFailure("POST", "Created branch identity requires reconciliation")
		}
	} else if branch == nil || branch.ID == "" || branch.Target.OID != in.ExpectedOldSHA {
		return "", failure("conflict", "Application branch head changed")
	}
	if err := p.authorizeBranch(ctx, in); err != nil {
		return "", err
	}
	input := map[string]any{"branch": map[string]string{"id": branch.ID}, "expectedHeadOid": in.BaseSHA, "message": map[string]string{"headline": in.Message, "body": operationBody("", in.OperationID)}, "fileChanges": map[string]any{"additions": additions, "deletions": deletions}, "clientMutationId": in.OperationID}
	var out struct {
		Created struct {
			Commit struct {
				OID string `json:"oid"`
			} `json:"commit"`
		} `json:"createCommitOnBranch"`
	}
	if err := p.graphql(ctx, `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid}}}`, map[string]any{"input": input}, &out, true); err != nil {
		return "", err
	}
	if !validSHA(out.Created.Commit.OID) {
		return "", transportFailure("POST", "GitHub returned no committed branch identity")
	}
	head, err := p.ResolveRef(ctx, in.Repository, "heads/"+in.Branch)
	if err != nil || head != out.Created.Commit.OID {
		return "", transportFailure("POST", "Published branch requires canonical reconciliation")
	}
	return head, nil
}

type graphRef struct {
	ID     string `json:"id"`
	Target struct {
		OID string `json:"oid"`
	} `json:"target"`
}

func (p *Provider) CloseIdleConnections() {
	if c, ok := p.graphClient.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
