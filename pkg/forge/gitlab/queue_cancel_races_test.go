package gitlab

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

func TestCancelNativeQueuePostWriteRaceFixtures(t *testing.T) {
	for _, scenario := range []struct {
		name string
		kind string
	}{
		{name: "post-cancel changed head", kind: "head_changed"},
		{name: "post-cancel merged state", kind: "merged"},
		{name: "post-cancel new queue admission", kind: "new_queue"},
		{name: "uncertain write result", kind: "uncertain_write"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			head := strings.Repeat("a", 40)
			target := strings.Repeat("b", 40)
			postWrites := 0
			afterCancel := false
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch {
				case request.Method == http.MethodPost && request.URL.Path == "/api/v4/projects/1/merge_requests/7/cancel_merge_when_pipeline_succeeds":
					postWrites++
					afterCancel = true
					writer.WriteHeader(http.StatusCreated)
				case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/merge_requests/7":
					currentHead, state := head, "opened"
					if afterCancel && scenario.kind == "head_changed" {
						currentHead = strings.Repeat("c", 40)
					}
					if afterCancel && scenario.kind == "merged" {
						state = "merged"
					}
					io.WriteString(writer, `{"id":70,"iid":7,"project_id":1,"source_project_id":1,"target_project_id":1,"sha":"`+currentHead+`","diff_refs":{"head_sha":"`+currentHead+`"},"source_branch":"feature","target_branch":"main","state":"`+state+`","draft":false,"merge_when_pipeline_succeeds":false}`)
				case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/repository/branches/main":
					io.WriteString(writer, `{"commit":{"id":"`+target+`"}}`)
				case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/merge_trains/merge_requests/7":
					queueID := `9`
					if afterCancel && scenario.kind == "new_queue" {
						queueID = `10`
					}
					io.WriteString(writer, `{"id":`+queueID+`,"status":"idle","target_branch":"main","merge_request":{"iid":7,"project_id":1},"pipeline":{"id":1,"project_id":1,"sha":"`+head+`"}}`)
				case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/repository/commits/"+head:
					io.WriteString(writer, `{"parent_ids":["`+head+`","`+target+`"]}`)
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			var client forge.HTTPClient = server.Client()
			if scenario.kind == "uncertain_write" {
				clientFunc := roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if request.Method == http.MethodPost {
						postWrites++
						return nil, errors.New("connection lost after write")
					}
					return server.Client().Do(request)
				})
				provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: clientFunc})
				if err != nil {
					t.Fatal(err)
				}
				_, err = provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{
					Repository:      forge.RepoRef{NativeID: "1", FullName: "org/repo"},
					ChangeID:        "7",
					QueueID:         "9",
					ExpectedHeadSHA: head,
					OperationID:     domain.NewID(),
				})
				var providerErr *domain.ProviderError
				if err == nil || !errors.As(err, &providerErr) || !providerErr.Uncertain {
					t.Fatalf("race accepted or not uncertain: err=%v", err)
				}
				if postWrites != 1 {
					t.Fatalf("mutation retried: writes=%d", postWrites)
				}
				return
			}
			provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: client})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{
				Repository:      forge.RepoRef{NativeID: "1", FullName: "org/repo"},
				ChangeID:        "7",
				QueueID:         "9",
				ExpectedHeadSHA: head,
				OperationID:     domain.NewID(),
			})
			var providerErr *domain.ProviderError
			if err == nil || !errors.As(err, &providerErr) || !providerErr.Uncertain {
				t.Fatalf("race accepted or not uncertain: err=%v", err)
			}
			if postWrites != 1 {
				t.Fatalf("mutation retried: writes=%d", postWrites)
			}
		})
	}
}
