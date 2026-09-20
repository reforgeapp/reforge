package gitlab

import (
	"context"
	"net/url"
	"strconv"

	"reforge/internal/forge"
	"reforge/internal/source"
)

func (p *Provider) ReadSourceManifest(ctx context.Context, r forge.RepoRef, commit string) (forge.SourceManifest, error) {
	var out forge.SourceManifest
	if !safeID(r.NativeID) || !source.ValidSHA(commit, "sha1") {
		return out, failure("invalid", "Immutable repository and qualified SHA-1 commit required")
	}
	repo, err := p.GetRepository(ctx, r)
	if err != nil {
		return out, err
	}
	if repo.RepoRef != r {
		return out, failure("identity", "Repository identity changed; refresh inventory")
	}
	route := []string{"projects", r.NativeID, "repository"}
	status, headers, body, err := p.request(ctx, "GET", append(route, "commits", commit), nil, nil)
	if err != nil {
		return out, err
	}
	if status != 200 {
		return out, responseError(status, headers)
	}
	var c struct {
		ID string `json:"id"`
	}
	if err = decode(body, &c); err != nil {
		return out, err
	}
	if c.ID != commit {
		return out, failure("response", "Missing immutable commit binding")
	}
	out = forge.SourceManifest{Repository: r, CommitSHA: commit, ObjectFormat: "sha1", Proof: "immutable_ref_api"}
	total := -1
	seen := map[string]bool{}
	for page := 1; page <= 101; page++ {
		status, headers, body, err = p.request(ctx, "GET", append(route, "tree"), url.Values{"ref": {commit}, "recursive": {"true"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}, nil)
		if err != nil {
			return forge.SourceManifest{}, err
		}
		if status != 200 {
			return forge.SourceManifest{}, responseError(status, headers)
		}
		var rows []treeEntry
		if err = decode(body, &rows); err != nil {
			return forge.SourceManifest{}, err
		}
		if rows == nil || len(rows) > 100 || len(out.Entries)+len(rows) > source.MaxEntries || headers.Get("X-Page") != strconv.Itoa(page) || headers.Get("X-Per-Page") != "100" {
			return forge.SourceManifest{}, failure("response", "Incomplete or oversized source tree pagination")
		}
		if raw := headers.Get("X-Total"); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 0 || n > source.MaxEntries || total >= 0 && n != total {
				return forge.SourceManifest{}, failure("response", "Inconsistent source tree count")
			}
			total = n
		}
		for _, e := range rows {
			entry := forge.SourceEntry{Path: e.Path, SHA: e.ID, Mode: e.Mode, Type: e.Type}
			if !source.ValidEntry(entry, "sha1") || seen[entry.Path] {
				return forge.SourceManifest{}, failure("response", "Invalid or duplicate source entry")
			}
			seen[entry.Path] = true
			out.Entries = append(out.Entries, entry)
		}
		next, e := nextPage(headers, page)
		if e != nil {
			return forge.SourceManifest{}, e
		}
		if next == "" {
			if len(rows) == 100 && (total < 0 || len(out.Entries) != total) || total >= 0 && len(out.Entries) != total {
				return forge.SourceManifest{}, failure("response", "Unverified final source tree page")
			}
			out.Complete = true
			if _, err = source.ValidateManifest(ctx, out); err != nil {
				return forge.SourceManifest{}, err
			}
			return out, nil
		}
		if len(rows) != 100 {
			return forge.SourceManifest{}, failure("response", "Incomplete intermediate source tree page")
		}
	}
	return forge.SourceManifest{}, failure("response", "Source tree exceeds pagination limit")
}
