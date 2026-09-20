package gitlab

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

func TestCancelNativeQueueReadsChangedCanonicalTrain(t *testing.T) {
	head := strings.Repeat("a", 40)
	target := strings.Repeat("b", 40)
	trainReads := 0
	cancelled := false
	tested := strings.Repeat("c", 40)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/merge_requests/7":
			io.WriteString(writer, `{"id":70,"iid":7,"project_id":1,"source_project_id":1,"target_project_id":1,"sha":"`+head+`","diff_refs":{"head_sha":"`+head+`"},"source_branch":"feature","target_branch":"main","state":"opened","draft":false,"merge_when_pipeline_succeeds":false}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/repository/branches/main":
			io.WriteString(writer, `{"commit":{"id":"`+target+`"}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/merge_trains/merge_requests/7":
			trainReads++
			if cancelled {
				http.NotFound(writer, request)
				return
			}
			queueID := int64(9)
			io.WriteString(writer, `{"id":`+strconv.FormatInt(queueID, 10)+`,"status":"idle","target_branch":"main","merge_request":{"iid":7,"project_id":1},"pipeline":{"id":1,"project_id":1,"sha":"`+tested+`"}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v4/projects/1/repository/commits/"+tested:
			io.WriteString(writer, `{"parent_ids":["`+head+`","`+target+`"]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/v4/projects/1/merge_requests/7/cancel_merge_when_pipeline_succeeds":
			cancelled = true
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
	state, err := provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, ChangeID: "7", QueueID: "9", ExpectedHeadSHA: head, OperationID: "11111111-1111-4111-8111-111111111111"})
	if err != nil || state.State != "not_queued" || trainReads != 2 || !cancelled {
		t.Fatalf("state=%+v trains=%d cancelled=%v err=%v", state, trainReads, cancelled, err)
	}
}

func TestCancelNativeQueueRejectsChangedTrainWithoutMutation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			t.Fatal("mutation sent for changed queue")
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v4/projects/1/merge_requests/7":
			io.WriteString(writer, `{"id":70,"iid":7,"project_id":1,"source_project_id":1,"target_project_id":1,"sha":"`+strings.Repeat("a", 40)+`","diff_refs":{"head_sha":"`+strings.Repeat("a", 40)+`"},"source_branch":"feature","target_branch":"main","state":"opened","draft":false,"merge_when_pipeline_succeeds":false}`)
		case "/api/v4/projects/1/repository/branches/main":
			io.WriteString(writer, `{"commit":{"id":"`+strings.Repeat("b", 40)+`"}}`)
		case "/api/v4/projects/1/merge_trains/merge_requests/7":
			io.WriteString(writer, `{"id":9,"status":"idle","target_branch":"main","merge_request":{"iid":7,"project_id":1},"pipeline":{"id":1,"project_id":1,"sha":"`+strings.Repeat("c", 40)+`"}}`)
		case "/api/v4/projects/1/repository/commits/" + strings.Repeat("c", 40):
			io.WriteString(writer, `{"parent_ids":["`+strings.Repeat("a", 40)+`","`+strings.Repeat("b", 40)+`"]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, ChangeID: "7", QueueID: "other", ExpectedHeadSHA: strings.Repeat("a", 40), OperationID: domain.NewID()})
	if err == nil {
		t.Fatal("changed train accepted")
	}
}

func TestCancelNativeQueueConvergesAbsentCanonicalTrainWithoutWrite(t *testing.T) {
	head := strings.Repeat("a", 40)
	target := strings.Repeat("b", 40)
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			writes++
			t.Fatal("mutation sent for absent train")
		}
		switch request.URL.Path {
		case "/api/v4/projects/1/merge_requests/7":
			io.WriteString(writer, `{"id":70,"iid":7,"project_id":1,"source_project_id":1,"target_project_id":1,"sha":"`+head+`","diff_refs":{"head_sha":"`+head+`"},"source_branch":"feature","target_branch":"main","state":"opened","draft":false,"merge_when_pipeline_succeeds":false}`)
		case "/api/v4/projects/1/repository/branches/main":
			io.WriteString(writer, `{"commit":{"id":"`+target+`"}}`)
		case "/api/v4/projects/1/merge_trains/merge_requests/7":
			http.NotFound(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	state, err := provider.CancelNativeQueue(context.Background(), forge.QueueCancelRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "org/repo"}, ChangeID: "7", QueueID: "9", ExpectedHeadSHA: head, OperationID: domain.NewID()})
	if err != nil || state.State != "not_queued" || writes != 0 {
		t.Fatalf("state=%+v writes=%d err=%v", state, writes, err)
	}
}
