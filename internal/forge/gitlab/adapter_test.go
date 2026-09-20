package gitlab

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

func TestNewRequiresExplicitClientAndValidConnection(t *testing.T) {
	if _, err := New(forge.Config{BaseURL: "https://gitlab.com", Token: "token"}); err == nil {
		t.Fatal("expected client requirement")
	}
	client := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF })
	if _, err := New(forge.Config{Client: client, Token: "token"}); err == nil {
		t.Fatal("expected base URL requirement")
	}
	if _, err := New(forge.Config{BaseURL: "https://gitlab.com", Client: client}); err == nil {
		t.Fatal("expected token requirement")
	}
	if _, err := New(forge.Config{BaseURL: "https://token@gitlab.com", Client: client, Token: "token"}); err == nil {
		t.Fatal("expected credential-bearing URL rejection")
	}
}

func TestListRepositoriesSubgroupsPaginationAndImmutableIDs(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v4/groups/42/projects" || request.URL.Query().Get("include_subgroups") != "true" {
			t.Errorf("request = %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("PRIVATE-TOKEN") != "access-token" {
			t.Errorf("private token header missing")
		}
		if request.URL.Query().Get("page") == "1" {
			writer.Header().Set("X-Next-Page", "2")
			_, _ = writer.Write([]byte(`[{"id":17,"path_with_namespace":"group/old-name","web_url":"https://gitlab.example/group/old-name","http_url_to_repo":"https://gitlab.example/group/old-name.git","default_branch":"main","visibility":"private"}]`))
			return
		}
		_, _ = writer.Write([]byte(`[{"id":17,"path_with_namespace":"group/new-name","web_url":"https://gitlab.example/group/new-name","http_url_to_repo":"https://gitlab.example/group/new-name.git","default_branch":"main","visibility":"private"}]`))
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL + "/api/v4", Token: "access-token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.ListRepositories(context.Background(), forge.InventoryRequest{Namespace: "group:42", Limit: 2})
	if err != nil || len(first.Items) != 1 || first.Items[0].NativeID != "17" || first.NextCursor != "2" || first.Complete {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := provider.ListRepositories(context.Background(), forge.InventoryRequest{Namespace: "group:42", Cursor: first.NextCursor, Limit: 2})
	if err != nil || len(second.Items) != 1 || second.Items[0].NativeID != "17" || second.Items[0].FullName != "group/new-name" || second.NextCursor != "" || !second.Complete {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

func TestWebhookTokenAndSignedVerification(t *testing.T) {
	provider, err := New(forge.Config{BaseURL: "https://gitlab.com", Token: "token", WebhookSecret: "webhook-secret", Client: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF })})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"object_kind":"merge_request","project":{"id":17,"path_with_namespace":"group/repo"},"object_attributes":{"iid":9,"last_commit":{"id":"head-9"}}}`)
	headers := make(http.Header)
	headers.Set("X-Gitlab-Token", "webhook-secret")
	headers.Set("X-Gitlab-Event", "Merge Request Hook")
	headers.Set("X-Gitlab-Event-UUID", "delivery-9")
	event, err := provider.DecodeEvent(headers, payload)
	if err != nil || event.Kind != "merge_request" || event.DeliveryID != "delivery-9" || event.Repository.NativeID != "17" || event.ChangeID != "9" || event.HeadSHA != "head-9" {
		t.Fatalf("event=%#v err=%v", event, err)
	}
	headers.Set("X-Gitlab-Token", "wrong")
	if err := provider.VerifyWebhook(headers, payload); err == nil {
		t.Fatal("expected invalid webhook token")
	}

	secret := base64.StdEncoding.EncodeToString([]byte("signing-secret"))
	signed, err := New(forge.Config{BaseURL: "https://gitlab.com", Token: "token", WebhookSecret: "whsec_" + secret, Client: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF })})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := time.Now().Unix()
	digest := hmac.New(sha256.New, []byte("signing-secret"))
	_, _ = digest.Write([]byte("delivery-9." + strconvFormatInt(timestamp) + "."))
	_, _ = digest.Write(payload)
	signedHeaders := make(http.Header)
	signedHeaders.Set("webhook-id", "delivery-9")
	signedHeaders.Set("webhook-timestamp", strconvFormatInt(timestamp))
	signedHeaders.Set("webhook-signature", "v1,"+base64.StdEncoding.EncodeToString(digest.Sum(nil)))
	if err := signed.VerifyWebhook(signedHeaders, payload); err != nil {
		t.Fatal(err)
	}
}

func TestReadFileResolveRefAndChecksPreserveSHAAndPublisher(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v4/projects/17/repository/files/.gitlab-ci.yml/raw":
			_, _ = writer.Write([]byte("stages:\n  - test\n"))
		case "/api/v4/projects/17/repository/branches/main":
			_, _ = writer.Write([]byte(`{"commit":{"id":"target-current"}}`))
		case "/api/v4/projects/17/repository/tags/v1":
			_, _ = writer.Write([]byte(`{"commit":{"id":"tag-sha"}}`))
		case "/api/v4/projects/17/repository/commits/head-sha/statuses":
			if request.URL.Query().Get("page") == "1" {
				writer.Header().Set("X-Next-Page", "2")
				_, _ = writer.Write([]byte(`[{"id":1,"name":"build","sha":"head-sha","status":"success","target_url":"https://ci/build","author":{"id":81}}]`))
				break
			}
			_, _ = writer.Write([]byte(`[]`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	reference := forge.RepoRef{NativeID: "17", FullName: "group/repo"}
	file, err := provider.ReadFileAtRef(context.Background(), reference, ".gitlab-ci.yml", "main")
	if err != nil || string(file.Content) != "stages:\n  - test\n" || file.Path != ".gitlab-ci.yml" {
		t.Fatalf("file=%#v err=%v", file, err)
	}
	if ref, err := provider.ResolveRef(context.Background(), reference, "refs/tags/v1"); err != nil || ref != "tag-sha" {
		t.Fatalf("ref=%s err=%v", ref, err)
	}
	checks, err := provider.ListChecks(context.Background(), reference, "head-sha")
	if err != nil || len(checks) != 1 || checks[0].HeadSHA != "head-sha" || checks[0].PublisherID != "81" {
		t.Fatalf("checks=%#v err=%v", checks, err)
	}
}

func TestReadChangeUsesCurrentTargetAndForkIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v4/projects/17/merge_requests/9":
			_, _ = writer.Write([]byte(`{"iid":9,"project_id":17,"source_project_id":23,"target_project_id":17,"title":"Repair","description":"body\n\n<!-- reforge-operation-id:op-9 -->","web_url":"https://gitlab.example/group/repo/-/merge_requests/9","state":"opened","source_branch":"feature","target_branch":"main","sha":"head-full","diff_refs":{"head_sha":"head-full","start_sha":"old-target"},"author":{"id":81,"username":"automation","bot":true}}`))
		case "/api/v4/projects/17/repository/branches/main":
			_, _ = writer.Write([]byte(`{"commit":{"id":"target-current"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	change, err := provider.ReadChange(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "9")
	if err != nil || change.HeadRepository.NativeID != "23" || change.TargetRepository.NativeID != "17" || change.HeadSHA != "head-full" || change.TargetSHA != "target-current" || change.Body != "body" {
		t.Fatalf("change=%#v err=%v", change, err)
	}
	if VerifyOperationOwner(change, forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "op-9", "feature", "main", "81") {
		t.Fatal("forked source must not be adopted as app-owned")
	}
}

func TestListBotWorkUsesNativeBotFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v4/projects/17/merge_requests":
			_, _ = writer.Write([]byte(`[{"iid":1,"project_id":17,"source_project_id":17,"target_project_id":17,"source_branch":"one","target_branch":"main","sha":"one-sha","author":{"id":81,"username":"automation-bot","bot":true}},{"iid":2,"project_id":17,"source_project_id":17,"target_project_id":17,"source_branch":"two","target_branch":"main","sha":"two-sha","author":{"id":82,"username":"looks-like-a-bot","bot":false}}]`))
		case "/api/v4/projects/17/repository/branches/main":
			_, _ = writer.Write([]byte(`{"commit":{"id":"target-current"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := provider.ListBotWork(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"})
	if err != nil || len(changes) != 1 || changes[0].ID != "1" || changes[0].AuthorID != "81" {
		t.Fatalf("changes=%#v err=%v", changes, err)
	}
}

func TestCreateChangePreflightsExactHeadAndRequestsReview(t *testing.T) {
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v4/user":
			_, _ = writer.Write([]byte(`{"id":81}`))
		case "/api/v4/projects/17/merge_requests":
			if request.Method == http.MethodGet {
				_, _ = writer.Write([]byte(`[]`))
				return
			}
			postCount++
			if request.Header.Get("Idempotency-Key") != "" {
				t.Errorf("undocumented idempotency promise sent")
			}
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), "reforge-operation-id:op-1") {
				t.Errorf("marker missing: %s", body)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"iid":9,"project_id":17,"source_project_id":17,"target_project_id":17,"title":"Repair","description":"body\n\n<!-- reforge-operation-id:op-1 -->","web_url":"https://gitlab.example/repo/-/merge_requests/9","state":"opened","source_branch":"feature","target_branch":"main","sha":"1111111111111111111111111111111111111111","author":{"id":81,"username":"automation","bot":true}}`))
		case "/api/v4/projects/17/repository/branches/feature":
			_, _ = writer.Write([]byte(`{"commit":{"id":"1111111111111111111111111111111111111111"}}`))
		case "/api/v4/projects/17/repository/branches/main":
			_, _ = writer.Write([]byte(`{"commit":{"id":"target-1"}}`))
		case "/api/v4/projects/17/merge_requests/9":
			if request.Method != http.MethodPut {
				t.Errorf("review method = %s", request.Method)
			}
			writer.WriteHeader(http.StatusOK)
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
	reference := forge.RepoRef{NativeID: "17", FullName: "group/repo"}
	change, err := provider.CreateChange(context.Background(), forge.CreateChangeRequest{Repository: reference, Title: "Repair", Body: "body", HeadBranch: "feature", TargetBranch: "main", ExpectedHeadSHA: "1111111111111111111111111111111111111111", OperationID: "op-1"})
	if err != nil || change.ID != "9" || change.TargetSHA != "target-1" || postCount != 1 {
		t.Fatalf("change=%#v err=%v posts=%d", change, err, postCount)
	}
	if err := provider.RequestReview(context.Background(), reference, "9", []string{"81"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateChangeRejectsStaleHead(t *testing.T) {
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v4/user" {
			_, _ = writer.Write([]byte(`{"id":81}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/merge_requests") {
			_, _ = writer.Write([]byte(`[]`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/repository/branches/feature") {
			_, _ = writer.Write([]byte(`{"commit":{"id":"new-head"}}`))
			return
		}
		if request.Method == http.MethodPost {
			postCount++
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider = authorizedFixture(provider)
	_, err = provider.CreateChange(context.Background(), forge.CreateChangeRequest{Repository: forge.RepoRef{NativeID: "17", FullName: "group/repo"}, Title: "Repair", HeadBranch: "feature", TargetBranch: "main", ExpectedHeadSHA: "1111111111111111111111111111111111111111", OperationID: "op-stale"})
	providerErr, ok := err.(*domain.ProviderError)
	if !ok || providerErr.Kind != "conflict" || postCount != 0 {
		t.Fatalf("err=%#v posts=%d", err, postCount)
	}
}

func TestOperationAdoptionBindsAuthenticatedActor(t *testing.T) {
	actor := int64(82)
	provider, err := New(forge.Config{BaseURL: "https://gitlab.example", Token: "token", Client: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/api/v4/user":
			body = `{"id":81}`
		case "/api/v4/projects/17/merge_requests":
			body = `[{"iid":9,"project_id":17,"source_project_id":17,"target_project_id":17,"source_branch":"reforge/op","target_branch":"main","sha":"head","description":"<!-- reforge-operation-id:operation -->","author":{"id":` + strconv.FormatInt(actor, 10) + `,"bot":false}}]`
		case "/api/v4/projects/17/repository/branches/main":
			body = `{"commit":{"id":"base"}}`
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	ref := forge.RepoRef{NativeID: "17", FullName: "group/repo"}
	if found, err := provider.FindChangeByOperation(context.Background(), ref, "operation", "reforge/op", "main"); err != nil || found != nil {
		t.Fatalf("spoofed actor adopted: %v %v", found, err)
	}
	actor = 81
	if found, err := provider.FindChangeByOperation(context.Background(), ref, "operation", "reforge/op", "main"); err != nil || found == nil {
		t.Fatalf("authenticated actor could not reconcile: %v %v", found, err)
	}
}

func TestPaginationRejectsNonAdvancingProviderCursor(t *testing.T) {
	provider, err := New(forge.Config{BaseURL: "https://gitlab.example", Token: "token", Client: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Next-Page": []string{"1"}}, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.ListRepositories(context.Background(), forge.InventoryRequest{}); err == nil {
		t.Fatal("repeating provider page marked complete")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

var _ forge.ForgeInventory = (*Provider)(nil)
var _ forge.ForgeEvents = (*Provider)(nil)
var _ forge.ForgeChanges = (*Provider)(nil)

func authorizedFixture(p *Provider) *Provider {
	return p.WithChangeAuthorizer(func(context.Context, forge.CreateChangeRequest) error { return nil }).WithReviewAuthorizer(func(context.Context, forge.RepoRef, string, []string) error { return nil }).WithBranchAuthorizer(func(context.Context, forge.UpdateBranchRequest) error { return nil })
}
