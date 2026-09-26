package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

const (
	defaultPageSize  = 50
	maxPageSize      = 100
	maxPages         = 1000
	maxResponseBytes = 4 << 20
	apiVersion       = "2022-11-28"
)

type Provider struct {
	deliveryGuard   forge.DeliveryGuard
	config          forge.Config
	base            *url.URL
	app             *installationAuth
	graphURL        *url.URL
	graphClient     forge.HTTPClient
	authorizeBranch func(context.Context, forge.UpdateBranchRequest) error
	authorizeChange func(context.Context, forge.CreateChangeRequest) error
	mergeGuard      MergeGuard
	authorizeReview func(context.Context, forge.RepoRef, string, []string) error
	user            *tokenUser
}

func New(config forge.Config) (*Provider, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("github client is required")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("github base URL is required")
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, fmt.Errorf("github token is required")
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.Opaque != "" || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("github base URL is invalid")
	}
	if c, ok := config.Client.(*http.Client); ok {
		clone := *c
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		if clone.Timeout == 0 || clone.Timeout > 15*time.Second {
			clone.Timeout = 15 * time.Second
		}
		config.Client = &clone
	}
	base.Path = strings.TrimRight(base.Path, "/")
	base.RawPath = ""
	return &Provider{config: config, base: base, user: &tokenUser{}}, nil
}

func (p *Provider) ProbeCapabilities(ctx context.Context) (forge.Capabilities, error) {
	if p.app != nil {
		if _, err := p.ListRepositories(ctx, forge.InventoryRequest{Namespace: "installation", Limit: 1}); err != nil {
			return forge.Capabilities{}, err
		}
	} else {
		var actor struct {
			ID int64 `json:"id"`
		}
		if err := p.get(ctx, []string{"user"}, nil, &actor); err != nil {
			return forge.Capabilities{}, err
		}
		if actor.ID <= 0 {
			return forge.Capabilities{}, failure("auth", "Authenticated GitHub identity missing")
		}
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"meta"}, nil, nil)
	if err != nil {
		return forge.Capabilities{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Capabilities{}, responseError(status, headers)
	}
	var meta struct {
		InstalledVersion string `json:"installed_version"`
	}
	if err := decode(body, &meta); err != nil {
		return forge.Capabilities{}, err
	}
	version := p.config.ServerVersion
	if version == "" {
		version = meta.InstalledVersion
	}
	if version == "" {
		version = "cloud"
	}
	features := map[string]domain.Capability{}
	for _, name := range []string{"inventory.repositories", "events.webhooks", "changes.read", "changes.create", "changes.review", "checks.check-runs"} {
		features[name] = domain.Capability{State: domain.Supported, Scope: "github-rest", Reason: "implemented by this adapter", Source: "github-rest", Version: version, LastChecked: time.Now().UTC()}
	}
	for _, name := range []string{"changes.create", "changes.review", "changes.update_branch", "protection.rules", "merge.native", "merge.queue", "delivery.workflows"} {
		features[name] = domain.Capability{State: domain.Unknown, Scope: "github-rest", Reason: "requires per-repository permission, persisted authorization and live native enforcement qualification", Source: "github-rest", Version: version, LastChecked: time.Now().UTC()}
	}
	features["delivery.workflows"] = domain.Capability{State: domain.Unsupported, Reason: "Deployment workflow adapter is not implemented", Source: "github-rest", Version: version, LastChecked: time.Now().UTC()}
	return forge.Capabilities{Provider: "github", ServerVersion: version, Features: features}, nil
}

func (p *Provider) ListRepositories(ctx context.Context, request forge.InventoryRequest) (domain.Page[forge.Repository], error) {
	page, err := pageNumber(request.Cursor)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	limit := boundedLimit(request.Limit)
	if name, ok := singleRepository(request.Namespace); ok {
		repository, err := p.GetRepository(ctx, forge.RepoRef{FullName: name})
		if err != nil {
			return domain.Page[forge.Repository]{}, err
		}
		return domain.Page[forge.Repository]{Items: []forge.Repository{repository}, Complete: true}, nil
	}
	segments, err := repositoryNamespace(request.Namespace)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	query := url.Values{"per_page": {strconv.Itoa(limit)}, "page": {strconv.Itoa(page)}}
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, query, nil)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	if status < 200 || status >= 300 {
		return domain.Page[forge.Repository]{}, responseError(status, headers)
	}
	var raw []githubRepository
	if segments[0] == "installation" {
		var response struct {
			Repositories []githubRepository `json:"repositories"`
		}
		if err := decode(body, &response); err != nil {
			return domain.Page[forge.Repository]{}, err
		}
		if response.Repositories == nil {
			return domain.Page[forge.Repository]{}, failure("provider", "Missing installation repository inventory")
		}
		raw = response.Repositories
	} else if err := decode(body, &raw); err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	if raw == nil || len(raw) > limit {
		return domain.Page[forge.Repository]{}, failure("provider", "Invalid repository inventory page")
	}
	items := make([]forge.Repository, 0, len(raw))
	for _, repository := range raw {
		if repository.ID <= 0 || repository.FullName == "" {
			return domain.Page[forge.Repository]{}, failure("identity", "Missing inventory repository identity")
		}
		items = append(items, mapRepository(repository))
	}
	next, err := p.nextCursor(headers, page)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	return domain.Page[forge.Repository]{Items: items, NextCursor: next, Complete: next == ""}, nil
}

func (p *Provider) GetRepository(ctx context.Context, reference forge.RepoRef) (forge.Repository, error) {
	segments, err := repositoryPath(reference)
	if err != nil {
		return forge.Repository{}, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, nil, nil)
	if err != nil {
		return forge.Repository{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Repository{}, responseError(status, headers)
	}
	var raw githubRepository
	if err := decode(body, &raw); err != nil {
		return forge.Repository{}, err
	}
	if reference.NativeID != "" && strconv.FormatInt(raw.ID, 10) != reference.NativeID {
		return forge.Repository{}, failure("identity", "Repository identity changed")
	}
	if raw.ID <= 0 || raw.FullName == "" {
		return forge.Repository{}, failure("identity", "Missing repository identity")
	}
	return mapRepository(raw), nil
}

func (p *Provider) ReadFileAtRef(ctx context.Context, reference forge.RepoRef, filePath string, ref string) (forge.File, error) {
	if ref == "" {
		return forge.File{}, &domain.ProviderError{Kind: "invalid", Message: "github ref is empty"}
	}
	segments, err := repositoryPath(reference)
	if err != nil {
		return forge.File{}, err
	}
	fileSegments, err := safePathSegments(filePath)
	if err != nil {
		return forge.File{}, err
	}
	segments = append(segments, "contents")
	segments = append(segments, fileSegments...)
	query := url.Values{"ref": {ref}}
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, query, nil)
	if err != nil {
		return forge.File{}, err
	}
	if status < 200 || status >= 300 {
		return forge.File{}, responseError(status, headers)
	}
	var raw githubContent
	if err := decode(body, &raw); err != nil {
		return forge.File{}, err
	}
	if raw.Type != "file" || raw.Encoding != "base64" {
		return forge.File{}, &domain.ProviderError{Kind: "unsupported", Message: "github content response is not a file"}
	}
	encoded := strings.Join(strings.Fields(raw.Content), "")
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return forge.File{}, &domain.ProviderError{Kind: "provider", Message: "github returned invalid file content"}
	}
	return forge.File{Path: raw.Path, SHA: raw.SHA, Content: content}, nil
}

func (p *Provider) ResolveRef(ctx context.Context, reference forge.RepoRef, ref string) (string, error) {
	segments, err := repositoryPath(reference)
	if err != nil {
		return "", err
	}
	refSegments, err := refSegments(ref)
	if err != nil {
		return "", err
	}
	segments = append(segments, "git", "ref")
	segments = append(segments, refSegments...)
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, nil, nil)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", responseError(status, headers)
	}
	var raw struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := decode(body, &raw); err != nil {
		return "", err
	}
	if raw.Object.SHA == "" {
		return "", &domain.ProviderError{Kind: "provider", Message: "github reference has no object SHA"}
	}
	return raw.Object.SHA, nil
}

func (p *Provider) VerifyWebhook(headers http.Header, payload []byte) error {
	if p.config.WebhookSecret == "" {
		return &domain.ProviderError{Kind: "auth", Message: "github webhook secret is not configured"}
	}
	signature := headerValue(headers, "X-Hub-Signature-256")
	if !strings.HasPrefix(signature, "sha256=") {
		return &domain.ProviderError{Kind: "auth", Message: "github webhook signature is missing"}
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil || len(provided) != sha256.Size {
		return &domain.ProviderError{Kind: "auth", Message: "github webhook signature is invalid"}
	}
	digest := hmac.New(sha256.New, []byte(p.config.WebhookSecret))
	_, _ = digest.Write(payload)
	if !hmac.Equal(provided, digest.Sum(nil)) {
		return &domain.ProviderError{Kind: "auth", Message: "github webhook signature is invalid"}
	}
	return nil
}

func (p *Provider) DecodeEvent(headers http.Header, payload []byte) (forge.Event, error) {
	if len(payload) > maxResponseBytes {
		return forge.Event{}, &domain.ProviderError{Kind: "provider", Message: "github webhook payload is too large"}
	}
	if err := p.VerifyWebhook(headers, payload); err != nil {
		return forge.Event{}, err
	}
	var raw githubEvent
	if err := json.Unmarshal(payload, &raw); err != nil {
		return forge.Event{}, &domain.ProviderError{Kind: "provider", Message: "github webhook payload is invalid"}
	}
	event := forge.Event{DeliveryID: headerValue(headers, "X-GitHub-Delivery"), Kind: headerValue(headers, "X-GitHub-Event"), Repository: forge.RepoRef{NativeID: strconv.FormatInt(raw.Repository.ID, 10), FullName: raw.Repository.FullName}}
	if event.DeliveryID == "" || event.Kind == "" || raw.Repository.ID <= 0 || raw.Repository.FullName == "" {
		return forge.Event{}, failure("provider", "Webhook delivery or repository identity missing")
	}
	if raw.PullRequest.Number != 0 {
		event.ChangeID = strconv.FormatInt(raw.PullRequest.Number, 10)
		event.HeadSHA = raw.PullRequest.Head.SHA
	}
	if event.Kind == "push" {
		event.HeadSHA = raw.After
	}
	if event.Kind == "merge_group" {
		event.HeadSHA = raw.MergeGroup.HeadSHA
	}
	return event, nil
}

func headerValue(headers http.Header, key string) string {
	if value := headers.Get(key); value != "" {
		return value
	}
	for header, values := range headers {
		if strings.EqualFold(header, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func (p *Provider) ReconcileChanges(ctx context.Context, reference forge.RepoRef, cursor string) (domain.Page[forge.Change], error) {
	return p.listChanges(ctx, reference, cursor, "all")
}

func (p *Provider) ListChecks(ctx context.Context, reference forge.RepoRef, ref string) ([]forge.Check, error) {
	segments, err := repositoryPath(reference)
	if err != nil {
		return nil, err
	}
	segments = append(segments, "commits", ref, "check-runs")
	checks := make([]forge.Check, 0)
	for page := 1; page <= maxPages; {
		query := url.Values{"per_page": {strconv.Itoa(maxPageSize)}, "page": {strconv.Itoa(page)}, "filter": {"latest"}}
		status, headers, body, requestErr := p.request(ctx, http.MethodGet, segments, query, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		if status < 200 || status >= 300 {
			return nil, responseError(status, headers)
		}
		var raw githubCheckRuns
		if err := decode(body, &raw); err != nil {
			return nil, err
		}
		if raw.CheckRuns == nil || len(raw.CheckRuns) > maxPageSize {
			return nil, failure("provider", "Check run collection missing")
		}
		for _, check := range raw.CheckRuns {
			if check.ID <= 0 || check.App.ID <= 0 || check.Name == "" || check.HeadSHA == "" {
				return nil, failure("provider", "Check publisher or identity is incomplete")
			}
			switch check.Status {
			case "queued", "in_progress", "completed", "waiting", "pending", "requested":
			default:
				return nil, failure("provider", "Unknown check status")
			}
			checks = append(checks, mapCheck(check))
		}
		next, err := p.nextCursor(headers, page)
		if err != nil {
			return nil, err
		}
		if next == "" {
			return checks, nil
		}
		page, err = strconv.Atoi(next)
		if err != nil || page <= 0 {
			return nil, &domain.ProviderError{Kind: "provider", Message: "github pagination cursor is invalid"}
		}
	}
	return nil, &domain.ProviderError{Kind: "provider", Message: "github pagination exceeded the safety bound"}
}

func (p *Provider) ListBotWork(ctx context.Context, reference forge.RepoRef) ([]forge.Change, error) {
	changes := make([]forge.Change, 0)
	cursor := ""
	for page := 0; page < maxPages; page++ {
		result, err := p.listChanges(ctx, reference, cursor, "open")
		if err != nil {
			return nil, err
		}
		for _, change := range result.Items {
			if strings.EqualFold(change.AuthorType, "bot") {
				changes = append(changes, change)
			}
		}
		if result.NextCursor == "" {
			return changes, nil
		}
		cursor = result.NextCursor
	}
	return nil, &domain.ProviderError{Kind: "provider", Message: "github pagination exceeded the safety bound"}
}

func (p *Provider) CreateChange(ctx context.Context, request forge.CreateChangeRequest) (forge.Change, error) {
	if p.authorizeChange == nil {
		return forge.Change{}, failure("unsupported", "Persisted publication authorization is required")
	}
	if !positive(request.Repository.NativeID) || !validSHA(request.ExpectedHeadSHA) || !validBranch(request.HeadBranch) || !validBranch(request.TargetBranch) || request.Title == "" {
		return forge.Change{}, failure("invalid", "Incomplete app-owned pull request")
	}
	actor, err := p.authenticatedBot(ctx)
	if err != nil {
		return forge.Change{}, err
	}
	if err = p.authorizeChange(ctx, request); err != nil {
		return forge.Change{}, err
	}

	if err := validOperationID(request.OperationID); err != nil {
		return forge.Change{}, err
	}
	existing, err := p.FindChangeByOperation(ctx, request.Repository, request.OperationID, request.HeadBranch, request.TargetBranch)
	if err != nil {
		return forge.Change{}, err
	}
	if existing != nil {
		if existing.HeadSHA != request.ExpectedHeadSHA {
			return forge.Change{}, failure("conflict", "Existing operation head changed")
		}
		return *existing, nil
	}
	observed, err := p.ResolveRef(ctx, request.Repository, "heads/"+branchName(request.HeadBranch))
	if err != nil {
		return forge.Change{}, err
	}
	if observed != request.ExpectedHeadSHA {
		return forge.Change{}, &domain.ProviderError{Kind: "conflict", Message: "github head changed before pull request creation"}
	}
	segments, err := repositoryPath(request.Repository)
	if err != nil {
		return forge.Change{}, err
	}
	segments = append(segments, "pulls")
	body := struct {
		Title string `json:"title"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Body  string `json:"body"`
		Draft bool   `json:"draft"`
	}{Title: request.Title, Head: request.HeadBranch, Base: request.TargetBranch, Body: operationBody(request.Body, request.OperationID), Draft: request.Draft}
	encoded, err := json.Marshal(body)
	if err != nil {
		return forge.Change{}, &domain.ProviderError{Kind: "provider", Message: "github request encoding failed"}
	}
	if err = p.authorizeChange(ctx, request); err != nil {
		return forge.Change{}, err
	}
	status, headers, responseBody, err := p.request(ctx, http.MethodPost, segments, nil, &requestBody{data: encoded})
	if err != nil {
		return forge.Change{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Change{}, responseError(status, headers)
	}
	var raw githubPullRequest
	if err := decode(responseBody, &raw); err != nil {
		return forge.Change{}, transportFailure("POST", "Published pull request response was invalid")
	}
	change := mapChange(raw)
	if !VerifyOperationOwner(change, request.Repository, request.OperationID, request.HeadBranch, request.TargetBranch, actor) || change.HeadSHA != request.ExpectedHeadSHA {
		return forge.Change{}, transportFailure("POST", "Published pull request identity requires reconciliation")
	}
	change.TargetSHA, err = p.ResolveRef(ctx, change.TargetRepository, "heads/"+change.TargetBranch)
	if err != nil {
		return forge.Change{}, transportFailure("POST", "Published pull request target requires reconciliation")
	}
	return change, nil
}

func (p *Provider) FindChangeByOperation(ctx context.Context, reference forge.RepoRef, operationID string, headBranch string, targetBranch string) (*forge.Change, error) {
	if err := validOperationID(operationID); err != nil {
		return nil, err
	}
	actor, err := p.authenticatedBot(ctx)
	if err != nil {
		return nil, err
	}
	cursor := ""
	var found *forge.Change
	for page := 0; page < maxPages; page++ {
		result, err := p.listChangesWithFilters(ctx, reference, cursor, "all", headBranch, targetBranch)
		if err != nil {
			return nil, err
		}
		for _, change := range result.Items {
			if VerifyOperationOwner(change, reference, operationID, headBranch, targetBranch, actor) {
				if found != nil && found.ID != change.ID {
					return nil, failure("conflict", "Multiple pull requests claim the same operation")
				}
				found = &change
			}
		}
		if result.NextCursor == "" {
			if found != nil {
				found.TargetSHA, err = p.ResolveRef(ctx, found.TargetRepository, "heads/"+found.TargetBranch)
				if err != nil {
					return nil, err
				}
			}
			return found, nil
		}
		cursor = result.NextCursor
	}
	return nil, &domain.ProviderError{Kind: "provider", Message: "github pagination exceeded the safety bound"}
}

func operationChangeMatches(change forge.Change, reference forge.RepoRef, operationID string, headBranch string, targetBranch string) bool {
	if change.OperationID != operationID {
		return false
	}
	if !sameRepoRef(change.Repository, reference) || !sameRepoRef(change.HeadRepository, reference) || !sameRepoRef(change.TargetRepository, reference) {
		return false
	}
	return change.HeadBranch == branchName(headBranch) && change.TargetBranch == targetBranch
}

func sameRepoRef(actual forge.RepoRef, expected forge.RepoRef) bool {
	if expected.NativeID == "" || actual.NativeID != expected.NativeID {
		return false
	}
	return expected.FullName == "" || strings.EqualFold(actual.FullName, expected.FullName)
}

func VerifyOperationOwner(change forge.Change, reference forge.RepoRef, operationID string, headBranch string, targetBranch string, actorID string) bool {
	return actorID != "" && change.AuthorID == actorID && operationChangeMatches(change, reference, operationID, headBranch, targetBranch)
}

func branchName(value string) string {
	if index := strings.LastIndexByte(value, ':'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func (p *Provider) ReadChange(ctx context.Context, reference forge.RepoRef, changeID string) (forge.Change, error) {
	if !positive(changeID) {
		return forge.Change{}, &domain.ProviderError{Kind: "invalid", Message: "github pull request ID is invalid"}
	}
	segments, err := repositoryPath(reference)
	if err != nil {
		return forge.Change{}, err
	}
	segments = append(segments, "pulls", changeID)
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, nil, nil)
	if err != nil {
		return forge.Change{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Change{}, responseError(status, headers)
	}
	var raw githubPullRequest
	if err := decode(body, &raw); err != nil {
		return forge.Change{}, err
	}
	change := mapChange(raw)
	if change.ID != changeID || reference.NativeID != "" && !sameRepoRef(change.TargetRepository, reference) {
		return forge.Change{}, failure("identity", "Pull request repository identity changed")
	}
	change.TargetSHA, err = p.ResolveRef(ctx, change.TargetRepository, "heads/"+change.TargetBranch)
	if err != nil {
		return forge.Change{}, err
	}
	return change, nil
}

func (p *Provider) RequestReview(ctx context.Context, reference forge.RepoRef, changeID string, reviewers []string) error {
	if p.authorizeReview == nil {
		return failure("unsupported", "Persisted review-request authorization required")
	}
	if _, err := p.authenticatedBot(ctx); err != nil {
		return err
	}
	if err := p.authorizeReview(ctx, reference, changeID, append([]string(nil), reviewers...)); err != nil {
		return err
	}

	if !positive(changeID) || len(reviewers) == 0 {
		return &domain.ProviderError{Kind: "invalid", Message: "github review request is invalid"}
	}
	segments, err := repositoryPath(reference)
	if err != nil {
		return err
	}
	segments = append(segments, "pulls", changeID, "requested_reviewers")
	encoded, err := json.Marshal(struct {
		Reviewers []string `json:"reviewers"`
	}{Reviewers: reviewers})
	if err != nil {
		return &domain.ProviderError{Kind: "provider", Message: "github request encoding failed"}
	}
	status, headers, _, err := p.request(ctx, http.MethodPost, segments, nil, &requestBody{data: encoded})
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return responseError(status, headers)
	}
	return nil
}

type requestBody struct {
	data    []byte
	headers http.Header
}

func (p *Provider) request(ctx context.Context, method string, segments []string, query url.Values, body *requestBody) (int, http.Header, []byte, error) {
	credential := p.config.Token
	if p.app != nil {
		var err error
		credential, err = p.installationToken(ctx)
		if err != nil {
			return 0, nil, nil, err
		}
	}
	status, headers, raw, err := p.requestToken(ctx, method, segments, query, body, credential)
	if status == 401 {
		p.invalidateToken(credential)
	}
	return status, headers, raw, err
}
func (p *Provider) requestToken(ctx context.Context, method string, segments []string, query url.Values, body *requestBody, credential string) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	target := p.baseURL(segments)
	if query != nil {
		target.RawQuery = query.Encode()
	}
	var reader io.Reader
	if body != nil {
		if len(body.data) > maxResponseBytes {
			return 0, nil, nil, failure("invalid", "GitHub request exceeds limit")
		}
		reader = strings.NewReader(string(body.data))
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return 0, nil, nil, &domain.ProviderError{Kind: "invalid", Message: "github request URL is invalid"}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		for key, values := range body.headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	response, err := p.config.Client.Do(req)
	if err != nil {
		return 0, nil, nil, transportFailure(method, "github request failed")
	}
	if response == nil || response.Body == nil {
		return 0, nil, nil, transportFailure(method, "GitHub response missing")
	}
	defer response.Body.Close()

	if method != "GET" && method != "HEAD" && (response.StatusCode >= 500 || response.StatusCode == http.StatusRequestTimeout) {
		return response.StatusCode, response.Header, nil, transportFailure(method, "GitHub mutation outcome is unknown")
	}
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	responseBody, readErr := io.ReadAll(limited)
	if readErr != nil {
		return response.StatusCode, response.Header, nil, transportFailure(method, "github response could not be read")
	}
	if len(responseBody) > maxResponseBytes {
		return response.StatusCode, response.Header, nil, transportFailure(method, "github response exceeded the safety limit")
	}
	return response.StatusCode, response.Header, responseBody, nil
}

func (p *Provider) baseURL(segments []string) *url.URL {
	result := *p.base
	raw := strings.TrimRight(p.base.Path, "/")
	plain := raw
	escaped := raw
	for _, segment := range segments {
		rawSegment := url.PathEscape(segment)
		escaped += "/" + rawSegment
		plain += "/" + segment
	}
	result.Path = plain
	result.RawPath = escaped
	return &result
}

func (p *Provider) nextCursor(headers http.Header, current int) (string, error) {
	link := headers.Get("Link")
	if link == "" {
		return "", nil
	}
	for _, value := range strings.Split(link, ",") {
		parts := strings.SplitN(strings.TrimSpace(value), ";", 2)
		if len(parts) != 2 || !strings.Contains(parts[1], `rel="next"`) {
			continue
		}
		candidate := strings.Trim(parts[0], " <>")
		parsed, err := url.Parse(candidate)
		if err != nil || !p.sameBase(parsed) {
			return "", &domain.ProviderError{Kind: "provider", Message: "github pagination link leaves the configured origin"}
		}
		page := parsed.Query().Get("page")
		if page == "" {
			return "", &domain.ProviderError{Kind: "provider", Message: "github pagination link has no page cursor"}
		}
		if number, err := strconv.Atoi(page); err != nil || number != current+1 || number > maxPages {
			return "", &domain.ProviderError{Kind: "provider", Message: "github pagination cursor is invalid"}
		}
		return page, nil
	}
	return "", nil
}

func (p *Provider) sameBase(candidate *url.URL) bool {
	if candidate.User != nil || candidate.Fragment != "" || candidate.Opaque != "" || candidate.Scheme != p.base.Scheme || !strings.EqualFold(candidate.Host, p.base.Host) {
		return false
	}
	basePath := strings.TrimRight(p.base.Path, "/")
	candidatePath := strings.TrimRight(candidate.Path, "/")
	return candidatePath == basePath || strings.HasPrefix(candidatePath, basePath+"/")
}

func (p *Provider) listChanges(ctx context.Context, reference forge.RepoRef, cursor string, state string) (domain.Page[forge.Change], error) {
	return p.listChangesWithFilters(ctx, reference, cursor, state, "", "")
}

func (p *Provider) listChangesWithFilters(ctx context.Context, reference forge.RepoRef, cursor string, state string, head string, base string) (domain.Page[forge.Change], error) {
	segments, err := repositoryPath(reference)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	segments = append(segments, "pulls")
	page, err := pageNumber(cursor)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	query := url.Values{"state": {state}, "per_page": {strconv.Itoa(maxPageSize)}, "page": {strconv.Itoa(page)}}
	if head != "" {
		query.Set("head", githubHeadFilter(reference, head))
	}
	if base != "" {
		query.Set("base", base)
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, query, nil)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	if status < 200 || status >= 300 {
		return domain.Page[forge.Change]{}, responseError(status, headers)
	}
	var raw []githubPullRequest
	if err := decode(body, &raw); err != nil {
		return domain.Page[forge.Change]{}, err
	}
	if raw == nil || len(raw) > maxPageSize {
		return domain.Page[forge.Change]{}, failure("provider", "Invalid pull request page")
	}
	items := make([]forge.Change, 0, len(raw))
	for _, pull := range raw {
		change := mapChange(pull)
		if pull.Number <= 0 || reference.NativeID != "" && !sameRepoRef(change.TargetRepository, reference) {
			return domain.Page[forge.Change]{}, failure("identity", "Pull request inventory identity changed")
		}
		items = append(items, change)
	}
	next, err := p.nextCursor(headers, page)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	return domain.Page[forge.Change]{Items: items, NextCursor: next, Complete: next == ""}, nil
}

func githubHeadFilter(reference forge.RepoRef, head string) string {
	if strings.Contains(head, ":") {
		return head
	}
	parts := strings.Split(reference.FullName, "/")
	if len(parts) == 2 && parts[0] != "" {
		return parts[0] + ":" + head
	}
	return head
}

func decode(body []byte, target any) error {
	if err := json.Unmarshal(body, target); err != nil {
		return &domain.ProviderError{Kind: "provider", Message: "github response was invalid JSON"}
	}
	return nil
}

func responseError(status int, headers http.Header) error {
	kind := "provider"
	switch {
	case status == http.StatusUnauthorized:
		kind = "auth"
	case status == http.StatusTooManyRequests || headers.Get("X-RateLimit-Remaining") == "0":
		kind = "rate_limit"
	case status == http.StatusForbidden:
		kind = "scope"
	case status == http.StatusConflict:
		kind = "conflict"
	case status == http.StatusNotFound:
		kind = "not_found"
	case status == http.StatusUnprocessableEntity:
		kind = "invalid"
	case status >= 500:
		kind = "transient"
	}
	return &domain.ProviderError{Kind: kind, Message: fmt.Sprintf("github request returned HTTP %d", status), RetryAfter: retryAfter(headers)}
}

func retryAfter(headers http.Header) time.Duration {
	value := headers.Get("Retry-After")
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func singleRepository(namespace string) (string, bool) {
	namespace = strings.TrimSpace(namespace)
	namespace = strings.TrimPrefix(strings.TrimPrefix(namespace, "https://"), "github.com/")
	namespace = strings.TrimSuffix(strings.TrimSuffix(namespace, "/"), ".git")
	owner, name, ok := strings.Cut(namespace, "/")
	if !ok || strings.Contains(name, "/") || owner == "user" || owner == "org" || owner == "installation" {
		return "", false
	}
	return namespace, true
}

func repositoryNamespace(namespace string) ([]string, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || namespace == "user" {
		return []string{"user", "repos"}, nil
	}
	if strings.HasPrefix(namespace, "user:") {
		namespace = strings.TrimPrefix(namespace, "user:")
		if namespace == "" || namespace == "." || namespace == ".." || strings.ContainsAny(namespace, "/\\%?#\x00") {
			return nil, &domain.ProviderError{Kind: "invalid", Message: "github user namespace is invalid"}
		}
		return []string{"users", namespace, "repos"}, nil
	}
	if strings.HasPrefix(namespace, "user/") {
		namespace = strings.TrimPrefix(namespace, "user/")
		if namespace == "" || namespace == "." || namespace == ".." || strings.ContainsAny(namespace, "/\\%?#\x00") {
			return nil, &domain.ProviderError{Kind: "invalid", Message: "github user namespace is invalid"}
		}
		return []string{"users", namespace, "repos"}, nil
	}
	if namespace == "installation" || strings.HasPrefix(namespace, "installation:") || strings.HasPrefix(namespace, "installation/") {
		return []string{"installation", "repositories"}, nil
	}
	if strings.HasPrefix(namespace, "org:") {
		namespace = strings.TrimPrefix(namespace, "org:")
	}
	if strings.HasPrefix(namespace, "org/") {
		namespace = strings.TrimPrefix(namespace, "org/")
	}
	if namespace == "" || namespace == "." || namespace == ".." || strings.ContainsAny(namespace, "/\\%?#\x00") {
		return nil, &domain.ProviderError{Kind: "invalid", Message: "github repository namespace is invalid"}
	}
	return []string{"orgs", namespace, "repos"}, nil
}

func repositoryPath(reference forge.RepoRef) ([]string, error) {
	parts := strings.Split(reference.FullName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." || strings.ContainsAny(reference.FullName, "\\%\x00") || strings.ContainsAny(parts[0], "?#") || strings.ContainsAny(parts[1], "?#") {
		return nil, &domain.ProviderError{Kind: "invalid", Message: "github repository full name is invalid"}
	}
	return []string{"repos", parts[0], parts[1]}, nil
}

func safePathSegments(value string) ([]string, error) {
	if value == "" {
		return nil, &domain.ProviderError{Kind: "invalid", Message: "github file path is empty"}
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "?#\\%\x00") {
			return nil, &domain.ProviderError{Kind: "invalid", Message: "github file path is invalid"}
		}
	}
	return parts, nil
}

func refSegments(ref string) ([]string, error) {
	if ref == "" {
		return nil, &domain.ProviderError{Kind: "invalid", Message: "github ref is empty"}
	}
	ref = strings.TrimPrefix(ref, "refs/")
	parts := strings.Split(ref, "/")
	if len(parts) < 2 || (parts[0] != "heads" && parts[0] != "tags") {
		parts = append([]string{"heads"}, parts...)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "?#\\%\x00") {
			return nil, &domain.ProviderError{Kind: "invalid", Message: "github ref is invalid"}
		}
	}
	return parts, nil
}

func pageNumber(cursor string) (int, error) {
	if cursor == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(cursor)
	if err != nil || page <= 0 || page > maxPages {
		return 0, &domain.ProviderError{Kind: "invalid", Message: "github pagination cursor is invalid"}
	}
	return page, nil
}

func boundedLimit(limit int) int {
	if limit <= 0 {
		return defaultPageSize
	}
	if limit > maxPageSize {
		return maxPageSize
	}
	return limit
}

func validOperationID(value string) error {
	if value == "" || len(value) > 200 {
		return &domain.ProviderError{Kind: "invalid", Message: "github operation ID is invalid"}
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && !strings.ContainsRune("._:-", character) {
			return &domain.ProviderError{Kind: "invalid", Message: "github operation ID is invalid"}
		}
	}
	return nil
}

func operationBody(body string, operationID string) string {
	if operationID == "" {
		return body
	}
	marker := "<!-- reforge-operation-id:" + operationID + " -->"
	if strings.Contains(body, marker) {
		return body
	}
	if body == "" {
		return marker
	}
	return body + "\n\n" + marker
}

func operationFromBody(body string) string {
	const prefix = "<!-- reforge-operation-id:"
	start := strings.Index(body, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(body[start:], " -->")
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}

func mapRepository(raw githubRepository) forge.Repository {
	permissions := make([]string, 0, len(raw.Permissions))
	for permission, allowed := range raw.Permissions {
		if allowed {
			permissions = append(permissions, permission)
		}
	}
	sort.Strings(permissions)
	return forge.Repository{RepoRef: forge.RepoRef{NativeID: strconv.FormatInt(raw.ID, 10), FullName: raw.FullName}, URL: raw.HTMLURL, CloneURL: raw.CloneURL, DefaultBranch: raw.DefaultBranch, Archived: raw.Archived, Private: raw.Private, Permissions: permissions}
}

func mapChange(raw githubPullRequest) forge.Change {
	state := raw.State
	if raw.Merged {
		state = "merged"
	}
	body := raw.Body
	operationID := operationFromBody(body)
	if operationID != "" {
		body = strings.TrimSpace(strings.TrimSuffix(body, "<!-- reforge-operation-id:"+operationID+" -->"))
	}
	return forge.Change{ID: strconv.FormatInt(raw.Number, 10), Repository: mapRepoRef(raw.Base.Repo), HeadRepository: mapRepoRef(raw.Head.Repo), TargetRepository: mapRepoRef(raw.Base.Repo), Title: raw.Title, Body: body, URL: raw.HTMLURL, HeadSHA: raw.Head.SHA, TargetSHA: raw.Base.SHA, HeadBranch: raw.Head.Ref, TargetBranch: raw.Base.Ref, AuthorID: strconv.FormatInt(raw.User.ID, 10), AuthorLogin: raw.User.Login, AuthorType: raw.User.Type, State: state, Draft: raw.Draft, MergeSHA: raw.MergeCommitSHA, MergeStatus: raw.MergeableState, OperationID: operationID}
}

func mapRepoRef(raw githubRepoRef) forge.RepoRef {
	if raw.ID <= 0 {
		return forge.RepoRef{FullName: raw.FullName}
	}
	return forge.RepoRef{NativeID: strconv.FormatInt(raw.ID, 10), FullName: raw.FullName}
}

func mapCheck(raw githubCheckRun) forge.Check {
	publisherID := ""
	if raw.App.ID != 0 {
		publisherID = strconv.FormatInt(raw.App.ID, 10)
	}
	return forge.Check{ID: strconv.FormatInt(raw.ID, 10), Name: raw.Name, PublisherID: publisherID, HeadSHA: raw.HeadSHA, Status: raw.Status, Conclusion: raw.Conclusion, URL: raw.HTMLURL}
}

type githubRepository struct {
	ID            int64           `json:"id"`
	FullName      string          `json:"full_name"`
	HTMLURL       string          `json:"html_url"`
	CloneURL      string          `json:"clone_url"`
	DefaultBranch string          `json:"default_branch"`
	Archived      bool            `json:"archived"`
	Private       bool            `json:"private"`
	Permissions   map[string]bool `json:"permissions"`
}

type githubContent struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type githubCheckRuns struct {
	CheckRuns []githubCheckRun `json:"check_runs"`
}

type githubCheckRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HeadSHA    string `json:"head_sha"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	App        struct {
		ID int64 `json:"id"`
	} `json:"app"`
}

type githubPullRequest struct {
	Number         int64         `json:"number"`
	Title          string        `json:"title"`
	Body           string        `json:"body"`
	HTMLURL        string        `json:"html_url"`
	State          string        `json:"state"`
	Draft          bool          `json:"draft"`
	Merged         bool          `json:"merged"`
	MergeCommitSHA string        `json:"merge_commit_sha"`
	MergeableState string        `json:"mergeable_state"`
	Repository     githubRepoRef `json:"repository"`
	User           struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
	Head struct {
		Ref  string        `json:"ref"`
		SHA  string        `json:"sha"`
		Repo githubRepoRef `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string        `json:"ref"`
		SHA  string        `json:"sha"`
		Repo githubRepoRef `json:"repo"`
	} `json:"base"`
}

type githubRepoRef struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type githubEvent struct {
	MergeGroup struct {
		HeadSHA string `json:"head_sha"`
	} `json:"merge_group"`
	After      string `json:"after"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest struct {
		Number int64 `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}
