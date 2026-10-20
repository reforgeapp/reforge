package gitlab

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
)

type treeEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Mode string `json:"mode"`
	Path string `json:"path"`
}

func (p *Provider) tree(ctx context.Context, r forge.RepoRef, sha string) (map[string]treeEntry, error) {
	rows, err := p.pages(ctx, []string{"projects", r.NativeID, "repository", "tree"}, url.Values{"ref": {sha}, "recursive": {"true"}})
	if err != nil {
		return nil, err
	}
	if len(rows) > 10000 {
		return nil, failure("unsupported", "Repository tree exceeds bounded publication profile")
	}
	out := map[string]treeEntry{}
	for _, raw := range rows {
		var entry treeEntry
		if json.Unmarshal(raw, &entry) != nil || entry.Path == "" || !validSHA(entry.ID) {
			return nil, failure("provider", "Invalid repository tree")
		}
		if entry.Type == "tree" {
			continue
		}
		if _, ok := out[entry.Path]; ok {
			return nil, failure("provider", "Duplicate tree path")
		}
		out[entry.Path] = entry
	}
	return out, nil
}
func blobSHA(data []byte) string {
	hash := sha1.New()
	_, _ = fmt.Fprintf(hash, "blob %d\x00", len(data))
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}
func (p *Provider) mutate(ctx context.Context, method string, route []string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return failure("invalid", "Invalid mutation input")
	}
	status, headers, response, err := p.request(ctx, method, route, nil, &requestBody{data: raw})
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return responseError(status, headers)
	}
	if out != nil && decode(response, out) != nil {
		return uncertain(failure("provider", "Mutation response requires reconciliation"))
	}
	return nil
}
func (p *Provider) UpdateAppBranch(ctx context.Context, in forge.UpdateBranchRequest) (string, error) {
	if p.authorizeBranch == nil {
		return "", failure("unsupported", "Persisted branch authorization required")
	}
	if in.ExpectedOldSHA != "" {
		return "", failure("unsupported", "GitLab API lacks branch-wide expected-head comparison; publish a fresh companion branch")
	}
	if !safeID(in.Repository.NativeID) || !validSHA(in.BaseSHA) || !validBranch(in.Branch) || !strings.HasPrefix(in.Branch, "reforge/") || len(in.Branch) > 240 || len(in.Edits) == 0 || len(in.Edits) > 100 || in.Message == "" || len(in.Message) > 4096 {
		return "", failure("invalid", "Fresh branch requires immutable base and bounded edits")
	}
	if err := validOperationID(in.OperationID); err != nil {
		return "", err
	}
	if _, err := p.authenticatedActor(ctx); err != nil {
		return "", err
	}
	if _, err := p.GetRepository(ctx, in.Repository); err != nil {
		return "", err
	}
	before, err := p.tree(ctx, in.Repository, in.BaseSHA)
	if err != nil {
		return "", err
	}
	expected := map[string]treeEntry{}
	for path, entry := range before {
		expected[path] = entry
	}
	actions := []map[string]any{}
	seen := map[string]bool{}
	total := 0
	for _, edit := range in.Edits {
		total += len(edit.Content)
		if !validFilePath(edit.Path) || seen[edit.Path] || total > 2<<20 {
			return "", failure("invalid", "Invalid or excessive branch edit")
		}
		seen[edit.Path] = true
		old, exists := before[edit.Path]
		if exists && (old.Type != "blob" || (old.Mode != "100644" && old.Mode != "100755")) {
			return "", failure("unsupported", "Symlink and submodule edits are not allowed")
		}
		action := map[string]any{"file_path": edit.Path}
		if edit.Delete {
			if !exists {
				return "", failure("invalid", "Cannot delete missing file")
			}
			action["action"] = "delete"
			delete(expected, edit.Path)
		} else {
			action["action"] = "create"
			mode := "100644"
			if exists {
				action["action"] = "update"
				mode = old.Mode
			}
			action["encoding"] = "base64"
			action["content"] = base64.StdEncoding.EncodeToString(edit.Content)
			expected[edit.Path] = treeEntry{Path: edit.Path, ID: blobSHA(edit.Content), Type: "blob", Mode: mode}
		}
		actions = append(actions, action)
	}
	operationHash := sha256.Sum256([]byte(in.OperationID))
	stage := "reforge/staging/" + hex.EncodeToString(operationHash[:8]) + "-" + domain.NewID()
	var staged struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err = p.authorizeBranch(ctx, in); err != nil {
		return "", err
	}
	if err = p.mutate(ctx, "POST", []string{"projects", in.Repository.NativeID, "repository", "branches"}, map[string]any{"branch": stage, "ref": in.BaseSHA}, &staged); err != nil {
		return "", err
	}
	if staged.Name != stage || staged.Commit.ID != in.BaseSHA {
		return "", uncertain(failure("identity", "Staging branch identity changed"))
	}
	if err = p.authorizeBranch(ctx, in); err != nil {
		return "", err
	}
	var commit struct {
		ID      string   `json:"id"`
		Parents []string `json:"parent_ids"`
	}
	if err = p.mutate(ctx, "POST", []string{"projects", in.Repository.NativeID, "repository", "commits"}, map[string]any{"branch": stage, "commit_message": operationBody(in.Message, in.OperationID), "actions": actions, "force": false}, &commit); err != nil {
		return "", err
	}
	if !validSHA(commit.ID) || len(commit.Parents) != 1 || commit.Parents[0] != in.BaseSHA {
		return "", uncertain(failure("conflict", "Staged commit does not extend authorized base"))
	}
	after, err := p.tree(ctx, in.Repository, commit.ID)
	if err != nil {
		return "", uncertain(err)
	}
	if len(after) != len(expected) {
		return "", failure("conflict", "Staged tree differs from authorized edits")
	}
	for path, entry := range expected {
		if after[path] != entry {
			return "", failure("conflict", "Staged tree differs from authorized edits")
		}
	}
	if err = p.authorizeBranch(ctx, in); err != nil {
		return "", err
	}
	var published struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err = p.mutate(ctx, "POST", []string{"projects", in.Repository.NativeID, "repository", "branches"}, map[string]any{"branch": in.Branch, "ref": commit.ID}, &published); err != nil {
		return "", err
	}
	if published.Name != in.Branch || published.Commit.ID != commit.ID {
		return "", uncertain(failure("identity", "Published branch identity changed"))
	}
	observed, err := p.ResolveRef(ctx, in.Repository, "heads/"+in.Branch)
	if err != nil || observed != commit.ID {
		return "", uncertain(failure("conflict", "Published branch requires canonical reconciliation"))
	}
	return commit.ID, nil
}
