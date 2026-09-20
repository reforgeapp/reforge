package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reforge/internal/forge"
)

func TestSourceManifestNativeTreeTraversalAndTruncation(t *testing.T) {
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const root = "610d5853f83d074babbf53f28eac09a52fe96976"
	const subtree = "10731d0b170b98481a00bdca161e874e0ab93377"
	for _, name := range []string{"complete", "truncated", "missing-truncation", "wrong-commit", "missing-entry", "wrong-subtree", "unsafe-entry"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.RawQuery != "" {
					t.Error("source must use fixed nonrecursive read operations")
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/git/commits/"+commit):
					c := commit
					if name == "wrong-commit" {
						c = root
					}
					fmt.Fprintf(w, `{"sha":%q,"tree":{"sha":%q}}`, c, root)
				case strings.HasSuffix(r.URL.Path, "/git/trees/"+root):
					calls++
					trunc := `,"truncated":false`
					if name == "truncated" {
						trunc = `,"truncated":true`
					}
					if name == "missing-truncation" {
						trunc = ""
					}
					p := "d"
					if name == "unsafe-entry" {
						p = "../outside"
					}
					fmt.Fprintf(w, `{"sha":%q%s,"tree":[{"path":%q,"mode":"040000","type":"tree","sha":%q}]}`, root, trunc, p, subtree)
				case strings.HasSuffix(r.URL.Path, "/git/trees/"+subtree):
					calls++
					s := subtree
					if name == "wrong-subtree" {
						s = root
					}
					entries := `[{"path":"f","mode":"100644","type":"blob","sha":"ce013625030ba8dba906f756967f9e9ca394464a"}]`
					if name == "missing-entry" {
						entries = `[]`
					}
					fmt.Fprintf(w, `{"sha":%q,"truncated":false,"tree":%s}`, s, entries)
				default:
					io.WriteString(w, `{"id":1,"full_name":"org/repo"}`)
				}
			}))
			defer server.Close()
			p, _ := New(forge.Config{BaseURL: server.URL, Token: "test", Client: server.Client()})
			m, err := p.ReadSourceManifest(context.Background(), forge.RepoRef{NativeID: "1", FullName: "org/repo"}, commit)
			if name == "complete" {
				if err != nil || calls != 2 || len(m.Entries) != 2 || m.Entries[1].Path != "d/f" || m.Proof != "commit_tree_hash" {
					t.Fatalf("manifest=%+v error=%v", m, err)
				}
			} else if err == nil {
				t.Fatal("unverified source accepted")
			}
		})
	}
}
