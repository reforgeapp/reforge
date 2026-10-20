package gitops

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/source"
)

func (s *Service) manifestFile(ctx context.Context, org string, g Gate, sha string, check func(context.Context, pgx.Tx, connections.Connection) error) ([]byte, error) {
	result, err := s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeSourceManifest, Source: &privateconnector.ChecksArgs{Repository: g.Delivery, CommitSHA: sha}}, check)
	if err != nil {
		return nil, err
	}
	if result.Manifest == nil || result.Manifest.Repository != g.Delivery || result.Manifest.CommitSHA != sha {
		return nil, auth.ErrConflict
	}
	manifest := *result.Manifest
	if _, err = source.ValidateManifest(ctx, manifest); err != nil {
		return nil, err
	}
	blob := ""
	for _, entry := range manifest.Entries {
		if entry.Path == g.Configuration.ManifestPath {
			if blob != "" || entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
				return nil, auth.ErrConflict
			}
			blob = entry.SHA
		}
	}
	if blob == "" {
		return nil, auth.ErrInvalid
	}
	result, err = s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadFile, File: &privateconnector.FileArgs{Repository: g.Delivery, CommitSHA: sha, Path: g.Configuration.ManifestPath}}, check)
	if err != nil {
		return nil, err
	}
	if result.File == nil || result.File.Path != g.Configuration.ManifestPath || !source.VerifyBlob(manifest.ObjectFormat, blob, result.File.Content) {
		return nil, auth.ErrConflict
	}
	return result.File.Content, nil
}
