package artifact

import (
	"context"
	"github.com/reforgeapp/reforge/internal/domain"
	"io"
	"time"
)

type Metadata struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	RepositoryID string    `json:"repository_id"`
	TaskID       string    `json:"task_id"`
	Name         string    `json:"name"`
	MediaType    string    `json:"media_type"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type ArtifactStore interface {
	PutTenantArtifact(context.Context, Metadata, io.Reader) (Metadata, error)
	GetMetadata(context.Context, domain.Actor, string) (Metadata, error)
	AuthoriseDownload(context.Context, domain.Actor, string) (io.ReadCloser, error)
	DeleteByRetention(context.Context, string, time.Time) (int, error)
}
