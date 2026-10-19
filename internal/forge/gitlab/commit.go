package gitlab

import (
	"context"
	"strconv"
	"unicode/utf8"

	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/source"
)

const maxCommitMessage = 64 << 10

func (p *Provider) ReadCommitProof(ctx context.Context, r forge.RepoRef, sha string) (forge.CommitProof, error) {
	if !safeID(r.NativeID) || !source.ValidSHA(sha, "sha1") {
		return forge.CommitProof{}, failure("invalid", "Immutable project and qualified SHA-1 commit required")
	}
	if _, err := p.GetRepository(ctx, r); err != nil {
		return forge.CommitProof{}, err
	}
	var raw struct {
		ID        string   `json:"id"`
		Message   string   `json:"message"`
		ParentIDs []string `json:"parent_ids"`
		AuthorID  int64    `json:"author_id"`
		Author    *struct {
			ID int64 `json:"id"`
		} `json:"author"`
	}
	if err := p.read(ctx, []string{"projects", r.NativeID, "repository", "commits", sha}, &raw); err != nil {
		return forge.CommitProof{}, err
	}
	authorID := ""
	if raw.AuthorID > 0 {
		authorID = strconv.FormatInt(raw.AuthorID, 10)
	} else if raw.Author != nil && raw.Author.ID > 0 {
		authorID = strconv.FormatInt(raw.Author.ID, 10)
	}
	return validateCommitProof(sha, raw.ID, raw.ParentIDs, raw.Message, authorID)
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
