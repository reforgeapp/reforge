package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/forge/gitea"
)

func TestNativeCommitProofAndPublication(t *testing.T) {
	if os.Getenv("REFORGE_GITEA_TEST") != "1" {
		t.Skip("set REFORGE_GITEA_TEST=1 for disposable pinned local Gitea proof test")
	}
	root := os.Getenv("REFORGE_TEST_ROOT")
	if root == "" {
		t.Skip("REFORGE_TEST_ROOT is absent")
	}
	tokens := map[string]string{}
	for _, actor := range []string{"admin", "bot"} {
		raw, err := os.ReadFile(filepath.Join(root, ".local/gitea/reforge-"+actor+".token"))
		if err != nil || strings.TrimSpace(string(raw)) == "" {
			t.Skip("local Gitea credentials are absent")
		}
		tokens[actor] = strings.TrimSpace(string(raw))
	}
	base := "http://127.0.0.1:53000"
	client := &http.Client{Timeout: 15 * time.Second}
	call := func(actor, method, route string, input, output any) int {
		t.Helper()
		var body []byte
		if input != nil {
			body, _ = json.Marshal(input)
		}
		req, err := http.NewRequestWithContext(context.Background(), method, base+"/api/v1"+route, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "token "+tokens[actor])
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") {
				t.Skip("local Gitea is absent")
			}
			t.Fatal(err)
		}
		defer response.Body.Close()
		if output != nil && response.StatusCode < 300 {
			if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(output); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode
	}
	var version struct{ Version string }
	if call("bot", "GET", "/version", nil, &version) != http.StatusOK {
		t.Skip("local Gitea is absent or unavailable")
	}
	if version.Version != "1.27.3" {
		t.Fatalf("pinned Gitea 1.27.3 required, got %s", version.Version)
	}
	var repo struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if status := call("admin", "POST", "/user/repos", map[string]any{"name": "t19-proof-" + domain.NewID(), "default_branch": "main", "private": true, "auto_init": false}, &repo); status != http.StatusCreated {
		t.Fatalf("repository creation HTTP %d", status)
	}
	route := "/repos/" + repo.FullName
	t.Cleanup(func() {
		if status := call("admin", "DELETE", route, nil, nil); status != http.StatusNoContent {
			t.Errorf("disposable repository cleanup HTTP %d", status)
		}
	})
	if status := call("admin", "PUT", route+"/collaborators/reforge-bot", map[string]string{"permission": "write"}, nil); status < 200 || status >= 300 {
		t.Fatalf("bot collaborator HTTP %d", status)
	}
	var created struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if status := call("admin", "POST", route+"/contents/proof.txt", map[string]any{"branch": "main", "message": "baseline", "content": base64.StdEncoding.EncodeToString([]byte("baseline\n"))}, &created); status != http.StatusCreated {
		t.Fatalf("baseline creation HTTP %d", status)
	}
	baseline := created.Commit.SHA
	if len(baseline) != 40 {
		t.Fatalf("invalid baseline SHA")
	}
	repoRef := forge.RepoRef{NativeID: fmt.Sprint(repo.ID), FullName: repo.FullName}
	provider, err := gitea.New(forge.Config{BaseURL: base, Client: client, Token: tokens["bot"]})
	if err != nil {
		t.Fatal(err)
	}
	op := domain.NewID()
	branch := "reforge/" + op
	update := forge.UpdateBranchRequest{Repository: repoRef, Branch: branch, BaseSHA: baseline, Message: "source-only candidate", OperationID: op, Edits: []forge.FileEdit{{Path: "proof.txt", Content: []byte("candidate\n")}}}
	if _, err := provider.UpdateAppBranch(context.Background(), update); err == nil {
		t.Fatal("unguarded provider accepted branch update")
	}
	provider = provider.WithBranchAuthorizer(func(_ context.Context, in forge.UpdateBranchRequest) error {
		if in.Repository != repoRef || !strings.HasPrefix(in.Branch, "reforge/") {
			return fmt.Errorf("unauthorized branch")
		}
		return nil
	})
	head, err := provider.UpdateAppBranch(context.Background(), update)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := provider.ReadCommitProof(context.Background(), repoRef, head)
	if err != nil || proof.SHA != head || len(proof.Parents) != 1 || proof.Parents[0] != baseline || !strings.Contains(proof.Message, "[reforge-operation:"+op+"]") {
		t.Fatalf("commit proof=%+v error=%v", proof, err)
	}
	manifest, err := provider.ReadSourceManifest(context.Background(), repoRef, head)
	if err != nil || !manifest.Complete || manifest.CommitSHA != head || manifest.Proof != "immutable_ref_api" {
		t.Fatalf("source manifest=%+v error=%v", manifest, err)
	}
	changeInput := forge.CreateChangeRequest{Repository: repoRef, Title: "Native proof candidate", Body: "T19 proof", HeadBranch: branch, TargetBranch: "main", ExpectedHeadSHA: head, OperationID: op}
	first, err := provider.CreateChange(context.Background(), changeInput)
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.CreateChange(context.Background(), changeInput)
	if err != nil || second.ID != first.ID {
		t.Fatalf("idempotent publication first=%+v second=%+v error=%v", first, second, err)
	}
	found, err := provider.FindChangeByOperation(context.Background(), repoRef, op, branch, "main")
	if err != nil || found == nil || found.ID != first.ID || found.HeadSHA != head || found.TargetSHA != baseline {
		t.Fatalf("operation reconciliation change=%+v error=%v", found, err)
	}
}
