package gitea

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

type pullBranch struct {
	Ref    string     `json:"ref"`
	SHA    string     `json:"sha"`
	Repo   repository `json:"repo"`
	RepoID int64      `json:"repo_id"`
}
type pull struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	URL       string     `json:"html_url"`
	User      user       `json:"user"`
	Head      pullBranch `json:"head"`
	Base      pullBranch `json:"base"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	Merged    bool       `json:"merged"`
	MergeSHA  string     `json:"merge_commit_sha"`
	Mergeable bool       `json:"mergeable"`
}

func (v pull) normalized(r forge.RepoRef) forge.Change {
	state := v.State
	status := "unknown"
	if v.Merged {
		state = "merged"
		status = "merged"
	} else if v.Mergeable {
		status = "mergeable"
	}
	return forge.Change{ID: strconv.FormatInt(v.Number, 10), Repository: r, HeadRepository: v.Head.Repo.ref(), TargetRepository: v.Base.Repo.ref(), Title: v.Title, Body: v.Body, URL: v.URL, HeadSHA: v.Head.SHA, TargetSHA: v.Base.SHA, HeadBranch: v.Head.Ref, TargetBranch: v.Base.Ref, AuthorID: strconv.FormatInt(v.User.ID, 10), AuthorLogin: v.User.Login, AuthorType: "user", State: state, Draft: v.Draft, MergeSHA: v.MergeSHA, MergeStatus: status}
}
func (p *Provider) loadPull(ctx context.Context, r forge.RepoRef, id string) (pull, error) {
	if !positive(id) {
		return pull{}, failure("invalid", "Pull request index required")
	}
	if _, e := p.repo(ctx, r); e != nil {
		return pull{}, e
	}
	route, _ := repoPath(r)
	var v pull
	e := p.request(ctx, "GET", route+"/pulls/"+id, nil, &v)
	if e == nil && (v.Number <= 0 || strconv.FormatInt(v.Number, 10) != id || v.Base.Repo.ref() != r || v.Head.Repo.ID <= 0 || v.Head.RepoID != v.Head.Repo.ID || v.Base.RepoID != v.Base.Repo.ID || !sha(v.Head.SHA) || !sha(v.Base.SHA)) {
		e = failure("identity", "Pull request source identity is incomplete or changed")
	}
	return v, e
}
func (p *Provider) ReadChange(ctx context.Context, r forge.RepoRef, id string) (forge.Change, error) {
	v, e := p.loadPull(ctx, r, id)
	if e != nil {
		return forge.Change{}, e
	}
	out := v.normalized(r)
	out.TargetSHA, e = p.ResolveRef(ctx, r, v.Base.Ref)
	if e != nil {
		return forge.Change{}, e
	}
	if out.State == "open" {
		current, e := p.ResolveRef(ctx, out.HeadRepository, out.HeadBranch)
		if e != nil {
			return forge.Change{}, e
		}
		if current != out.HeadSHA {
			return forge.Change{}, failure("conflict", "Pull request head is being refreshed")
		}
	}
	return out, nil
}
func (p *Provider) ReconcileChanges(ctx context.Context, r forge.RepoRef, cursor string) (domain.Page[forge.Change], error) {
	var out domain.Page[forge.Change]
	if _, e := p.repo(ctx, r); e != nil {
		return out, e
	}
	n, e := pageNumber(cursor)
	if e != nil {
		return out, e
	}
	route, _ := repoPath(r)
	var rows []pull
	e = p.request(ctx, "GET", route+"/pulls?state=all&limit=100&page="+strconv.Itoa(n), nil, &rows)
	if e != nil {
		return out, e
	}
	for _, v := range rows {
		if v.Base.Repo.ref() != r || v.Head.Repo.ID <= 0 || v.User.ID <= 0 {
			return out, failure("identity", "Incomplete pull request identity")
		}
		out.Items = append(out.Items, v.normalized(r))
	}
	out.Complete = len(rows) < 100
	if !out.Complete {
		if n == maxPages {
			return out, failure("pagination", "Pull request inventory exceeds limit")
		}
		out.NextCursor = strconv.Itoa(n + 1)
	}
	return out, nil
}
func (p *Provider) actor(ctx context.Context) (user, error) {
	var v user
	e := p.request(ctx, "GET", "/user", nil, &v)
	if e == nil && v.ID <= 0 {
		e = failure("identity", "Missing authenticated actor identity")
	}
	return v, e
}
func (p *Provider) ListBotWork(ctx context.Context, r forge.RepoRef) ([]forge.Change, error) {
	actor, e := p.actor(ctx)
	if e != nil {
		return nil, e
	}
	var out []forge.Change
	cursor := ""
	for {
		page, e := p.ReconcileChanges(ctx, r, cursor)
		if e != nil {
			return nil, e
		}
		for _, v := range page.Items {
			if v.AuthorID == strconv.FormatInt(actor.ID, 10) {
				v.AuthorType = "connection_actor"
				out = append(out, v)
			}
		}
		if page.Complete {
			return out, nil
		}
		cursor = page.NextCursor
	}
}
func operationMarker(id string) string { return "\n\n[reforge-operation:" + id + "]" }
func validOperation(id string) bool {
	if len(id) < 8 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func (p *Provider) FindChangeByOperation(ctx context.Context, r forge.RepoRef, op, branch, target string) (*forge.Change, error) {
	if !validOperation(op) || branch == "" || target == "" {
		return nil, failure("invalid", "Operation and exact source/target branches required")
	}
	rows, e := p.ListBotWork(ctx, r)
	if e != nil {
		return nil, e
	}
	var found *forge.Change
	for _, v := range rows {
		if v.HeadRepository == r && v.TargetRepository == r && v.HeadBranch == branch && v.TargetBranch == target && strings.HasSuffix(v.Body, operationMarker(op)) {
			if found != nil {
				return nil, failure("conflict", "Duplicate operation pull requests")
			}
			copy := v
			copy.OperationID = op
			found = &copy
		}
	}
	return found, nil
}
func (p *Provider) CreateChange(ctx context.Context, in forge.CreateChangeRequest) (forge.Change, error) {
	if in.Draft {
		return forge.Change{}, failure("unsupported", "Draft creation has not been certified")
	}
	if !validOperation(in.OperationID) || !sha(in.ExpectedHeadSHA) || in.HeadBranch == "" || in.TargetBranch == "" || in.Title == "" {
		return forge.Change{}, failure("invalid", "Change requires operation, branches, title and immutable head")
	}
	if existing, e := p.FindChangeByOperation(ctx, in.Repository, in.OperationID, in.HeadBranch, in.TargetBranch); e != nil {
		return forge.Change{}, e
	} else if existing != nil {
		if existing.HeadSHA != in.ExpectedHeadSHA {
			return forge.Change{}, failure("conflict", "Existing operation head changed")
		}
		return *existing, nil
	}
	head, e := p.ResolveRef(ctx, in.Repository, in.HeadBranch)
	if e != nil {
		return forge.Change{}, e
	}
	if head != in.ExpectedHeadSHA {
		return forge.Change{}, failure("conflict", "Change head moved")
	}
	route, _ := repoPath(in.Repository)
	var v pull
	e = p.request(ctx, "POST", route+"/pulls", map[string]any{"head": in.HeadBranch, "base": in.TargetBranch, "title": in.Title, "body": in.Body + operationMarker(in.OperationID), "allow_maintainer_edit": false}, &v)
	if e != nil {
		return forge.Change{}, e
	}
	out, e := p.ReadChange(ctx, in.Repository, strconv.FormatInt(v.Number, 10))
	if e != nil {
		return forge.Change{}, &domain.ProviderError{Kind: "uncertain", Message: "Created pull request could not be reconciled", Uncertain: true}
	}
	if out.HeadSHA != in.ExpectedHeadSHA || out.HeadRepository != in.Repository {
		return out, &domain.ProviderError{Kind: "uncertain", Message: "Created pull request head changed; reconcile its ownership", Uncertain: true}
	}
	out.OperationID = in.OperationID
	return out, nil
}
func (p *Provider) RequestReview(ctx context.Context, r forge.RepoRef, id string, reviewers []string) error {
	if _, e := p.loadPull(ctx, r, id); e != nil {
		return e
	}
	if len(reviewers) == 0 || len(reviewers) > 100 {
		return failure("invalid", "Bounded reviewer list required")
	}
	route, _ := repoPath(r)
	return p.request(ctx, "POST", route+"/pulls/"+id+"/requested_reviewers", map[string]any{"reviewers": reviewers}, nil)
}

type status struct {
	ID      int64  `json:"id"`
	Context string `json:"context"`
	Creator user   `json:"creator"`
	State   string `json:"status"`
	URL     string `json:"target_url"`
}

func (p *Provider) ListChecks(ctx context.Context, r forge.RepoRef, head string) ([]forge.Check, error) {
	if !sha(head) {
		return nil, failure("invalid", "Immutable status head required")
	}
	if _, e := p.repo(ctx, r); e != nil {
		return nil, e
	}
	route, _ := repoPath(r)
	var out []forge.Check
	seen := map[string]bool{}
	for n := 1; n <= maxPages; n++ {
		var rows []status
		if e := p.request(ctx, "GET", route+"/statuses/"+head+"?sort=highestindex&limit=100&page="+strconv.Itoa(n), nil, &rows); e != nil {
			return nil, e
		}
		for _, v := range rows {
			publisher := strconv.FormatInt(v.Creator.ID, 10)
			if v.ID <= 0 || v.Creator.ID <= 0 {
				return nil, failure("identity", "Status publisher identity missing")
			}
			key := v.Context + "\x00" + publisher
			if seen[key] {
				continue
			}
			seen[key] = true
			state := "completed"
			if v.State == "pending" {
				state = "pending"
			}
			out = append(out, forge.Check{ID: strconv.FormatInt(v.ID, 10), Name: v.Context, PublisherID: publisher, HeadSHA: head, Status: state, Conclusion: v.State, URL: v.URL})
		}
		if len(rows) < 100 {
			return out, nil
		}
	}
	return nil, failure("pagination", "Status inventory exceeds limit")
}

type treeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

func (p *Provider) tree(ctx context.Context, r forge.RepoRef, commit string) (map[string]treeEntry, error) {
	route, _ := repoPath(r)
	out := map[string]treeEntry{}
	seen := map[string]bool{}
	total := -1
	for n := 1; n <= maxPages; n++ {
		var v struct {
			Tree      []treeEntry `json:"tree"`
			Truncated bool        `json:"truncated"`
			Total     int         `json:"total_count"`
		}
		if e := p.request(ctx, "GET", route+"/git/trees/"+commit+"?recursive=true&per_page=1000&page="+strconv.Itoa(n), nil, &v); e != nil {
			return nil, e
		}
		if total < 0 {
			total = v.Total
		}
		if v.Total != total || total < 0 || total > 20000 {
			return nil, failure("response", "Inconsistent or excessive Git tree size")
		}
		for _, entry := range v.Tree {
			if seen[entry.Path] || !filePath(entry.Path) || !sha(entry.SHA) {
				return nil, failure("response", "Invalid or duplicate tree entry")
			}
			seen[entry.Path] = true
			if entry.Type != "tree" {
				out[entry.Path] = entry
			}
		}
		if !v.Truncated {
			if len(seen) != total {
				return nil, failure("response", "Incomplete Git tree")
			}
			return out, nil
		}
		if len(v.Tree) == 0 || len(seen) >= total {
			return nil, failure("response", "Inconsistent Git tree pagination")
		}
	}
	return nil, failure("pagination", "Git tree exceeds verification limit")
}
func blobSHA(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func (p *Provider) UpdateAppBranch(ctx context.Context, in forge.UpdateBranchRequest) (string, error) {
	if p.authorizeBranch == nil {
		return "", failure("forbidden", "Persisted application branch authorization required")
	}
	if !validOperation(in.OperationID) || in.Branch == "" || !sha(in.BaseSHA) || len(in.Edits) == 0 || len(in.Edits) > 100 || in.ExpectedOldSHA != "" && in.ExpectedOldSHA != in.BaseSHA {
		return "", failure("invalid", "Branch update requires immutable base and bounded append-only edits")
	}
	var version struct {
		Version string `json:"version"`
	}
	if e := p.request(ctx, "GET", "/version", nil, &version); e != nil {
		return "", e
	}
	if version.Version != "1.27.3" {
		return "", failure("unsupported", "Guarded branch writes require certified Gitea 1.27.3")
	}
	if e := p.authorizeBranch(ctx, in); e != nil {
		return "", e
	}
	if _, e := p.repo(ctx, in.Repository); e != nil {
		return "", e
	}
	if in.ExpectedOldSHA != "" {
		head, e := p.ResolveRef(ctx, in.Repository, in.Branch)
		if e != nil {
			return "", e
		}
		if head != in.ExpectedOldSHA {
			return "", failure("conflict", "Branch head moved")
		}
	}
	before, e := p.tree(ctx, in.Repository, in.BaseSHA)
	if e != nil {
		return "", e
	}
	expected := map[string]treeEntry{}
	for name, v := range before {
		expected[name] = v
	}
	files := []map[string]any{}
	seen := map[string]bool{}
	for _, edit := range in.Edits {
		if !filePath(edit.Path) || seen[edit.Path] || len(edit.Content) > maxBody {
			return "", failure("invalid", "Invalid or duplicate file edit")
		}
		seen[edit.Path] = true
		old, exists := before[edit.Path]
		if exists && (old.Type != "blob" || (old.Mode != "100644" && old.Mode != "100755")) {
			return "", failure("unsupported", "Cannot modify symlink or submodule")
		}
		entry := map[string]any{"path": edit.Path}
		if edit.Delete {
			if !exists {
				return "", failure("invalid", "Cannot delete missing file")
			}
			entry["operation"] = "delete"
			entry["sha"] = old.SHA
			delete(expected, edit.Path)
		} else {
			entry["operation"] = "create"
			mode := "100644"
			if exists {
				entry["operation"] = "update"
				entry["sha"] = old.SHA
				mode = old.Mode
			}
			entry["content"] = base64.StdEncoding.EncodeToString(edit.Content)
			expected[edit.Path] = treeEntry{Path: edit.Path, Type: "blob", Mode: mode, SHA: blobSHA(edit.Content)}
		}
		files = append(files, entry)
	}
	route, _ := repoPath(in.Repository)
	stage := "reforge-staging/" + in.OperationID + "-" + domain.NewID()
	if e := p.authorizeBranch(ctx, in); e != nil {
		return "", e
	}
	if e := p.request(ctx, "POST", route+"/branches", map[string]any{"new_branch_name": stage, "old_ref_name": in.BaseSHA}, nil); e != nil {
		return "", e
	}
	var changed struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if e := p.authorizeBranch(ctx, in); e != nil {
		return "", e
	}
	if e := p.request(ctx, "POST", route+"/contents", map[string]any{"branch": stage, "message": in.Message + operationMarker(in.OperationID), "files": files, "force_push": false}, &changed); e != nil {
		return "", e
	}
	head := changed.Commit.SHA
	if !sha(head) {
		return "", failure("response", "Missing staged commit identity")
	}
	var commit struct {
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	if e := p.request(ctx, "GET", route+"/git/commits/"+head+"?stat=false&verification=false", nil, &commit); e != nil {
		return "", e
	}
	if len(commit.Parents) != 1 || commit.Parents[0].SHA != in.BaseSHA {
		return "", failure("conflict", "Staged commit does not directly extend authorized base")
	}
	after, e := p.tree(ctx, in.Repository, head)
	if e != nil {
		return "", e
	}
	if len(after) != len(expected) {
		return "", failure("conflict", "Staged tree differs from authorized edits")
	}
	for name, v := range expected {
		if after[name] != v {
			return "", failure("conflict", "Staged tree differs from authorized edits")
		}
	}
	if e := p.authorizeBranch(ctx, in); e != nil {
		return "", e
	}
	if in.ExpectedOldSHA == "" {
		e = p.request(ctx, "POST", route+"/branches", map[string]any{"new_branch_name": in.Branch, "old_ref_name": head}, nil)
	} else {
		e = p.request(ctx, "PUT", route+"/branches/"+url.PathEscape(in.Branch), map[string]any{"old_commit_id": in.ExpectedOldSHA, "new_commit_id": head, "force": false}, nil)
	}
	if e != nil {
		return "", e
	}
	return head, nil
}
