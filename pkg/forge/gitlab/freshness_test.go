package gitlab

import (
	"context"
	"github.com/reforgeapp/reforge/pkg/forge"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBehindReadsReverseExactCommitComparison(t *testing.T) {
	base, head := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, tc := range []struct {
		name, body string
		want       int
		fail       bool
	}{{"current", `{"commits":[],"compare_timeout":false}`, 0, false}, {"behind", `{"commits":[{"id":"` + base + `"}],"compare_timeout":false}`, 1, false}, {"missing", `{}`, 0, true}, {"timeout", `{"commits":[],"compare_timeout":true}`, 0, true}, {"invalid commit", `{"commits":[{"id":"bad"}],"compare_timeout":false}`, 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v4/projects/17/repository/compare" || r.URL.Query().Get("from") != head || r.URL.Query().Get("to") != base || r.URL.Query().Get("straight") != "true" {
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
