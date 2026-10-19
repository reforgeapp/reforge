package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/internal/forge"
)

func TestReadCommitProofValidatesPinnedIdentityParentsAndMessage(t *testing.T) {
	commit := strings.Repeat("a", 40)
	parent := strings.Repeat("b", 40)
	for _, test := range []struct {
		name, response, wantMessage string
		wantErr                     bool
	}{
		{"valid", fmt.Sprintf(`{"id":%q,"message":"repair\n\n<!-- reforge-operation-id:op-1 -->","parent_ids":[%q],"author":{"id":7}}`, commit, parent), "repair\n\n<!-- reforge-operation-id:op-1 -->", false},
		{"wrong sha", fmt.Sprintf(`{"id":%q,"message":"repair","parent_ids":[]}`, strings.Repeat("c", 40)), "", true},
		{"invalid parent", fmt.Sprintf(`{"id":%q,"message":"repair","parent_ids":["bad"]}`, commit), "", true},
		{"missing parents", fmt.Sprintf(`{"id":%q,"message":"repair"}`, commit), "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v4/projects/17":
					fmt.Fprint(w, `{"id":17,"path_with_namespace":"group/repo"}`)
				case "/api/v4/projects/17/repository/commits/" + commit:
					fmt.Fprint(w, test.response)
				default:
					t.Errorf("path = %s", r.URL.Path)
				}
			}))
			defer server.Close()
			provider, err := New(forge.Config{BaseURL: server.URL, Token: "token", Client: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			proof, err := provider.ReadCommitProof(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, commit)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected malformed proof rejection")
				}
				return
			}
			if err != nil || proof.SHA != commit || proof.Message != test.wantMessage || proof.AuthorID != "7" || len(proof.Parents) != 1 {
				t.Fatalf("proof=%#v err=%v", proof, err)
			}
		})
	}
}
