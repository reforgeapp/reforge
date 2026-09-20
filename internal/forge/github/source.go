package github

import (
	"context"
	"strings"

	"reforge/internal/forge"
	"reforge/internal/source"
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
	type pending struct{ path, sha string }
	seen := map[string]bool{}
	queue := []pending{{"", c.Tree.SHA}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		var tree struct {
			SHA       string              `json:"sha"`
			Tree      []forge.SourceEntry `json:"tree"`
			Truncated *bool               `json:"truncated"`
		}
		if err = p.get(ctx, append(route, "git", "trees", current.sha), nil, &tree); err != nil {
			return forge.SourceManifest{}, err
		}
		if tree.SHA != current.sha || tree.Truncated == nil || *tree.Truncated || len(out.Entries)+len(tree.Tree) > source.MaxEntries {
			return forge.SourceManifest{}, failure("response", "Incomplete or oversized source tree")
		}
		for _, e := range tree.Tree {
			if e.Path == "" || strings.Contains(e.Path, "/") {
				return forge.SourceManifest{}, failure("response", "Invalid nonrecursive tree entry")
			}
			if current.path != "" {
				e.Path = current.path + "/" + e.Path
			}
			if !source.ValidEntry(e, "sha1") || seen[e.Path] {
				return forge.SourceManifest{}, failure("response", "Invalid or duplicate source entry")
			}
			seen[e.Path] = true
			if e.Type == "tree" {
				queue = append(queue, pending{e.Path, e.SHA})
			}
			out.Entries = append(out.Entries, e)
		}
	}
	out.Complete = true
	if _, err = source.ValidateManifest(ctx, out); err != nil {
		return forge.SourceManifest{}, err
	}
	return out, nil
}
