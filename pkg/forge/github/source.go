package github

import (
	"context"
	"net/url"

	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/source"
)

func (p *Provider) ReadSourceManifest(ctx context.Context, r forge.RepoRef, commit string) (forge.SourceManifest, error) {
	var out forge.SourceManifest
	if !positive(r.NativeID) || !source.ValidSHA(commit, "sha1") {
		return out, failure("invalid", "Immutable repository and qualified SHA-1 commit required")
	}
	repo, err := p.GetRepository(ctx, r)
	if err != nil {
		return out, err
	}
	if repo.RepoRef != r {
		return out, failure("identity", "Repository identity changed; refresh inventory")
	}
	route, _ := repositoryPath(r)
	var c struct {
		SHA  string `json:"sha"`
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err = p.get(ctx, append(route, "git", "commits", commit), nil, &c); err != nil {
		return out, err
	}
	if c.SHA != commit || !source.ValidSHA(c.Tree.SHA, "sha1") {
		return out, failure("response", "Missing immutable commit tree binding")
	}
	out = forge.SourceManifest{Repository: r, CommitSHA: commit, TreeSHA: c.Tree.SHA, ObjectFormat: "sha1", Proof: "commit_tree_hash"}
	var tree struct {
		SHA       string              `json:"sha"`
		Tree      []forge.SourceEntry `json:"tree"`
		Truncated *bool               `json:"truncated"`
	}
	if err = p.get(ctx, append(route, "git", "trees", c.Tree.SHA), url.Values{"recursive": {"1"}}, &tree); err != nil {
		return forge.SourceManifest{}, err
	}
	if tree.SHA != c.Tree.SHA || tree.Truncated == nil || *tree.Truncated || len(tree.Tree) > source.MaxEntries {
		return forge.SourceManifest{}, failure("response", "Incomplete or oversized source tree")
	}
	seen := map[string]bool{}
	for _, e := range tree.Tree {
		if !source.ValidEntry(e, "sha1") || seen[e.Path] {
			return forge.SourceManifest{}, failure("response", "Invalid or duplicate source entry")
		}
		seen[e.Path] = true
		out.Entries = append(out.Entries, e)
	}
	out.Complete = true
	if _, err = source.ValidateManifest(ctx, out); err != nil {
		return forge.SourceManifest{}, err
	}
	return out, nil
}
