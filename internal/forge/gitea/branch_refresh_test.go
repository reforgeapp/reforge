package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"reforge/internal/forge"
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
		{name: "accepted", author: 5, headSHA: head, updateCode: http.StatusOK, wantWrite: true},
		{name: "wrong actor", author: 6, headSHA: head},
		{name: "stale head", author: 5, headSHA: "3333333333333333333333333333333333333333"},
		{name: "provider failure", author: 5, headSHA: "1111111111111111111111111111111111111111", updateCode: http.StatusInternalServerError, wantWrite: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes, authorized := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/v1/user":
					_, _ = w.Write([]byte(`{"id":5}`))
				case r.URL.Path == "/api/v1/repos/acme/repo":
					_, _ = w.Write([]byte(`{"id":17,"full_name":"acme/repo"}`))
				case r.URL.Path == "/api/v1/repos/acme/repo/pulls/7":
					_, _ = w.Write([]byte(`{"number":7,"state":"open","user":{"id":` + strconv.Itoa(tc.author) + `},"head":{"repo_id":17,"ref":"reforge/repair/task-1","sha":"` + tc.headSHA + `","repo":{"id":17,"full_name":"acme/repo"}},"base":{"repo_id":17,"ref":"main","sha":"` + target + `","repo":{"id":17,"full_name":"acme/repo"}}}`))
				case strings.HasPrefix(r.URL.EscapedPath(), "/api/v1/repos/acme/repo/git/commits/"):
					if strings.HasSuffix(r.URL.EscapedPath(), "/main") {
						_, _ = w.Write([]byte(`{"sha":"` + target + `"}`))
					} else {
						_, _ = w.Write([]byte(`{"sha":"` + tc.headSHA + `"}`))
					}
				case r.URL.Path == "/api/v1/repos/acme/repo/pulls/7/update":
					writes++
					if r.Method != http.MethodPost || r.URL.Query().Get("style") != "rebase" {
						t.Errorf("update request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
					}
					w.WriteHeader(tc.updateCode)
				default:
					t.Errorf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			p, err := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "token"})
			if err != nil {
				t.Fatal(err)
			}
			p = p.WithBranchRefreshAuthorizer(func(context.Context, forge.RefreshBranchRequest) error { authorized++; return nil })
			in := forge.RefreshBranchRequest{Repository: forge.RepoRef{NativeID: "17", FullName: "acme/repo"}, ChangeID: "7", HeadBranch: "reforge/repair/task-1", TargetBranch: "main", ExpectedHeadSHA: head, ExpectedTargetSHA: target, OperationID: "refresh-1"}
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
	p, err := New(forge.Config{BaseURL: "https://forge.example", Client: &http.Client{}, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.RefreshAppBranch(context.Background(), forge.RefreshBranchRequest{}); err == nil {
		t.Fatal("refresh without persisted authorization accepted")
	}
}
