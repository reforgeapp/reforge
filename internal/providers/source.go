package providers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

func (s *Service) SourceReader(org, connection string, authorize func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	return source.Reader{
		Manifest: func(ctx context.Context, repo forge.RepoRef, commit string) (forge.SourceManifest, error) {
			result, err := s.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeSourceManifest, Source: &privateconnector.ChecksArgs{Repository: repo, CommitSHA: commit}}, authorize)
			if err != nil {
				return forge.SourceManifest{}, err
			}
			if result.Manifest == nil {
				return forge.SourceManifest{}, privateconnector.ErrInvalid
			}
			return *result.Manifest, nil
		},
		File: func(ctx context.Context, repo forge.RepoRef, path, commit string) (forge.File, error) {
			result, err := s.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadFile, File: &privateconnector.FileArgs{Repository: repo, CommitSHA: commit, Path: path}}, authorize)
			if err != nil {
				return forge.File{}, err
			}
			if result.File == nil {
				return forge.File{}, privateconnector.ErrInvalid
			}
			return *result.File, nil
		},
	}
}
