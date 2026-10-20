package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/reforgeapp/reforge/pkg/forge"
)

func TestRefreshAppBranchPreflightsAndUsesNativeRebase(t *testing.T) {
	const head = "1111111111111111111111111111111111111111"
	const target = "2222222222222222222222222222222222222222"
	for _, tc := range []struct {
		name       string
		author     int
		headSHA    string
		updateCode int
		wantWrite  bool
	}{
		{name: "accepted", author: 5, headSHA: head, updateCode: http.StatusAccepted, wantWrite: true},
		{name: "wrong actor", author: 6, headSHA: head},
		{name: "stale head", author: 5, headSHA: "3333333333333333333333333333333333333333"},
		{name: "provider failure", author: 5, headSHA: head, updateCode: http.StatusInternalServerError, wantWrite: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes, authorized := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v4/user":
					_, _ = w.Write([]byte(`{"id":5}`))
				case "/api/v4/projects/17/merge_requests/7":
					_, _ = w.Write([]byte(`{"iid":7,"project_id":17,"target_project_id":17,"source_project_id":17,"source_branch":"reforge/repair/task-1","target_branch":"main","state":"opened","sha":"` + tc.headSHA + `","diff_refs":{"head_sha":"` + tc.headSHA + `"},"author":{"id":` + strconv.Itoa(tc.author) + `}}`))
				case "/api/v4/projects/17/repository/branches/main":
					_, _ = w.Write([]byte(`{"commit":{"id":"` + target + `"}}`))
				case "/api/v4/projects/17/merge_requests/7/rebase":
					writes++
					if r.Method != http.MethodPut {
						t.Errorf("rebase method %s", r.Method)
					}
					w.WriteHeader(tc.updateCode)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			p, err := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "token"})
			if err != nil {
				t.Fatal(err)
			}
			p = p.WithBranchRefreshAuthorizer(func(context.Context, forge.RefreshBranchRequest) error { authorized++; return nil })
			in := forge.RefreshBranchRequest{Repository: forge.RepoRef{NativeID: "17", FullName: "group/repo"}, ChangeID: "7", HeadBranch: "reforge/repair/task-1", TargetBranch: "main", ExpectedHeadSHA: head, ExpectedTargetSHA: target, OperationID: "refresh-1"}
			err = p.RefreshAppBranch(context.Background(), in)
			if tc.name == "accepted" && err != nil {
				t.Fatal(err)
			}
			if tc.name == "provider failure" && err == nil {
				t.Fatal("provider failure accepted")
			}
			if (tc.name == "wrong actor" || tc.name == "stale head") && err == nil {
				t.Fatal("invalid preflight accepted")
			}
			if (writes > 0) != tc.wantWrite {
				t.Fatalf("writes=%d wantWrite=%t", writes, tc.wantWrite)
			}
			if tc.wantWrite && authorized != 1 {
				t.Fatalf("authorization calls=%d", authorized)
			}
		})
	}
}

func TestRefreshAppBranchRequiresAuthorization(t *testing.T) {
	p, err := New(forge.Config{BaseURL: "https://gitlab.example", Client: &http.Client{}, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.RefreshAppBranch(context.Background(), forge.RefreshBranchRequest{}); err == nil {
		t.Fatal("refresh without persisted authorization accepted")
	}
}
