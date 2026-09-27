package gitea

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

const maxBody = 4 << 20
const maxPages = 20

type Provider struct {
	cfg                    forge.Config
	base                   string
	inspector              *Provider
	authorizeBranch        func(context.Context, forge.UpdateBranchRequest) error
	authorizeRefreshBranch func(context.Context, forge.RefreshBranchRequest) error
	publishers             map[string]string
}

var _ forge.Provider = (*Provider)(nil)

func New(cfg forge.Config) (*Provider, error) {
	u, e := url.Parse(cfg.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || cfg.Client == nil || cfg.Token == "" {
		return nil, failure("configuration", "Gitea requires a fixed-origin client, origin and token")
	}
	if c, ok := cfg.Client.(*http.Client); ok {
		clone := *c
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		cfg.Client = &clone
	}
	base := strings.TrimRight(u.String(), "/")
	if !strings.HasSuffix(base, "/api/v1") {
		base += "/api/v1"
	}
	return &Provider{cfg: cfg, base: base, publishers: map[string]string{}}, nil
}
func (p *Provider) WithBranchRefreshAuthorizer(f func(context.Context, forge.RefreshBranchRequest) error) *Provider {
	q := *p
	q.authorizeRefreshBranch = f
	return &q
}
func (p *Provider) WithBranchAuthorizer(f func(context.Context, forge.UpdateBranchRequest) error) *Provider {
	q := *p
	q.authorizeBranch = f
	return &q
}
func (p *Provider) WithCheckPublishers(m map[string]string) *Provider {
	q := *p
	q.publishers = map[string]string{}
	for k, v := range m {
		q.publishers[k] = v
	}
	return &q
}
func (p *Provider) WithProtectionReader(reader *Provider) (*Provider, error) {
	if reader == nil || reader.base != p.base || reader.cfg.OrgID != p.cfg.OrgID {
		return nil, failure("configuration", "Protection reader must use the same Gitea origin")
	}
	q := *p
	q.inspector = reader
	return &q, nil
}
func failure(kind, message string) error { return &domain.ProviderError{Kind: kind, Message: message} }
func (p *Provider) request(ctx context.Context, method, route string, input, output any) error {
	var body io.Reader
	if input != nil {
		b, e := json.Marshal(input)
		if e != nil {
			return e
		}
		if len(b) > maxBody {
			return failure("invalid", "Request exceeds limit")
		}
		body = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, method, p.base+route, body)
	if e != nil {
		return failure("invalid", "Invalid API request")
	}
	req.Header.Set("Authorization", "token "+p.cfg.Token)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, e := p.cfg.Client.Do(req)
	if e != nil {
		return &domain.ProviderError{Kind: "transport", Message: "Gitea request failed", Uncertain: method != "GET"}
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if e != nil || len(b) > maxBody {
		return &domain.ProviderError{Kind: "response", Message: "Gitea response incomplete or too large", Uncertain: method != "GET"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		kind := "provider"
		switch resp.StatusCode {
		case 401, 403:
			kind = "forbidden"
		case 404:
			kind = "not_found"
		case 409, 412:
			kind = "conflict"
		case 429:
			kind = "rate_limited"
		}
		return &domain.ProviderError{Kind: kind, Message: "Gitea rejected request (HTTP " + strconv.Itoa(resp.StatusCode) + ")", Uncertain: method != "GET" && resp.StatusCode >= 500}
	}
	if output != nil {
		if len(bytes.TrimSpace(b)) == 0 {
			return &domain.ProviderError{Kind: "response", Message: "Gitea omitted the required response body", Uncertain: method != "GET"}
		}
		if json.Unmarshal(b, output) != nil {
			return &domain.ProviderError{Kind: "response", Message: "Invalid Gitea response", Uncertain: method != "GET"}
		}
	}
	return nil
}
func repoPath(r forge.RepoRef) (string, error) {
	parts := strings.Split(r.FullName, "/")
	if len(parts) != 2 || !positive(r.NativeID) || parts[0] == "" || parts[1] == "" || parts[0] == "." || parts[1] == "." || parts[0] == ".." || parts[1] == ".." {
		return "", failure("invalid", "Immutable repository identity required")
	}
	return "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), nil
}
func positive(v string) bool { n, e := strconv.ParseInt(v, 10, 64); return e == nil && n > 0 }
func sha(v string) bool {
	if len(v) != 40 && len(v) != 64 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func pageNumber(v string) (int, error) {
	if v == "" {
		return 1, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > maxPages {
		return 0, failure("pagination", "Invalid or exhausted page cursor")
	}
	return n, nil
}
func filePath(v string) bool {
	return v != "" && len(v) <= 1024 && !strings.HasPrefix(v, "/") && !strings.ContainsAny(v, "\\\x00\r\n") && path.Clean(v) == v && v != ".." && !strings.HasPrefix(v, "../") && !strings.HasPrefix(strings.ToLower(v), ".git/") && strings.ToLower(v) != ".git"
}

type user struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Admin bool   `json:"is_admin"`
}
type repository struct {
	ID             int64           `json:"id"`
	FullName       string          `json:"full_name"`
	URL            string          `json:"html_url"`
	CloneURL       string          `json:"clone_url"`
	Default        string          `json:"default_branch"`
	Archived       bool            `json:"archived"`
	Private        bool            `json:"private"`
	Permissions    map[string]bool `json:"permissions"`
	Merge          bool            `json:"allow_merge_commits"`
	Squash         bool            `json:"allow_squash_merge"`
	Rebase         bool            `json:"allow_rebase"`
	RebaseExplicit bool            `json:"allow_rebase_explicit"`
	FastForward    bool            `json:"allow_fast_forward_only_merge"`
}

func (r repository) ref() forge.RepoRef {
	return forge.RepoRef{NativeID: strconv.FormatInt(r.ID, 10), FullName: r.FullName}
}
func (r repository) normalized() forge.Repository {
	o := forge.Repository{RepoRef: r.ref(), URL: r.URL, CloneURL: r.CloneURL, DefaultBranch: r.Default, Archived: r.Archived, Private: r.Private}
	for _, k := range []string{"admin", "push", "pull"} {
		if r.Permissions[k] {
			o.Permissions = append(o.Permissions, k)
		}
	}
	return o
}
func (p *Provider) repo(ctx context.Context, r forge.RepoRef) (repository, error) {
	route, e := repoPath(r)
	if e != nil {
		return repository{}, e
	}
	var v repository
	e = p.request(ctx, "GET", route, nil, &v)
	if e == nil && (v.ID <= 0 || v.ref() != r) {
		e = failure("identity", "Repository identity changed")
	}
	return v, e
}
func (p *Provider) GetRepository(ctx context.Context, r forge.RepoRef) (forge.Repository, error) {
	v, e := p.repo(ctx, r)
	return v.normalized(), e
}
func (p *Provider) ListRepositories(ctx context.Context, in forge.InventoryRequest) (domain.Page[forge.Repository], error) {
	var out domain.Page[forge.Repository]
	n, e := pageNumber(in.Cursor)
	if e != nil {
		return out, e
	}
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	route := "/user/repos"
	if in.Namespace != "" {
		route = "/orgs/" + url.PathEscape(in.Namespace) + "/repos"
	}
	var rows []repository
	e = p.request(ctx, "GET", route+"?page="+strconv.Itoa(n)+"&limit="+strconv.Itoa(limit), nil, &rows)
	if e != nil && in.Namespace != "" && isNotFound(e) {
		rows = nil
		route = "/users/" + url.PathEscape(in.Namespace) + "/repos"
		e = p.request(ctx, "GET", route+"?page="+strconv.Itoa(n)+"&limit="+strconv.Itoa(limit), nil, &rows)
	}
	if e != nil {
		return out, e
	}
	for _, r := range rows {
		if r.ID <= 0 {
			return out, failure("identity", "Missing repository identity")
		}
		out.Items = append(out.Items, r.normalized())
	}
	out.Complete = len(rows) < limit
	if !out.Complete {
		if n == maxPages {
			return out, failure("pagination", "Inventory exceeds bounded pagination")
		}
		out.NextCursor = strconv.Itoa(n + 1)
	}
	return out, nil
}

func isNotFound(err error) bool {
	var providerErr *domain.ProviderError
	return errors.As(err, &providerErr) && providerErr.Kind == "not_found"
}
func (p *Provider) ResolveRef(ctx context.Context, r forge.RepoRef, ref string) (string, error) {
	if _, e := p.repo(ctx, r); e != nil {
		return "", e
	}
	if ref == "" {
		return "", failure("invalid", "Reference required")
	}
	route, _ := repoPath(r)
	var v struct {
		SHA string `json:"sha"`
	}
	e := p.request(ctx, "GET", route+"/git/commits/"+url.PathEscape(ref)+"?stat=false&verification=false", nil, &v)
	if e == nil && !sha(v.SHA) {
		e = failure("response", "Missing commit identity")
	}
	return v.SHA, e
}
func (p *Provider) ReadFileAtRef(ctx context.Context, r forge.RepoRef, name, ref string) (forge.File, error) {
	if !filePath(name) || !sha(ref) {
		return forge.File{}, failure("invalid", "Canonical file path and immutable commit required")
	}
	if _, e := p.repo(ctx, r); e != nil {
		return forge.File{}, e
	}
	route, _ := repoPath(r)
	var v struct {
		Path, SHA, Type, Encoding, Content string
		Size                               int64
	}
	e := p.request(ctx, "GET", route+"/contents/"+url.PathEscape(name)+"?ref="+url.QueryEscape(ref), nil, &v)
	if e != nil {
		return forge.File{}, e
	}
	if v.Type != "file" || v.Encoding != "base64" || v.Path != name || !sha(v.SHA) || v.Size > maxBody {
		return forge.File{}, failure("unsupported", "Only bounded regular repository files are supported")
	}
	b, e := base64.StdEncoding.DecodeString(strings.ReplaceAll(v.Content, "\n", ""))
	if e != nil || int64(len(b)) != v.Size {
		return forge.File{}, failure("response", "Invalid file encoding")
	}
	return forge.File{Path: name, SHA: v.SHA, Content: b}, nil
}
func (p *Provider) VerifyWebhook(h http.Header, b []byte) error {
	if len(b) > maxBody || p.cfg.WebhookSecret == "" {
		return failure("unauthorized", "Webhook verification unavailable")
	}
	got, e := hex.DecodeString(h.Get("X-Gitea-Signature"))
	mac := hmac.New(sha256.New, []byte(p.cfg.WebhookSecret))
	mac.Write(b)
	if e != nil || !hmac.Equal(got, mac.Sum(nil)) {
		return failure("unauthorized", "Invalid webhook signature")
	}
	return nil
}
func (p *Provider) DecodeEvent(h http.Header, b []byte) (forge.Event, error) {
	if e := p.VerifyWebhook(h, b); e != nil {
		return forge.Event{}, e
	}
	var v struct {
		Repository repository `json:"repository"`
		After      string     `json:"after"`
		Number     int64      `json:"number"`
		Pull       *pull      `json:"pull_request"`
	}
	if json.Unmarshal(b, &v) != nil || v.Repository.ID <= 0 {
		return forge.Event{}, failure("invalid", "Invalid webhook repository identity")
	}
	out := forge.Event{DeliveryID: h.Get("X-Gitea-Delivery"), Kind: h.Get("X-Gitea-Event"), Repository: v.Repository.ref(), HeadSHA: v.After}
	if out.DeliveryID == "" || out.Kind == "" {
		return forge.Event{}, failure("invalid", "Missing webhook identity")
	}
	if v.Pull != nil {
		out.ChangeID = strconv.FormatInt(v.Pull.Number, 10)
		out.HeadSHA = v.Pull.Head.SHA
	}
	return out, nil
}
func (p *Provider) ProbeCapabilities(ctx context.Context) (forge.Capabilities, error) {
	var v struct {
		Version string `json:"version"`
	}
	if e := p.request(ctx, "GET", "/version", nil, &v); e != nil {
		return forge.Capabilities{}, e
	}
	if _, e := p.actor(ctx); e != nil {
		return forge.Capabilities{}, e
	}
	out := forge.Capabilities{Provider: "gitea", ServerVersion: v.Version, Features: map[string]domain.Capability{}}
	for _, name := range []string{"inventory", "webhooks", "pull_requests", "checks", "approvals"} {
		out.Features[name] = domain.Capability{State: domain.Supported, Version: v.Version, Source: "Gitea REST API", LastChecked: time.Now().UTC()}
	}
	for _, name := range []string{"codeowners_enforcement", "native_queue", "protected_deployments", "actions_delivery"} {
		out.Features[name] = domain.Capability{State: domain.Unknown, Reason: "Native enforcement has not been certified for this connection", Version: v.Version, LastChecked: time.Now().UTC()}
	}
	state := domain.Unknown
	if v.Version == "1.27.3" {
		state = domain.Supported
	}
	out.Features["guarded_branch_update"] = domain.Capability{State: state, Version: v.Version, Reason: "Requires persisted branch authorization and compare-and-swap"}
	out.Features["strict_target_merge"] = domain.Capability{State: state, Version: v.Version, Reason: "Only fast-forward-only; requires block_on_outdated_branch and non-bypass actor"}
	return out, nil
}
