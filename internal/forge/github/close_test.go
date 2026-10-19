package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reforgeapp/reforge/internal/forge"
)

func TestCloseChangeOnlyClosesOwnPullRequest(t *testing.T) {
	author, closed := "5", false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/user":
			_, _ = writer.Write([]byte(`{"id":5}`))
		case request.URL.Path == "/repos/acme/repo/git/ref/heads/main":
			_, _ = writer.Write([]byte(`{"object":{"sha":"2222222222222222222222222222222222222222"}}`))
		case request.URL.Path == "/repos/acme/repo/pulls/7" && request.Method == http.MethodGet:
			state := "open"
			if closed {
				state = "closed"
			}
			_, _ = writer.Write([]byte(`{"number":7,"state":"` + state + `","user":{"id":` + author + `,"type":"User"},"head":{"ref":"reforge/repair/x","sha":"1111111111111111111111111111111111111111","repo":{"id":1,"full_name":"acme/repo"}},"base":{"ref":"main","sha":"2222222222222222222222222222222222222222","repo":{"id":1,"full_name":"acme/repo"}}}`))
		case request.URL.Path == "/repos/acme/repo/issues/7/comments":
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{}`))
		case request.URL.Path == "/repos/acme/repo/pulls/7" && request.Method == http.MethodPatch:
			closed = true
			_, _ = writer.Write([]byte(`{}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	in := forge.CloseChangeRequest{Repository: forge.RepoRef{NativeID: "1", FullName: "acme/repo"}, ChangeID: "7", HeadBranch: "reforge/repair/x", Comment: "giving up"}
	author = "6"
	if _, err = provider.CloseChange(context.Background(), in); err == nil || closed {
		t.Fatal("closed a pull request by another author")
	}
	author = "5"
	change, err := provider.CloseChange(context.Background(), in)
	if err != nil || !closed || change.State != "closed" {
		t.Fatalf("change=%+v err=%v", change, err)
	}
}
