package gitlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"reforge/internal/forge"
)

func TestSourceManifestPinnedRESTPaginationAndProof(t *testing.T) {
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, name := range []string{"complete", "missing-header", "loop", "wrong-total", "wrong-commit", "duplicate", "wrong-subtree", "unsafe-entry"} {
		t.Run(name, func(t *testing.T) {
			pages := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("unexpected mutation")
				}
				switch {
				case strings.Contains(r.URL.Path, "/repository/commits/"):
					id := commit
					if name == "wrong-commit" {
						id = strings.Repeat("b", 40)
					}
					fmt.Fprintf(w, `{"id":%q}`, id)
				case strings.HasSuffix(r.URL.Path, "/repository/tree"):
					pages++
					if r.URL.Query().Get("ref") != commit || r.URL.Query().Get("recursive") != "true" {
						t.Error("tree is not pinned")
					}
					page, _ := strconv.Atoi(r.URL.Query().Get("page"))
					w.Header().Set("X-Page", strconv.Itoa(page))
					w.Header().Set("X-Per-Page", "100")
					w.Header().Set("X-Total", "101")
					if name == "missing-header" {
						w.Header().Del("X-Page")
					}
					if name == "wrong-total" && page == 2 {
						w.Header().Set("X-Total", "102")
					}
					count := 100
					if page == 1 {
						w.Header().Set("X-Next-Page", "2")
						if name == "loop" {
							w.Header().Set("X-Next-Page", "1")
						}
					} else {
						count = 1
					}
					io.WriteString(w, "[")
					for i := 0; i < count; i++ {
						if i > 0 {
							io.WriteString(w, ",")
						}
						index := (page-1)*100 + i
						p := fmt.Sprintf("file-%03d", index)
						if name == "duplicate" && page == 2 {
							p = "file-000"
						}
						mode, kind, id := "100644", "blob", "ce013625030ba8dba906f756967f9e9ca394464a"
						if name == "unsafe-entry" && i == 0 {
							p = "../outside"
						}
						if name == "wrong-subtree" && page == 2 {
							p = "dir"
							mode = "040000"
							kind = "tree"
						}
						fmt.Fprintf(w, `{"path":%q,"id":%q,"mode":%q,"type":%q}`, p, id, mode, kind)
					}
					io.WriteString(w, "]")
				default:
					io.WriteString(w, `{"id":1,"path_with_namespace":"org/repo"}`)
				}
			}))
			defer server.Close()
			p, _ := New(forge.Config{BaseURL: server.URL, Token: "test", Client: server.Client()})
			m, err := p.ReadSourceManifest(context.Background(), forge.RepoRef{NativeID: "1", FullName: "org/repo"}, commit)
			if name == "complete" {
				if err != nil || pages != 2 || len(m.Entries) != 101 || m.Proof != "immutable_ref_api" || m.TreeSHA != "" {
					t.Fatalf("invalid source proof entries=%d error=%v", len(m.Entries), err)
				}
			} else if err == nil {
				t.Fatal("incomplete or unbound source accepted")
			}
		})
	}
}
