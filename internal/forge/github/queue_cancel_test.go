package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

func TestCancelNativeQueueChecksIdentityAndReadsCanonicalState(t *testing.T) {
	head := strings.Repeat("a", 40)
	target := strings.Repeat("b", 40)
	queue := `{"id":"queue-1","state":"QUEUED","baseCommit":{"oid":"` + target + `"}}`
	requests := 0
	provider, err := New(forge.Config{BaseURL: "https://github.example", Token: "token", Client: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet && request.URL.Path == "/repos/org/repo/pulls/7" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"number":7,"state":"open","draft":false,"merged":false,"auto_merge":null,"head":{"ref":"feature","sha":"` + head + `","repo":{"id":1,"full_name":"org/repo"}},"base":{"ref":"main","sha":"` + target + `","repo":{"id":1,"full_name":"org/repo"}},"repository":{"id":1,"full_name":"org/repo"}}`))}, nil
		}
		if request.Method == http.MethodGet && request.URL.Path == "/repos/org/repo/git/ref/heads/main" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"object":{"sha":"` + target + `"}}`))}, nil
		}
		return nil, io.EOF
	})})
	if err != nil {
		t.Fatal(err)
	}
	provider, err = provider.WithGraphQL("https://github.example/graphql", roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			return nil, err
		}
		var data string
		if strings.Contains(body.Query, "dequeuePullRequest") {
			if body.Variables["input"].(map[string]any)["clientMutationId"] != "11111111-1111-4111-8111-111111111111" {
				t.Error("operation ID missing")
			}
			data = `{"dequeuePullRequest":{"mergeQueueEntry":{"id":"queue-1"}}}`
		} else if requests == 1 {
			data = `{"repository":{"databaseId":1,"pullRequest":{"id":"PR_node","headRefOid":"` + head + `","baseRefOid":"` + target + `","mergeQueueEntry":` + queue + `}}}`
		} else {
			data = `{"repository":{"databaseId":1,"pullRequest":{"id":"PR_node","headRefOid":"` + head + `","baseRefOid":"` + target + `","mergeQueueEntry":null}}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":` + data + `}`))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	state, err := provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, ChangeID: "7", QueueID: "queue-1", ExpectedHeadSHA: head, OperationID: "11111111-1111-4111-8111-111111111111"})
	if err != nil || state.State != "not_queued" || requests != 3 {
		t.Fatalf("state=%+v requests=%d err=%v", state, requests, err)
	}
}

func TestCancelNativeQueueRejectsChangedAdmissionWithoutMutation(t *testing.T) {
	head := strings.Repeat("a", 40)
	provider, err := New(forge.Config{BaseURL: "https://github.example", Token: "token", Client: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.EOF })})
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	provider, err = provider.WithGraphQL("https://github.example/graphql", roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"repository":{"databaseId":1,"pullRequest":{"id":"PR_node","headRefOid":"` + strings.Repeat("c", 40) + `","baseRefOid":"` + strings.Repeat("b", 40) + `","mergeQueueEntry":{"id":"queue-1","state":"QUEUED"}}}}}`))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, ChangeID: "7", QueueID: "queue-1", ExpectedHeadSHA: head, OperationID: domain.NewID()})
	if err == nil || requests != 1 {
		t.Fatalf("changed admission err=%v requests=%d", err, requests)
	}
}

func TestCancelNativeQueuePostWriteRacesAreUncertain(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		state string
		head  string
		queue string
	}{
		{"merged", "merged", strings.Repeat("a", 40), ""},
		{"head changed", "open", strings.Repeat("c", 40), ""},
		{"new queue", "open", strings.Repeat("a", 40), "queue-2"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			head := strings.Repeat("a", 40)
			target := strings.Repeat("b", 40)
			graphRequests := 0
			api := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/repos/org/repo/pulls/7" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"number":7,"state":"` + scenario.state + `","draft":false,"merged":` + strconv.FormatBool(scenario.state == "merged") + `,"auto_merge":null,"head":{"ref":"feature","sha":"` + scenario.head + `","repo":{"id":1,"full_name":"org/repo"}},"base":{"ref":"main","sha":"` + target + `","repo":{"id":1,"full_name":"org/repo"}},"repository":{"id":1,"full_name":"org/repo"}}`))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"object":{"sha":"` + target + `"}}`))}, nil
			})
			provider, err := New(forge.Config{BaseURL: "https://github.example", Token: "token", Client: api})
			if err != nil {
				t.Fatal(err)
			}
			provider, err = provider.WithGraphQL("https://github.example/graphql", roundTripFunc(func(request *http.Request) (*http.Response, error) {
				graphRequests++
				var body struct {
					Query string `json:"query"`
				}
				_ = json.NewDecoder(request.Body).Decode(&body)
				if strings.Contains(body.Query, "dequeuePullRequest") {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"dequeuePullRequest":{"mergeQueueEntry":{"id":"queue-1"}}}}`))}, nil
				}
				entry := `{"id":"queue-1","state":"QUEUED","baseCommit":{"oid":"` + target + `"}}`
				if graphRequests > 2 {
					if scenario.queue == "" {
						entry = "null"
					} else {
						entry = `{"id":"` + scenario.queue + `","state":"QUEUED","baseCommit":{"oid":"` + target + `"}}`
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"repository":{"databaseId":1,"pullRequest":{"id":"PR_node","headRefOid":"` + head + `","baseRefOid":"` + target + `","mergeQueueEntry":` + entry + `}}}}`))}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, ChangeID: "7", QueueID: "queue-1", ExpectedHeadSHA: head, OperationID: domain.NewID()})
			if err == nil {
				t.Fatal("post-write race accepted")
			}
		})
	}
}

func TestCancelNativeQueueConvergesAbsentCanonicalQueueWithoutWrite(t *testing.T) {
	head := strings.Repeat("a", 40)
	target := strings.Repeat("b", 40)
	graphqlRequests := 0
	provider, err := New(forge.Config{BaseURL: "https://github.example", Token: "token", Client: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/repos/org/repo/pulls/7" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"number":7,"state":"open","draft":false,"merged":false,"auto_merge":null,"head":{"sha":"` + head + `"},"base":{"sha":"` + target + `","repo":{"id":1,"full_name":"org/repo"}}}`))}, nil
		}
		return nil, io.EOF
	})})
	if err != nil {
		t.Fatal(err)
	}
	provider, err = provider.WithGraphQL("https://github.example/graphql", roundTripFunc(func(request *http.Request) (*http.Response, error) {
		graphqlRequests++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"repository":{"databaseId":1,"pullRequest":{"id":"PR_node","headRefOid":"` + head + `","baseRefOid":"` + target + `","mergeQueueEntry":null}}}}`))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	state, err := provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, ChangeID: "7", QueueID: "queue-1", ExpectedHeadSHA: head, OperationID: domain.NewID()})
	if err != nil || state.State != "not_queued" || graphqlRequests != 2 {
		t.Fatalf("state=%+v graphql=%d err=%v", state, graphqlRequests, err)
	}
}
