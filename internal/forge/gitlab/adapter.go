package gitlab

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	webhookSkew      = 5 * time.Minute
)

type Provider struct {
	deliveryGuard   forge.DeliveryGuard
	authorizeTrain  func(context.Context, forge.TrainGateRequest) error
	inspector       *Provider
	config          forge.Config
	base            *url.URL
	authorizeBranch func(context.Context, forge.UpdateBranchRequest) error
	authorizeChange func(context.Context, forge.CreateChangeRequest) error
	authorizeReview func(context.Context, forge.RepoRef, string, []string) error
	mergeGuard      MergeGuard
	publishers      map[string]string
}

func New(config forge.Config) (*Provider, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("gitlab client is required")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("gitlab base URL is required")
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, fmt.Errorf("gitlab token is required")
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.Opaque != "" || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("gitlab base URL is invalid")
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if !strings.HasSuffix(base.Path, "/api/v4") {
		base.Path += "/api/v4"
	}
	base.RawPath = ""
	if client, ok := config.Client.(*http.Client); ok {
		clone := *client
		if clone.Timeout == 0 || clone.Timeout > 15*time.Second {
			clone.Timeout = 15 * time.Second
		}
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		config.Client = &clone
	}
	return &Provider{config: config, base: base}, nil
}

func (p *Provider) ProbeCapabilities(ctx context.Context) (forge.Capabilities, error) {
	if _, err := p.authenticatedActor(ctx); err != nil {
		return forge.Capabilities{}, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"metadata"}, nil, nil)
	if err != nil {
		return forge.Capabilities{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Capabilities{}, responseError(status, headers)
	}
	var metadata struct {
		Version string `json:"version"`
	}
	if err := decode(body, &metadata); err != nil {
		return forge.Capabilities{}, err
	}
	version := p.config.ServerVersion
	if version == "" {
		version = metadata.Version
	}
	if version == "" {
		version = "unknown"
	}
	now := time.Now().UTC()
	features := map[string]domain.Capability{}
	for _, name := range []string{"inventory.projects", "events.webhooks", "changes.read", "changes.create", "changes.review", "checks.statuses"} {
		features[name] = domain.Capability{State: domain.Supported, Scope: "gitlab-rest-v4", Reason: "implemented by this adapter", Source: "gitlab-rest-v4", Version: version, LastChecked: now}
	}
	for _, name := range []string{"changes.create", "changes.review", "changes.update_branch", "protection.rules", "merge.native", "merge.queue"} {
		features[name] = domain.Capability{State: domain.Unknown, Scope: "gitlab-rest-v4", Reason: "Requires project permission, persisted authority and native enforcement qualification", Source: "gitlab-rest-v4", Version: version, LastChecked: now}
	}
	features["delivery.workflows"] = domain.Capability{State: domain.Unsupported, Reason: "Delivery adapter not implemented", Version: version, LastChecked: now}
	return forge.Capabilities{Provider: "gitlab", ServerVersion: version, Features: features}, nil
}

func (p *Provider) ListRepositories(ctx context.Context, request forge.InventoryRequest) (domain.Page[forge.Repository], error) {
	page, err := pageNumber(request.Cursor)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	segments, query, err := inventoryPath(request.Namespace)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	query.Set("per_page", strconv.Itoa(boundedLimit(request.Limit)))
	query.Set("page", strconv.Itoa(page))
	status, headers, body, err := p.request(ctx, http.MethodGet, segments, query, nil)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	if status < 200 || status >= 300 {
		return domain.Page[forge.Repository]{}, responseError(status, headers)
	}
	var raw []gitlabProject
	if err := decode(body, &raw); err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	if raw == nil || len(raw) > maxPageSize {
		return domain.Page[forge.Repository]{}, failure("provider", "Incomplete inventory page")
	}
	items := make([]forge.Repository, 0, len(raw))
	for _, project := range raw {
		if project.ID <= 0 || project.PathWithNamespace == "" {
			return domain.Page[forge.Repository]{}, failure("identity", "Incomplete repository identity")
		}
		items = append(items, mapRepository(project))
	}
	next, err := nextPage(headers, page)
	if err != nil {
		return domain.Page[forge.Repository]{}, err
	}
	return domain.Page[forge.Repository]{Items: items, NextCursor: next, Complete: next == ""}, nil
}

func (p *Provider) GetRepository(ctx context.Context, reference forge.RepoRef) (forge.Repository, error) {
	project, err := p.projectSegment(reference)
	if err != nil {
		return forge.Repository{}, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project}, nil, nil)
	if err != nil {
		return forge.Repository{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Repository{}, responseError(status, headers)
	}
	var raw gitlabProject
	if err := decode(body, &raw); err != nil {
		return forge.Repository{}, err
	}
	if !safeID(stringID(raw.ID)) || raw.PathWithNamespace == "" || reference.NativeID != "" && reference.NativeID != stringID(raw.ID) {
		return forge.Repository{}, &domain.ProviderError{Kind: "protocol", Message: "GitLab repository identity mismatch"}
	}
	return mapRepository(raw), nil
}

func (p *Provider) ReadFileAtRef(ctx context.Context, reference forge.RepoRef, filePath string, ref string) (forge.File, error) {
	project, err := p.projectSegment(reference)
	if err != nil {
		return forge.File{}, err
	}
	if ref == "" || !validFilePath(filePath) {
		return forge.File{}, &domain.ProviderError{Kind: "invalid", Message: "gitlab file path or ref is invalid"}
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "repository", "files", filePath, "raw"}, url.Values{"ref": {ref}}, nil)
	if err != nil {
		return forge.File{}, err
	}
	if status < 200 || status >= 300 {
		return forge.File{}, responseError(status, headers)
	}
	return forge.File{Path: filePath, Content: body}, nil
}

func (p *Provider) ResolveRef(ctx context.Context, reference forge.RepoRef, ref string) (string, error) {
	project, err := p.projectSegment(reference)
	if err != nil {
		return "", err
	}
	kind, name, err := refParts(ref)
	if err != nil {
		return "", err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "repository", kind, name}, nil, nil)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", responseError(status, headers)
	}
	var raw struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := decode(body, &raw); err != nil {
		return "", err
	}
	if raw.Commit.ID == "" {
		return "", &domain.ProviderError{Kind: "provider", Message: "gitlab ref has no commit SHA"}
	}
	return raw.Commit.ID, nil
}

func (p *Provider) VerifyWebhook(headers http.Header, payload []byte) error {
	if p.config.WebhookSecret == "" {
		return &domain.ProviderError{Kind: "auth", Message: "gitlab webhook secret is not configured"}
	}
	signature := headerValue(headers, "webhook-signature")
	if signature != "" || headerValue(headers, "webhook-id") != "" || headerValue(headers, "webhook-timestamp") != "" {
		return p.verifySignedWebhook(headers, payload)
	}
	provided := []byte(headerValue(headers, "X-Gitlab-Token"))
	if len(provided) == 0 || !hmac.Equal(provided, []byte(p.config.WebhookSecret)) {
		return &domain.ProviderError{Kind: "auth", Message: "gitlab webhook token is invalid"}
	}
	return nil
}

func (p *Provider) verifySignedWebhook(headers http.Header, payload []byte) error {
	id := headerValue(headers, "webhook-id")
	timestamp := headerValue(headers, "webhook-timestamp")
	signature := headerValue(headers, "webhook-signature")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if id == "" || signature == "" || err != nil || absDuration(time.Since(time.Unix(seconds, 0))) > webhookSkew {
		return &domain.ProviderError{Kind: "auth", Message: "gitlab webhook signature is invalid"}
	}
	key := strings.TrimPrefix(p.config.WebhookSecret, "whsec_")
	decodedKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decodedKey) == 0 {
		return &domain.ProviderError{Kind: "auth", Message: "gitlab webhook signing secret is invalid"}
	}
	digest := hmac.New(sha256.New, decodedKey)
	_, _ = digest.Write([]byte(id + "." + timestamp + "."))
	_, _ = digest.Write(payload)
	expected := digest.Sum(nil)
	for _, candidate := range strings.Fields(signature) {
		if !strings.HasPrefix(candidate, "v1,") {
			continue
		}
		provided, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(candidate, "v1,"))
		if decodeErr == nil && hmac.Equal(provided, expected) {
			return nil
		}
	}
	return &domain.ProviderError{Kind: "auth", Message: "gitlab webhook signature is invalid"}
}

func (p *Provider) DecodeEvent(headers http.Header, payload []byte) (forge.Event, error) {
	if len(payload) > maxResponseBytes {
		return forge.Event{}, &domain.ProviderError{Kind: "provider", Message: "gitlab webhook payload is too large"}
	}
	if err := p.VerifyWebhook(headers, payload); err != nil {
		return forge.Event{}, err
	}
	var raw gitlabWebhook
	if err := json.Unmarshal(payload, &raw); err != nil {
		return forge.Event{}, &domain.ProviderError{Kind: "provider", Message: "gitlab webhook payload is invalid"}
	}
	kind := strings.ToLower(raw.ObjectKind)
	if kind == "" {
		kind = strings.ToLower(strings.TrimSuffix(headerValue(headers, "X-Gitlab-Event"), " Hook"))
	}
	event := forge.Event{DeliveryID: deliveryID(headers), Kind: kind, Repository: forge.RepoRef{NativeID: stringID(raw.Project.ID), FullName: raw.Project.PathWithNamespace}}
	if raw.ObjectAttributes.IID != 0 {
		event.ChangeID = strconv.FormatInt(raw.ObjectAttributes.IID, 10)
		event.HeadSHA = raw.ObjectAttributes.LastCommit.ID
	}
	if event.Kind == "push" {
		event.HeadSHA = raw.After
	}
	return event, nil
}

func (p *Provider) ReconcileChanges(ctx context.Context, reference forge.RepoRef, cursor string) (domain.Page[forge.Change], error) {
	return p.listChanges(ctx, reference, cursor, "all")
}

func (p *Provider) ListChecks(ctx context.Context, reference forge.RepoRef, ref string) ([]forge.Check, error) {
	if !validBranch(ref) {
		return nil, failure("invalid", "Invalid status revision")
	}
	project, err := p.projectSegment(reference)
	if err != nil {
		return nil, err
	}
	checks := make([]forge.Check, 0)
	page := 1
	for requestCount := 0; requestCount < maxPages; requestCount++ {
		query := url.Values{"all": {"true"}, "per_page": {strconv.Itoa(maxPageSize)}, "page": {strconv.Itoa(page)}}
		status, headers, body, requestErr := p.request(ctx, http.MethodGet, []string{"projects", project, "repository", "commits", ref, "statuses"}, query, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		if status < 200 || status >= 300 {
			return nil, responseError(status, headers)
		}
		var raw []gitlabStatus
		if err := decode(body, &raw); err != nil {
			return nil, err
		}
		if raw == nil || len(raw) > maxPageSize {
			return nil, failure("provider", "Incomplete status page")
		}
		for _, status := range raw {
			if status.ID <= 0 || status.Author.ID <= 0 || status.Name == "" || status.SHA == "" {
				return nil, failure("identity", "Incomplete status publisher identity")
			}
			checks = append(checks, forge.Check{ID: stringID(status.ID), Name: status.Name, PublisherID: stringID(status.Author.ID), HeadSHA: status.SHA, Status: status.Status, URL: status.TargetURL})
		}
		next, err := nextPage(headers, page)
		if err != nil || next == "" {
			if err != nil {
				return nil, err
			}
			return checks, nil
		}
		page, err = strconv.Atoi(next)
		if err != nil || page <= 0 || page > maxPages {
			return nil, &domain.ProviderError{Kind: "provider", Message: "gitlab pagination cursor is invalid"}
		}
	}
	return nil, &domain.ProviderError{Kind: "provider", Message: "gitlab pagination exceeded the safety bound"}
}

func (p *Provider) ListBotWork(ctx context.Context, reference forge.RepoRef) ([]forge.Change, error) {
	changes := make([]forge.Change, 0)
	cursor := ""
	for page := 0; page < maxPages; page++ {
		result, err := p.listChanges(ctx, reference, cursor, "opened")
		if err != nil {
			return nil, err
		}
		for _, change := range result.Items {
			if change.AuthorType == "Bot" {
				changes = append(changes, change)
			}
		}
		if result.NextCursor == "" {
			return changes, nil
		}
		cursor = result.NextCursor
	}
	return nil, &domain.ProviderError{Kind: "provider", Message: "gitlab pagination exceeded the safety bound"}
}

func (p *Provider) CreateChange(ctx context.Context, request forge.CreateChangeRequest) (forge.Change, error) {
	if p.authorizeChange == nil {
		return forge.Change{}, failure("unsupported", "Persisted MR publication authorization required")
	}
	if !safeID(request.Repository.NativeID) || request.Title == "" || request.HeadBranch == "" || request.TargetBranch == "" || !validSHA(request.ExpectedHeadSHA) {
		return forge.Change{}, &domain.ProviderError{Kind: "invalid", Message: "gitlab change request is incomplete"}
	}
	if !validBranch(request.HeadBranch) || !validBranch(request.TargetBranch) {
		return forge.Change{}, &domain.ProviderError{Kind: "unsupported", Message: "gitlab fork change creation requires an explicit source project"}
	}
	if err := validOperationID(request.OperationID); err != nil {
		return forge.Change{}, err
	}
	actor, err := p.authenticatedActor(ctx)
	if err != nil {
		return forge.Change{}, err
	}
	if err = p.authorizeChange(ctx, request); err != nil {
		return forge.Change{}, err
	}
	existing, err := p.FindChangeByOperation(ctx, request.Repository, request.OperationID, request.HeadBranch, request.TargetBranch)
	if err != nil {
		return forge.Change{}, err
	}
	if existing != nil {
		if existing.HeadSHA != request.ExpectedHeadSHA {
			return forge.Change{}, failure("conflict", "Operation source head changed")
		}
		return *existing, nil
	}
	observed, err := p.ResolveRef(ctx, request.Repository, "heads/"+request.HeadBranch)
	if err != nil {
		return forge.Change{}, err
	}
	if observed != request.ExpectedHeadSHA {
		return forge.Change{}, &domain.ProviderError{Kind: "conflict", Message: "gitlab source head changed before merge request creation"}
	}
	project, err := p.projectSegment(request.Repository)
	if err != nil {
		return forge.Change{}, err
	}
	body := struct {
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		Title        string `json:"title"`
		Description  string `json:"description"`
	}{request.HeadBranch, request.TargetBranch, request.Title, operationBody(request.Body, request.OperationID)}
	encoded, err := json.Marshal(body)
	if err != nil {
		return forge.Change{}, &domain.ProviderError{Kind: "provider", Message: "gitlab request encoding failed"}
	}
	if request.Draft && !strings.HasPrefix(strings.ToLower(body.Title), "draft:") {
		body.Title = "Draft: " + body.Title
		encoded, _ = json.Marshal(body)
	}
	if err = p.authorizeChange(ctx, request); err != nil {
		return forge.Change{}, err
	}
	status, headers, responseBody, err := p.request(ctx, http.MethodPost, []string{"projects", project, "merge_requests"}, nil, &requestBody{data: encoded})
	if err != nil {
		return forge.Change{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Change{}, responseError(status, headers)
	}
	var raw gitlabMergeRequest
	if err := decode(responseBody, &raw); err != nil {
		return forge.Change{}, uncertain(err)
	}
	change, err := p.mapAndRefreshTarget(ctx, raw, request.Repository)
	if err != nil {
		return forge.Change{}, uncertain(err)
	}
	if !VerifyOperationOwner(change, request.Repository, request.OperationID, request.HeadBranch, request.TargetBranch, actor) || change.HeadSHA != request.ExpectedHeadSHA || change.HeadBranch != request.HeadBranch || change.TargetBranch != request.TargetBranch || !sameRepo(change.HeadRepository, request.Repository) {
		return forge.Change{}, &domain.ProviderError{Kind: "conflict", Message: "GitLab merge request changed during creation", Uncertain: true}
	}
	return change, nil
}

func (p *Provider) FindChangeByOperation(ctx context.Context, reference forge.RepoRef, operationID string, headBranch string, targetBranch string) (*forge.Change, error) {
	if err := validOperationID(operationID); err != nil {
		return nil, err
	}
	actor, err := p.authenticatedActor(ctx)
	if err != nil {
		return nil, err
	}
	cursor := ""
	var found *forge.Change
	for page := 0; page < maxPages; page++ {
		result, err := p.listChanges(ctx, reference, cursor, "all")
		if err != nil {
			return nil, err
		}
		for _, change := range result.Items {
			if VerifyOperationOwner(change, reference, operationID, headBranch, targetBranch, actor) {
				if found != nil && found.ID != change.ID {
					return nil, failure("conflict", "Duplicate operation merge requests")
				}
				found = &change
			}
		}
		if result.NextCursor == "" {
			return found, nil
		}
		cursor = result.NextCursor
	}
	return nil, &domain.ProviderError{Kind: "provider", Message: "gitlab pagination exceeded the safety bound"}
}

func (p *Provider) ReadChange(ctx context.Context, reference forge.RepoRef, changeID string) (forge.Change, error) {
	if !safeID(changeID) {
		return forge.Change{}, &domain.ProviderError{Kind: "invalid", Message: "gitlab merge request IID is invalid"}
	}
	project, err := p.projectSegment(reference)
	if err != nil {
		return forge.Change{}, err
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "merge_requests", changeID}, nil, nil)
	if err != nil {
		return forge.Change{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Change{}, responseError(status, headers)
	}
	var raw gitlabMergeRequest
	if err := decode(body, &raw); err != nil {
		return forge.Change{}, err
	}
	if stringID(raw.IID) != changeID {
		return forge.Change{}, failure("identity", "Merge request identity changed")
	}
	return p.mapAndRefreshTarget(ctx, raw, reference)
}

func (p *Provider) RequestReview(ctx context.Context, reference forge.RepoRef, changeID string, reviewers []string) error {
	if p.authorizeReview == nil {
		return failure("unsupported", "Persisted reviewer authorization required")
	}
	if !safeID(reference.NativeID) || !safeID(changeID) || len(reviewers) > 100 {
		return failure("invalid", "Immutable project and bounded reviewers required")
	}
	if _, err := strconv.ParseInt(changeID, 10, 64); err != nil || len(reviewers) == 0 {
		return &domain.ProviderError{Kind: "invalid", Message: "gitlab review request is invalid"}
	}
	ids := make([]int64, 0, len(reviewers))
	for _, reviewer := range reviewers {
		id, err := strconv.ParseInt(reviewer, 10, 64)
		if err != nil || id <= 0 {
			return &domain.ProviderError{Kind: "invalid", Message: "gitlab reviewer ID is invalid"}
		}
		ids = append(ids, id)
	}
	project, err := p.projectSegment(reference)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(struct {
		ReviewerIDs []int64 `json:"reviewer_ids"`
	}{ids})
	if err != nil {
		return &domain.ProviderError{Kind: "provider", Message: "gitlab request encoding failed"}
	}
	if _, err = p.authenticatedActor(ctx); err != nil {
		return err
	}
	if err = p.authorizeReview(ctx, reference, changeID, append([]string(nil), reviewers...)); err != nil {
		return err
	}
	status, headers, _, err := p.request(ctx, http.MethodPut, []string{"projects", project, "merge_requests", changeID}, nil, &requestBody{data: encoded})
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return responseError(status, headers)
	}
	return nil
}

func (p *Provider) listChanges(ctx context.Context, reference forge.RepoRef, cursor string, state string) (domain.Page[forge.Change], error) {
	project, err := p.projectSegment(reference)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	page, err := pageNumber(cursor)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	query := url.Values{"state": {state}, "per_page": {strconv.Itoa(maxPageSize)}, "page": {strconv.Itoa(page)}}
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "merge_requests"}, query, nil)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	if status < 200 || status >= 300 {
		return domain.Page[forge.Change]{}, responseError(status, headers)
	}
	var raw []gitlabMergeRequest
	if err := decode(body, &raw); err != nil {
		return domain.Page[forge.Change]{}, err
	}
	if raw == nil || len(raw) > maxPageSize {
		return domain.Page[forge.Change]{}, failure("provider", "Incomplete MR page")
	}
	items := make([]forge.Change, 0, len(raw))
	for _, mergeRequest := range raw {
		change, err := p.mapAndRefreshTarget(ctx, mergeRequest, reference)
		if err != nil {
			return domain.Page[forge.Change]{}, err
		}
		items = append(items, change)
	}
	next, err := nextPage(headers, page)
	if err != nil {
		return domain.Page[forge.Change]{}, err
	}
	return domain.Page[forge.Change]{Items: items, NextCursor: next, Complete: next == ""}, nil
}

func (p *Provider) mapAndRefreshTarget(ctx context.Context, raw gitlabMergeRequest, fallback forge.RepoRef) (forge.Change, error) {
	change := mapChange(raw, fallback)
	if !safeID(change.ID) || !safeID(change.TargetRepository.NativeID) || !validBranch(change.TargetBranch) || fallback.NativeID != "" && fallback.NativeID != change.TargetRepository.NativeID {
		return forge.Change{}, &domain.ProviderError{Kind: "provider", Message: "gitlab merge request has incomplete target identity"}
	}
	project := change.TargetRepository.NativeID
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"projects", project, "repository", "branches", change.TargetBranch}, nil, nil)
	if err != nil {
		return forge.Change{}, err
	}
	if status < 200 || status >= 300 {
		return forge.Change{}, responseError(status, headers)
	}
	var branch struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := decode(body, &branch); err != nil {
		return forge.Change{}, err
	}
	if branch.Commit.ID == "" {
		return forge.Change{}, &domain.ProviderError{Kind: "provider", Message: "gitlab target branch has no current SHA"}
	}
	change.TargetSHA = branch.Commit.ID
	return change, nil
}

type requestBody struct {
	data    []byte
	headers http.Header
}

func (p *Provider) request(ctx context.Context, method string, segments []string, query url.Values, body *requestBody) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	mutation := method != http.MethodGet && method != http.MethodHead
	if body != nil && len(body.data) > maxResponseBytes {
		return 0, nil, nil, &domain.ProviderError{Kind: "invalid", Message: "GitLab request exceeds limit"}
	}
	target := p.baseURL(segments)
	if query != nil {
		target.RawQuery = query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body.data))
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return 0, nil, nil, &domain.ProviderError{Kind: "invalid", Message: "gitlab request URL is invalid"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("PRIVATE-TOKEN", p.config.Token)
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
		return 0, nil, nil, &domain.ProviderError{Kind: "transient", Message: "gitlab request failed", Uncertain: method != http.MethodGet && method != http.MethodHead}
	}
	if response == nil || response.Body == nil {
		return 0, nil, nil, &domain.ProviderError{Kind: "protocol", Message: "GitLab response missing", Uncertain: mutation}
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	responseBody, readErr := io.ReadAll(limited)
	if readErr != nil {
		return response.StatusCode, response.Header, nil, &domain.ProviderError{Kind: "transient", Message: "gitlab response could not be read", Uncertain: mutation}
	}
	if len(responseBody) > maxResponseBytes {
		return response.StatusCode, response.Header, nil, &domain.ProviderError{Kind: "provider", Message: "gitlab response exceeded the safety limit", Uncertain: mutation}
	}
	if mutation && (response.StatusCode >= 500 || response.StatusCode == 408) {
		return response.StatusCode, response.Header, nil, &domain.ProviderError{Kind: "provider", Message: "GitLab mutation outcome unknown", Uncertain: true}
	}
	return response.StatusCode, response.Header, responseBody, nil
}

func (p *Provider) baseURL(segments []string) *url.URL {
	result := *p.base
	plain := strings.TrimRight(p.base.Path, "/")
	escaped := plain
	for _, segment := range segments {
		plain += "/" + segment
		escaped += "/" + url.PathEscape(segment)
	}
	result.Path = plain
	result.RawPath = escaped
	return &result
}

func (p *Provider) projectSegment(reference forge.RepoRef) (string, error) {
	if reference.NativeID != "" {
		if !safeID(reference.NativeID) {
			return "", &domain.ProviderError{Kind: "invalid", Message: "gitlab project ID is invalid"}
		}
		return reference.NativeID, nil
	}
	if !validFilePath(reference.FullName) {
		return "", &domain.ProviderError{Kind: "invalid", Message: "gitlab project reference is incomplete"}
	}
	return reference.FullName, nil
}

func inventoryPath(namespace string) ([]string, url.Values, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || namespace == "projects" {
		return []string{"projects"}, url.Values{}, nil
	}
	if strings.HasPrefix(namespace, "group:") {
		namespace = strings.TrimPrefix(namespace, "group:")
	} else if strings.HasPrefix(namespace, "group/") {
		namespace = strings.TrimPrefix(namespace, "group/")
	} else {
		return nil, nil, &domain.ProviderError{Kind: "invalid", Message: "gitlab inventory namespace is invalid"}
	}
	if !validFilePath(namespace) {
		return nil, nil, &domain.ProviderError{Kind: "invalid", Message: "gitlab group namespace is invalid"}
	}
	return []string{"groups", namespace, "projects"}, url.Values{"include_subgroups": {"true"}}, nil
}

func nextPage(headers http.Header, current int) (string, error) {
	value := headers.Get("X-Next-Page")
	if value == "" {
		return "", nil
	}
	page, err := strconv.Atoi(value)
	if err != nil || page != current+1 || page > maxPages {
		return "", &domain.ProviderError{Kind: "provider", Message: "gitlab pagination cursor is invalid"}
	}
	return value, nil
}

func pageNumber(cursor string) (int, error) {
	if cursor == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(cursor)
	if err != nil || page <= 0 || page > maxPages {
		return 0, &domain.ProviderError{Kind: "invalid", Message: "gitlab pagination cursor is invalid"}
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

func refParts(ref string) (string, string, error) {
	ref = strings.TrimPrefix(ref, "refs/")
	if ref == "" {
		return "", "", &domain.ProviderError{Kind: "invalid", Message: "gitlab ref is empty"}
	}
	parts := strings.SplitN(ref, "/", 2)
	kind, name := "branches", ref
	if len(parts) == 2 {
		switch parts[0] {
		case "heads":
			kind, name = "branches", parts[1]
		case "tags":
			kind, name = "tags", parts[1]
		}
	}
	if !validBranch(name) {
		return "", "", &domain.ProviderError{Kind: "invalid", Message: "gitlab ref is invalid"}
	}
	return kind, name, nil
}

func safeID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0
}

func validOperationID(value string) error {
	if value == "" || len(value) > 200 {
		return &domain.ProviderError{Kind: "invalid", Message: "gitlab operation ID is invalid"}
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && !strings.ContainsRune("._:-", character) {
			return &domain.ProviderError{Kind: "invalid", Message: "gitlab operation ID is invalid"}
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

func verifyOperation(change forge.Change, reference forge.RepoRef, operationID string, headBranch string, targetBranch string) bool {
	return change.OperationID == operationID && sameRepo(change.Repository, reference) && sameRepo(change.HeadRepository, reference) && sameRepo(change.TargetRepository, reference) && change.HeadBranch == headBranch && change.TargetBranch == targetBranch
}

func VerifyOperationOwner(change forge.Change, reference forge.RepoRef, operationID string, headBranch string, targetBranch string, actorID string) bool {
	return actorID != "" && change.AuthorID == actorID && verifyOperation(change, reference, operationID, headBranch, targetBranch)
}

func sameRepo(actual forge.RepoRef, expected forge.RepoRef) bool {
	return expected.NativeID != "" && actual.NativeID == expected.NativeID && (expected.FullName == "" || actual.FullName == expected.FullName)
}

func mapRepository(raw gitlabProject) forge.Repository {
	permissions := make([]string, 0, 2)
	if raw.Permissions.ProjectAccess.AccessLevel > 0 {
		permissions = append(permissions, "project")
	}
	if raw.Permissions.GroupAccess.AccessLevel > 0 {
		permissions = append(permissions, "group")
	}
	return forge.Repository{RepoRef: forge.RepoRef{NativeID: stringID(raw.ID), FullName: raw.PathWithNamespace}, URL: raw.WebURL, CloneURL: raw.HTTPURLToRepo, DefaultBranch: raw.DefaultBranch, Archived: raw.Archived, Private: raw.Visibility == "private", Permissions: permissions}
}

func mapChange(raw gitlabMergeRequest, fallback forge.RepoRef) forge.Change {
	targetID := raw.TargetProjectID
	if targetID == 0 {
		targetID = raw.ProjectID
	}
	sourceID := raw.SourceProjectID
	headSHA := raw.SHA
	if headSHA == "" {
		headSHA = raw.DiffRefs.HeadSHA
	}
	body := raw.Description
	operationID := operationFromBody(body)
	if operationID != "" {
		body = strings.TrimSpace(strings.TrimSuffix(body, "<!-- reforge-operation-id:"+operationID+" -->"))
	}
	authorType := "User"
	if raw.Author.Bot {
		authorType = "Bot"
	}
	target := forge.RepoRef{NativeID: stringID(targetID), FullName: fallback.FullName}
	head := forge.RepoRef{NativeID: stringID(sourceID)}
	if sourceID == targetID {
		head.FullName = fallback.FullName
	}
	return forge.Change{ID: strconv.FormatInt(raw.IID, 10), Repository: target, HeadRepository: head, TargetRepository: target, Title: raw.Title, Body: body, URL: raw.WebURL, HeadSHA: headSHA, TargetSHA: raw.DiffRefs.StartSHA, HeadBranch: raw.SourceBranch, TargetBranch: raw.TargetBranch, AuthorID: stringID(raw.Author.ID), AuthorLogin: raw.Author.Username, AuthorType: authorType, State: raw.State, Draft: raw.Draft || raw.WorkInProgress, MergeSHA: raw.MergeCommitSHA, MergeStatus: raw.DetailedMergeStatus, OperationID: operationID}
}

func stringID(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func responseError(status int, headers http.Header) error {
	kind := "provider"
	switch {
	case status == http.StatusUnauthorized:
		kind = "auth"
	case status == http.StatusTooManyRequests || headers.Get("RateLimit-Remaining") == "0":
		kind = "rate_limit"
	case status == http.StatusForbidden:
		kind = "scope"
	case status == http.StatusConflict:
		kind = "conflict"
	case status == http.StatusNotFound:
		kind = "not_found"
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		kind = "invalid"
	case status >= 500:
		kind = "transient"
	}
	return &domain.ProviderError{Kind: kind, Message: fmt.Sprintf("gitlab request returned HTTP %d", status), RetryAfter: retryAfter(headers)}
}

func retryAfter(headers http.Header) time.Duration {
	if seconds, err := strconv.Atoi(headers.Get("Retry-After")); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if reset, err := strconv.ParseInt(headers.Get("RateLimit-Reset"), 10, 64); err == nil {
		if delay := time.Until(time.Unix(reset, 0)); delay > 0 {
			return delay
		}
	}
	return 0
}

func decode(body []byte, target any) error {
	if err := json.Unmarshal(body, target); err != nil {
		return &domain.ProviderError{Kind: "provider", Message: "gitlab response was invalid JSON"}
	}
	return nil
}

func headerValue(headers http.Header, key string) string {
	if value := headers.Get(key); value != "" {
		return value
	}
	for name, values := range headers {
		if strings.EqualFold(name, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func deliveryID(headers http.Header) string {
	for _, key := range []string{"X-Gitlab-Event-UUID", "webhook-id", "Idempotency-Key"} {
		if value := headerValue(headers, key); value != "" {
			return value
		}
	}
	return ""
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

type gitlabProject struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	DefaultBranch     string `json:"default_branch"`
	Archived          bool   `json:"archived"`
	Visibility        string `json:"visibility"`
	Permissions       struct {
		ProjectAccess struct {
			AccessLevel int `json:"access_level"`
		} `json:"project_access"`
		GroupAccess struct {
			AccessLevel int `json:"access_level"`
		} `json:"group_access"`
	} `json:"permissions"`
}

type gitlabMergeRequest struct {
	HasConflicts        *bool           `json:"has_conflicts"`
	DiscussionsResolved *bool           `json:"blocking_discussions_resolved"`
	HeadPipeline        *pipelineRecord `json:"head_pipeline"`
	ID                  int64           `json:"id"`
	IID                 int64           `json:"iid"`
	ProjectID           int64           `json:"project_id"`
	Title               string          `json:"title"`
	Description         string          `json:"description"`
	WebURL              string          `json:"web_url"`
	State               string          `json:"state"`
	SourceBranch        string          `json:"source_branch"`
	TargetBranch        string          `json:"target_branch"`
	SourceProjectID     int64           `json:"source_project_id"`
	TargetProjectID     int64           `json:"target_project_id"`
	SHA                 string          `json:"sha"`
	Draft               bool            `json:"draft"`
	WorkInProgress      bool            `json:"work_in_progress"`
	MergeCommitSHA      string          `json:"merge_commit_sha"`
	DetailedMergeStatus string          `json:"detailed_merge_status"`
	Author              struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"author"`
	DiffRefs struct {
		HeadSHA  string `json:"head_sha"`
		StartSHA string `json:"start_sha"`
	} `json:"diff_refs"`
}

type gitlabStatus struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	TargetURL string `json:"target_url"`
	Author    struct {
		ID int64 `json:"id"`
	} `json:"author"`
}

type gitlabWebhook struct {
	ObjectKind string `json:"object_kind"`
	After      string `json:"after"`
	Project    struct {
		ID                int64  `json:"id"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	ObjectAttributes struct {
		IID        int64 `json:"iid"`
		LastCommit struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
}

var _ forge.ForgeInventory = (*Provider)(nil)
var _ forge.ForgeEvents = (*Provider)(nil)
var _ forge.ForgeChanges = (*Provider)(nil)

func (p *Provider) authenticatedActor(ctx context.Context) (string, error) {
	status, headers, body, err := p.request(ctx, http.MethodGet, []string{"user"}, nil, nil)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", responseError(status, headers)
	}
	var value struct {
		ID int64 `json:"id"`
	}
	if decode(body, &value) != nil || value.ID < 1 {
		return "", &domain.ProviderError{Kind: "protocol", Message: "GitLab authenticated actor missing"}
	}
	return stringID(value.ID), nil
}
func uncertain(err error) error {
	var provider *domain.ProviderError
	if errors.As(err, &provider) {
		copyError := *provider
		copyError.Uncertain = true
		return &copyError
	}
	return &domain.ProviderError{Kind: "provider", Message: "GitLab mutation outcome unknown", Uncertain: true}
}
