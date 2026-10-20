package gitea

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/source"
)

func TestSourceManifestRejectsWrongCommitAndPagination(t *testing.T) {
	const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const tree = commit
	for _, name := range []string{"valid-empty", "wrong-commit", "wrong-tree", "missing-page", "wrong-page", "incomplete", "truncated-empty"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/git/commits/"+commit):
					c := commit
					if name == "wrong-commit" {
						c = strings.Repeat("b", 40)
					}
					fmt.Fprintf(w, `{"sha":%q,"commit":{"tree":{"sha":%q}}}`, c, tree)
				case strings.Contains(r.URL.Path, "/git/trees/"):
					v := map[string]any{"sha": tree, "page": 1, "total_count": 0, "truncated": false, "tree": []any{}}
					switch name {
					case "wrong-tree":
						v["sha"] = strings.Repeat("b", 40)
					case "missing-page":
						delete(v, "page")
					case "wrong-page":
						v["page"] = 2
					case "incomplete":
						v["total_count"] = 1
					case "truncated-empty":
						v["truncated"] = true
						v["total_count"] = 1
					}
					json.NewEncoder(w).Encode(v)
				default:
					io.WriteString(w, `{"id":1,"full_name":"org/repo"}`)
				}
			}))
			defer server.Close()
			p, _ := New(forge.Config{BaseURL: server.URL, Token: "test", Client: server.Client()})
			m, err := p.ReadSourceManifest(context.Background(), forge.RepoRef{NativeID: "1", FullName: "org/repo"}, commit)
			if name == "valid-empty" {
				if err != nil || !m.Complete {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid native source proof accepted")
			}
		})
	}
}

func TestRealGiteaPinnedSourcePaginationAndBranchMovement(t *testing.T) {
	if os.Getenv("REFORGE_GITEA_TEST") != "1" {
		t.Skip("requires disposable local Gitea 1.27.3 fixture")
	}
	root := os.Getenv("REFORGE_TEST_ROOT")
	if root == "" {
		t.Fatal("REFORGE_TEST_ROOT required")
	}
	credential, err := os.ReadFile(filepath.Join(root, ".local/gitea/reforge-admin.token"))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := "http://127.0.0.1:53000"
	call := func(method, route string, input, output any) int {
		t.Helper()
		body, _ := json.Marshal(input)
		req, e := http.NewRequest(method, base+"/api/v1"+route, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "token "+strings.TrimSpace(string(credential)))
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		if output != nil && resp.StatusCode < 300 {
			if e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(output); e != nil {
				t.Fatal(e)
			}
		}
		return resp.StatusCode
	}
	var version struct{ Version string }
	if call("GET", "/version", nil, &version) != 200 || version.Version != "1.27.3" {
		t.Fatal("pinned Gitea 1.27.3 required")
	}
	var repo struct {
		ID       int64
		FullName string `json:"full_name"`
	}
	if call("POST", "/user/repos", map[string]any{"name": "t07-source-" + domain.NewID(), "default_branch": "main", "auto_init": false, "private": true}, &repo) != 201 {
		t.Fatal("create disposable repository failed")
	}
	route := "/repos/" + repo.FullName
	t.Cleanup(func() {
		if call("DELETE", route, nil, nil) != 204 {
			t.Error("disposable source fixture cleanup failed")
		}
	})
	files := []map[string]string{}
	for i := 0; i < 105; i++ {
		files = append(files, map[string]string{"operation": "create", "path": fmt.Sprintf("dir/file-%03d", i), "content": base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("fixture %d\n", i)))})
	}
	var result struct{ Commit struct{ SHA string } }
	status := call("POST", route+"/contents", map[string]any{"branch": "main", "message": "Source pagination fixture", "files": files}, &result)
	if status != 201 {
		t.Fatalf("fixture files HTTP %d", status)
	}
	p, e := New(forge.Config{BaseURL: base, Token: strings.TrimSpace(string(credential)), Client: client})
	if e != nil {
		t.Fatal(e)
	}
	ref := forge.RepoRef{NativeID: fmt.Sprint(repo.ID), FullName: repo.FullName}
	commit := result.Commit.SHA
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m, e := p.ReadSourceManifest(ctx, ref, commit)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Entries) != 106 || m.TreeSHA != "" || m.Proof != "immutable_ref_api" || !m.Complete {
		t.Fatalf("invalid manifest count=%d complete=%v", len(m.Entries), m.Complete)
	}
	if call("POST", route+"/contents/new-file", map[string]any{"branch": "main", "message": "Move main after source pin", "content": base64.StdEncoding.EncodeToString([]byte("later\n"))}, nil) != 201 {
		t.Fatal("move fixture branch failed")
	}
	snapshot, e := source.Fetch(ctx, source.Reader{Manifest: p.ReadSourceManifest, File: p.ReadFileAtRef}, ref, commit)
	if e != nil || !snapshot.Complete || len(snapshot.Files) != 105 || snapshot.CommitSHA != commit {
		t.Fatalf("pinned source failed files=%d error=%v", len(snapshot.Files), e)
	}
	for i, f := range snapshot.Files {
		if string(f.Content) != fmt.Sprintf("fixture %d\n", i) {
			t.Fatal("source content changed after branch movement")
		}
	}
	ref.NativeID = "999999999"
	if _, e = p.ReadSourceManifest(ctx, ref, commit); e == nil {
		t.Fatal("replacement repository accepted")
	}
}
