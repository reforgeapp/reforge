package gitea

import (
	"context"
	"strconv"

	"reforge/internal/forge"
	"reforge/internal/source"
)

func (p *Provider) ReadSourceManifest(ctx context.Context, r forge.RepoRef, commit string) (forge.SourceManifest, error) {
	var out forge.SourceManifest
	if !positive(r.NativeID) || !source.ValidSHA(commit, "sha1") {
		return out, failure("invalid", "Immutable repository and qualified SHA-1 commit required")
	}
	if _, err := p.repo(ctx, r); err != nil {
		return out, err
	}
	route, _ := repoPath(r)
	var c struct {
		SHA    string `json:"sha"`
		Commit struct {
			Tree struct {
				SHA string `json:"sha"`
			} `json:"tree"`
		} `json:"commit"`
	}
	if err := p.request(ctx, "GET", route+"/git/commits/"+commit+"?stat=false&verification=false&files=false", nil, &c); err != nil {
		return out, err
	}
	if c.SHA != commit || c.Commit.Tree.SHA != commit {
		return out, failure("response", "Missing immutable commit tree binding")
	}
	out = forge.SourceManifest{Repository: r, CommitSHA: commit, ObjectFormat: "sha1", Proof: "immutable_ref_api"}
	total := -1
	seen := map[string]bool{}
	for page := 1; page <= 101; page++ {
		var v struct {
			SHA       string              `json:"sha"`
			Page      *int                `json:"page"`
			Total     *int                `json:"total_count"`
			Truncated *bool               `json:"truncated"`
			Tree      []forge.SourceEntry `json:"tree"`
		}
		if err := p.request(ctx, "GET", route+"/git/trees/"+commit+"?recursive=true&per_page=100&page="+strconv.Itoa(page), nil, &v); err != nil {
			return forge.SourceManifest{}, err
		}
		if v.SHA != commit || v.Page == nil || *v.Page != page || v.Total == nil || *v.Total < 0 || *v.Total > source.MaxEntries || v.Truncated == nil {
			return forge.SourceManifest{}, failure("response", "Invalid source tree pagination or identity")
		}
		if total < 0 {
			total = *v.Total
		}
		if *v.Total != total || len(v.Tree) > 100 || len(out.Entries)+len(v.Tree) > total {
			return forge.SourceManifest{}, failure("response", "Inconsistent source tree pagination")
		}
		for _, e := range v.Tree {
			if !source.ValidEntry(e, "sha1") || seen[e.Path] {
				return forge.SourceManifest{}, failure("response", "Invalid or duplicate source entry")
			}
			seen[e.Path] = true
		}
		out.Entries = append(out.Entries, v.Tree...)
		if !*v.Truncated {
			if len(out.Entries) != total {
				return forge.SourceManifest{}, failure("response", "Incomplete source tree")
			}
			out.Complete = true
			if _, err := source.ValidateManifest(ctx, out); err != nil {
				return forge.SourceManifest{}, err
			}
			return out, nil
		}
		if len(v.Tree) == 0 || len(out.Entries) >= total {
			return forge.SourceManifest{}, failure("response", "Source pagination made no progress")
		}
	}
	return forge.SourceManifest{}, failure("response", "Source tree exceeds pagination limit")
}
