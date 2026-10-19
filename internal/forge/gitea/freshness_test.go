package gitea

import (
	"context"
	"github.com/reforgeapp/reforge/internal/forge"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBehindReadsReverseDirectExactCommitComparison(t *testing.T) {
	base, head := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, tc := range []struct {
		name, body string
		want       int
		fail       bool
	}{{"current", `{"total_commits":0}`, 0, false}, {"behind", `{"total_commits":2}`, 2, false}, {"unrelated histories", `{"total_commits":3}`, 3, false}, {"missing", `{}`, 0, true}, {"negative", `{"total_commits":-1}`, 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/team/repo/compare/"+head+".."+base {
					t.Errorf("incorrect comparison: %s %s", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			provider, err := New(forge.Config{BaseURL: server.URL, Client: server.Client(), Token: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := provider.Behind(context.Background(), forge.RepoRef{NativeID: "17", FullName: "team/repo"}, base, head)
			if (err != nil) != tc.fail || !tc.fail && got != tc.want {
				t.Fatalf("behind=%d error=%v", got, err)
			}
		})
	}
}
