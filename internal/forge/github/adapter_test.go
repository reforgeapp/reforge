package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

func TestNewRequiresExplicitTransportAndConnection(t *testing.T) {
	if _, err := New(forge.Config{BaseURL: "https://api.github.com", Token: "token"}); err == nil {
		t.Fatal("expected client requirement")
	}
	if _, err := New(forge.Config{Client: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF }), Token: "token"}); err == nil {
		t.Fatal("expected base URL requirement")
	}
	if _, err := New(forge.Config{BaseURL: "https://api.github.com", Client: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF })}); err == nil {
		t.Fatal("expected token requirement")
	}
	if _, err := New(forge.Config{BaseURL: "https://token@api.github.com", Token: "token", Client: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF })}); err == nil {
		t.Fatal("expected credential-bearing base URL rejection")
	}
}

func TestListRepositoriesUsesScopedPaginationAndRejectsExternalLink(t *testing.T) {
	var requests []string
	var mu sync.Mutex
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v3/orgs/acme/repos" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer installation-token" {
			t.Errorf("authorization header leaked or missing")
		}
		if request.Header.Get("Accept") != "application/vnd.github+json" || request.Header.Get("X-GitHub-Api-Version") != apiVersion {
			t.Errorf("github headers missing")
		}
		mu.Lock()
		requests = append(requests, request.URL.RawQuery)
		mu.Unlock()
		if request.URL.Query().Get("page") == "1" {
			writer.Header().Set("Link", "<"+server.URL+"/api/v3/orgs/acme/repos?page=2&per_page=2>; rel=\"next\"")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`[{"id":1,"full_name":"acme/one","html_url":"https://github.com/acme/one","clone_url":"https://github.com/acme/one.git","default_branch":"main","permissions":{"pull":true,"push":false}}]`))
			return
		}
		writer.Header().Set("Link", "<https://evil.example/api/v3/orgs/acme/repos?page=3>; rel=\"next\"")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`[{"id":2,"full_name":"acme/two","default_branch":"trunk"}]`))
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL + "/api/v3", Token: "installation-token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.ListRepositories(context.Background(), forge.InventoryRequest{Namespace: "org:acme", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.NextCursor != "2" || first.Complete {
		t.Fatalf("first page = %#v", first)
	}
	second, err := provider.ListRepositories(context.Background(), forge.InventoryRequest{Namespace: "org:acme", Cursor: first.NextCursor, Limit: 2})
	if second.Items != nil || err == nil {
		t.Fatalf("expected external pagination rejection, page=%#v err=%v", second, err)
	}
	if providerErr, ok := err.(*domain.ProviderError); !ok || providerErr.Kind != "provider" {
		t.Fatalf("error = %#v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || !strings.Contains(requests[0], "page=1") || !strings.Contains(requests[1], "page=2") {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestProviderErrorsClassifyRevokedAndRateLimitedResponses(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		status    int
		header    string
		wantKind  string
		wantRetry bool
	}{
		{name: "revoked installation", status: http.StatusForbidden, wantKind: "scope"},
		{name: "rate limit", status: http.StatusTooManyRequests, header: "9", wantKind: "rate_limit", wantRetry: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if testCase.header != "" {
					writer.Header().Set("Retry-After", testCase.header)
				}
				writer.WriteHeader(testCase.status)
				_, _ = writer.Write([]byte(`{"message":"installation-token-secret-must-not-escape"}`))
			}))
			defer server.Close()
			provider, err := New(forge.Config{BaseURL: server.URL, Token: "installation-token-secret", Client: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.GetRepository(context.Background(), forge.RepoRef{FullName: "acme/repo"})
			if err == nil {
				t.Fatal("expected provider error")
			}
			providerErr, ok := err.(*domain.ProviderError)
			if !ok || providerErr.Kind != testCase.wantKind || strings.Contains(err.Error(), "installation-token-secret") || strings.Contains(err.Error(), "must-not-escape") {
				t.Fatalf("error = %#v", err)
			}
			if testCase.wantRetry && providerErr.RetryAfter <= 0 {
				t.Fatalf("retry-after = %s", providerErr.RetryAfter)
			}
		})
	}
}

func TestWebhookVerificationAndEventDecode(t *testing.T) {
	provider, err := New(forge.Config{BaseURL: "https://api.github.com", Token: "token", WebhookSecret: "webhook-secret", Client: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF })})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"action":"synchronize","repository":{"id":41,"full_name":"acme/repo"},"pull_request":{"number":7,"head":{"sha":"head-7"}}}`)
	digest := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = digest.Write(payload)
	header := make(http.Header)
	header.Set("X-GitHub-Delivery", "delivery-7")
	header.Set("X-GitHub-Event", "pull_request")
	header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(digest.Sum(nil)))
	event, err := provider.DecodeEvent(header, payload)
	if err != nil || event.DeliveryID != "delivery-7" || event.ChangeID != "7" || event.Repository.NativeID != "41" || event.HeadSHA != "head-7" {
		t.Fatalf("event=%#v err=%v", event, err)
	}
	header.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64))
	if err := provider.VerifyWebhook(header, payload); err == nil {
		t.Fatal("expected tampered webhook rejection")
	}
}

func TestListChecksPreservesPublisherAndHeadSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/acme/repo/commits/head-sha/check-runs" {
			t.Errorf("path = %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"check_runs":[{"id":1,"name":"build","head_sha":"head-sha","status":"completed","conclusion":"success","html_url":"https://github.com/acme/repo/runs/1","app":{"id":11}},{"id":2,"name":"build","head_sha":"head-sha","status":"completed","conclusion":"success","app":{"id":99}}]}`))
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	checks, err := provider.ListChecks(context.Background(), forge.RepoRef{FullName: "acme/repo"}, "head-sha")
	if err != nil || len(checks) != 2 || checks[0].PublisherID != "11" || checks[1].PublisherID != "99" || checks[0].HeadSHA != "head-sha" {
		t.Fatalf("checks=%#v err=%v", checks, err)
	}
}

func TestCreateChangePreflightsExactHeadAndFindsOperation(t *testing.T) {
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/repo/pulls":
			if request.URL.Query().Get("head") != "acme:feature" || request.URL.Query().Get("base") != "main" {
				t.Errorf("pull filters = %s", request.URL.RawQuery)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[]`))
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/repo/git/ref/heads/feature":
			_, _ = writer.Write([]byte(`{"object":{"sha":"1111111111111111111111111111111111111111"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/repo/git/ref/heads/main":
			_, _ = writer.Write([]byte(`{"object":{"sha":"fresh-target"}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/repo/pulls":
			postCount++
			if request.Header.Get("Idempotency-Key") != "" {
				t.Error("undocumented provider idempotency guarantee sent")
			}
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), "reforge-operation-id:op-1") {
				t.Errorf("operation marker missing: %s", body)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"number":7,"title":"Repair","body":"body\n\n<!-- reforge-operation-id:op-1 -->","html_url":"https://github.com/acme/repo/pull/7","state":"open","repository":{"id":1,"full_name":"acme/repo"},"user":{"id":2,"login":"reforge[bot]","type":"Bot"},"head":{"ref":"feature","sha":"1111111111111111111111111111111111111111","repo":{"id":1,"full_name":"acme/repo"}},"base":{"ref":"main","sha":"base-1","repo":{"id":1,"full_name":"acme/repo"}}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider = authorizedFixture(provider)
	change, err := provider.CreateChange(context.Background(), forge.CreateChangeRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, Title: "Repair", Body: "body", HeadBranch: "feature", TargetBranch: "main", ExpectedHeadSHA: "1111111111111111111111111111111111111111", OperationID: "op-1"})
	if err != nil || change.ID != "7" || change.OperationID != "op-1" || change.Body != "body" || change.Repository.NativeID != "1" || change.HeadRepository.NativeID != "1" || change.TargetRepository.NativeID != "1" || postCount != 1 {
		t.Fatalf("change=%#v err=%v posts=%d", change, err, postCount)
	}
	if !VerifyOperationOwner(change, forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, "op-1", "feature", "main", "2") || VerifyOperationOwner(change, forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, "op-1", "feature", "main", "3") {
		t.Fatalf("operation owner verification failed: %#v", change)
	}
}

func TestListBotWorkRetainsBotActorIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/acme/repo/pulls" {
			t.Errorf("path = %s", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`[{"number":7,"state":"open","repository":{"id":1,"full_name":"acme/repo"},"user":{"id":42,"login":"dependabot[bot]","type":"Bot"},"head":{"ref":"dependabot/npm","sha":"bot-head","repo":{"id":1,"full_name":"acme/repo"}},"base":{"ref":"main","sha":"base","repo":{"id":1,"full_name":"acme/repo"}}},{"number":8,"state":"open","repository":{"id":1,"full_name":"acme/repo"},"user":{"id":43,"login":"alice","type":"User"},"head":{"ref":"feature","sha":"human-head","repo":{"id":1,"full_name":"acme/repo"}},"base":{"ref":"main","sha":"base","repo":{"id":1,"full_name":"acme/repo"}}}]`))
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := provider.ListBotWork(context.Background(), forge.RepoRef{FullName: "acme/repo"})
	if err != nil || len(changes) != 1 || changes[0].AuthorID != "42" || changes[0].AuthorLogin != "dependabot[bot]" {
		t.Fatalf("changes=%#v err=%v", changes, err)
	}
}

func TestReadChangeMapsForkRepositoriesAndRejectsOwnerAdoption(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/repos/acme/repo/git/ref/heads/main" {
			_, _ = writer.Write([]byte(`{"object":{"sha":"fresh-target"}}`))
			return
		}
		if request.URL.Path != "/repos/acme/repo/pulls/9" {
			t.Errorf("path = %s", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"number":9,"body":"<!-- reforge-operation-id:op-fork -->","state":"open","repository":{"id":1,"full_name":"acme/repo"},"user":{"id":2,"login":"reforge[bot]","type":"Bot"},"head":{"ref":"feature","sha":"fork-head","repo":{"id":2,"full_name":"other/repo"}},"base":{"ref":"main","sha":"base","repo":{"id":1,"full_name":"acme/repo"}}}`))
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	change, err := provider.ReadChange(context.Background(), forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, "9")
	if err != nil || change.Repository.NativeID != "1" || change.HeadRepository.NativeID != "2" || change.TargetRepository.NativeID != "1" || VerifyOperationOwner(change, forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, "op-fork", "feature", "main", "2") {
		t.Fatalf("change=%#v err=%v", change, err)
	}
}

func TestCreateChangeRejectsStaleHeadBeforePost(t *testing.T) {
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/git/ref/heads/feature") {
			_, _ = writer.Write([]byte(`{"object":{"sha":"new-head"}}`))
			return
		}
		if request.Method == http.MethodPost {
			postCount++
		}
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider = authorizedFixture(provider)
	_, err = provider.CreateChange(context.Background(), forge.CreateChangeRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, Title: "Repair", Body: "body", HeadBranch: "feature", TargetBranch: "main", ExpectedHeadSHA: "1111111111111111111111111111111111111111", OperationID: "op-stale"})
	providerErr, ok := err.(*domain.ProviderError)
	if !ok || providerErr.Kind != "conflict" || postCount != 0 {
		t.Fatalf("err=%#v posts=%d", err, postCount)
	}
}

func TestReadFileResolveRefAndRequestReview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/repo/contents/.github/CODEOWNERS":
			_, _ = writer.Write([]byte(`{"type":"file","path":".github/CODEOWNERS","sha":"file-sha","encoding":"base64","content":"IyBvd25lcnM="}`))
		case "/repos/acme/repo/git/ref/heads/main":
			_, _ = writer.Write([]byte(`{"object":{"sha":"main-sha"}}`))
		case "/repos/acme/repo/pulls/7/requested_reviewers":
			if request.Method != http.MethodPost {
				t.Errorf("method = %s", request.Method)
			}
			writer.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	file, err := provider.ReadFileAtRef(context.Background(), forge.RepoRef{FullName: "acme/repo"}, ".github/CODEOWNERS", "main")
	if err != nil || string(file.Content) != "# owners" || file.SHA != "file-sha" {
		t.Fatalf("file=%#v err=%v", file, err)
	}
	ref, err := provider.ResolveRef(context.Background(), forge.RepoRef{FullName: "acme/repo"}, "main")
	if err != nil || ref != "main-sha" {
		t.Fatalf("ref=%s err=%v", ref, err)
	}
	provider = authorizedFixture(provider)
	if err := provider.RequestReview(context.Background(), forge.RepoRef{FullName: "acme/repo"}, "7", []string{"platform"}); err != nil {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

var _ forge.ForgeInventory = (*Provider)(nil)
var _ forge.ForgeEvents = (*Provider)(nil)
var _ forge.ForgeChanges = (*Provider)(nil)

func authorizedFixture(p *Provider) *Provider {
	p.app = &installationAuth{appID: "42", installationID: "17", token: p.config.Token, actorID: "2", slug: "reforge", expires: time.Now().Add(time.Hour)}
	return p.WithChangeAuthorizer(func(context.Context, forge.CreateChangeRequest) error { return nil }).WithBranchAuthorizer(func(context.Context, forge.UpdateBranchRequest) error { return nil }).WithReviewAuthorizer(func(context.Context, forge.RepoRef, string, []string) error { return nil })
}

func TestListRepositoriesAcceptsSingleRepositoryScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v3/repos/sindef/servicer" {
			t.Errorf("path = %s", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"id":7,"full_name":"sindef/servicer","default_branch":"main"}`))
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL + "/api/v3", Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{"sindef/servicer", "https://github.com/sindef/servicer.git"} {
		page, err := provider.ListRepositories(context.Background(), forge.InventoryRequest{Namespace: namespace})
		if err != nil || !page.Complete || len(page.Items) != 1 || page.Items[0].FullName != "sindef/servicer" {
			t.Fatalf("%s: %+v %v", namespace, page, err)
		}
	}
}

func TestTokenConnectionOwnsOperationsAsTokenUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/user":
			_, _ = writer.Write([]byte(`{"id":5,"login":"owner"}`))
		case "/repos/acme/repo/git/ref/heads/main":
			_, _ = writer.Write([]byte(`{"object":{"sha":"2222222222222222222222222222222222222222"}}`))
		case "/repos/acme/repo/pulls":
			_, _ = writer.Write([]byte(`[{"number":7,"title":"Repair","body":"<!-- reforge-operation-id:op-1 -->","state":"open","repository":{"id":1,"full_name":"acme/repo"},"user":{"id":5,"login":"owner","type":"User"},"head":{"ref":"reforge/repair/x","sha":"1111111111111111111111111111111111111111","repo":{"id":1,"full_name":"acme/repo"}},"base":{"ref":"main","sha":"base-1","repo":{"id":1,"full_name":"acme/repo"}}}]`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	found, err := provider.FindChangeByOperation(context.Background(), forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, "op-1", "reforge/repair/x", "main")
	if err != nil || found == nil || found.ID != "7" {
		t.Fatalf("found=%#v err=%v", found, err)
	}
	if _, err := provider.WriteExecutionCheck(context.Background(), forge.ExecutionCheckRequest{}); err == nil {
		t.Fatal("check runs must stay App-only")
	}
}
