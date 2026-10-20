package gitea

import (
	"context"
	"strconv"
	"unicode/utf8"

	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/source"
)

const maxCommitMessage = 64 << 10

func (p *Provider) ReadCommitProof(ctx context.Context, r forge.RepoRef, sha string) (forge.CommitProof, error) {
	if !positive(r.NativeID) || !source.ValidSHA(sha, "sha1") {
		return forge.CommitProof{}, failure("invalid", "Immutable repository and qualified SHA-1 commit required")
	}
	if _, err := p.repo(ctx, r); err != nil {
		return forge.CommitProof{}, err
	}
	route, _ := repoPath(r)
	var raw struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
		Author *struct {
			ID int64 `json:"id"`
		} `json:"author"`
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	if err := p.request(ctx, "GET", route+"/git/commits/"+sha+"?stat=false&verification=false&files=false", nil, &raw); err != nil {
		return forge.CommitProof{}, err
	}
	authorID := ""
	if raw.Author != nil && raw.Author.ID > 0 {
		authorID = strconv.FormatInt(raw.Author.ID, 10)
	}
	var parents []string
	if raw.Parents != nil {
		parents = make([]string, 0, len(raw.Parents))
		for _, parent := range raw.Parents {
			parents = append(parents, parent.SHA)
		}
	}
	return validateCommitProof(sha, raw.SHA, parents, raw.Commit.Message, authorID)
}

func validateCommitProof(want, got string, parents []string, message, authorID string) (forge.CommitProof, error) {
	if got != want {
		return forge.CommitProof{}, failure("response", "Commit identity does not match the pinned SHA")
	}
	if len(parents) > 64 {
		return forge.CommitProof{}, failure("response", "Commit parent list exceeds the bounded profile")
	}
	seen := make(map[string]bool, len(parents))
	for _, parent := range parents {
		if !source.ValidSHA(parent, "sha1") || seen[parent] {
			return forge.CommitProof{}, failure("response", "Commit parents are incomplete or invalid")
		}
		seen[parent] = true
	}
	if parents == nil || !utf8.ValidString(message) || len(message) > maxCommitMessage {
		return forge.CommitProof{}, failure("response", "Commit message or parent proof is incomplete")
	}
	return forge.CommitProof{SHA: got, Parents: parents, Message: message, AuthorID: authorID}, nil
}
