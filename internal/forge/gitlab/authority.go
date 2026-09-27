package gitlab

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type MergeGuard func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error

func failure(kind, message string) error { return &domain.ProviderError{Kind: kind, Message: message} }
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
func (p *Provider) WithReviewAuthorizer(fn func(context.Context, forge.RepoRef, string, []string) error) *Provider {
	q := *p
	q.authorizeReview = fn
	return &q
}
func (p *Provider) WithMergeGuard(fn MergeGuard) *Provider { q := *p; q.mergeGuard = fn; return &q }
func (p *Provider) WithCheckPublishers(values map[string]string) *Provider {
	q := *p
	q.publishers = map[string]string{}
	for name, id := range values {
		q.publishers[name] = id
	}
	return &q
}
func validSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func validBranch(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") || strings.HasSuffix(value, ".") || strings.ContainsAny(value, " ~^:?*[%\\\x00\r\n\t") || strings.Contains(value, "..") || strings.Contains(value, "@{") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
func validFilePath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00\r\n?#%") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}
func (p *Provider) read(ctx context.Context, route []string, out any) error {
	status, headers, raw, err := p.request(ctx, "GET", route, nil, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return responseError(status, headers)
	}
	return decode(raw, out)
}
func (p *Provider) pages(ctx context.Context, route []string, query url.Values) ([]json.RawMessage, error) {
	if query == nil {
		query = url.Values{}
	}
	out := []json.RawMessage{}
	total := 0
	for page := 1; page <= maxPages; page++ {
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", "100")
		status, headers, raw, err := p.request(ctx, "GET", route, query, nil)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, responseError(status, headers)
		}
		var rows []json.RawMessage
		if decode(raw, &rows) != nil || rows == nil || len(rows) > 100 {
			return nil, failure("provider", "Incomplete collection page")
		}
		total += len(raw)
		if len(out)+len(rows) > 10000 || total > 32<<20 {
			return nil, failure("unsupported", "Collection exceeds bounded inspection profile")
		}
		out = append(out, rows...)
		next, err := nextPage(headers, page)
		if err != nil {
			return nil, err
		}
		if next == "" {
			return out, nil
		}
	}
	return nil, failure("provider", "Collection exceeded pagination bound")
}

func (p *Provider) protectionReader() *Provider {
	if p.inspector != nil {
		return p.inspector
	}
	return p
}
func (p *Provider) WithProtectionReader(reader *Provider) (*Provider, error) {
	if reader == nil || p.config.OrgID == "" || reader.config.OrgID != p.config.OrgID || !strings.EqualFold(reader.base.Host, p.base.Host) || reader.base.Scheme != p.base.Scheme || reader.base.Path != p.base.Path {
		return nil, failure("configuration", "Protection reader must bind the same tenant and API endpoint")
	}
	q := *p
	inspector := *reader
	inspector.inspector = nil
	q.inspector = &inspector
	return &q, nil
}
func parseID(value string) (int64, error) { return strconv.ParseInt(value, 10, 64) }

type pipelineRecord struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	Ref       string `json:"ref"`
	Source    string `json:"source"`
}
