package gitlab

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestApprovalsBindOnlyCurrentNativeResetRevision(t *testing.T) {
	for _, mode := range []string{"current", "stale", "syncing"} {
		t.Run(mode, func(t *testing.T) {
			fixture := &nativeFixture{mode: "eligible"}
			if mode == "stale" {
				fixture.mode = "stale_approval"
			}
			provider := fixtureProvider(t, func(request *http.Request) (*http.Response, error) {
				if strings.HasSuffix(request.URL.Path, "/merge_requests/9/approvals") {
					if mode == "syncing" {
						fixture.mode = "calculating"
					}
					return jsonResponse(200, map[string]any{"iid": 9, "project_id": 17, "approved_by": []any{map[string]any{"user": map[string]int{"id": 82}}}}), nil
				}
				return fixture.call(t, request)
			})
			approvals, err := provider.ReadApprovals(context.Background(), testRepo, "9")
			if mode == "syncing" {
				if err == nil {
					t.Fatal("syncing approvals trusted")
				}
				return
			}
			if err != nil || len(approvals) != 1 {
				t.Fatalf("approvals=%+v err=%v", approvals, err)
			}
			want := ""
			if mode == "current" {
				want = testHead
			}
			if approvals[0].HeadSHA != want {
				t.Fatalf("incorrect review binding: %+v", approvals)
			}
		})
	}
}
