package gitops

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/source"
	"strings"
)

func (s *Service) verifyCandidate(ctx context.Context, org string, g Gate, p Promotion, check func(context.Context, pgx.Tx, connections.Connection) error) error {
	if !source.ValidSHA(p.CandidateSHA, "sha1") {
		return auth.ErrConflict
	}
	var stageID string
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT stage_dispatch_id::text FROM gitops_promotions WHERE org_id=$1 AND id=$2`, org, p.ID).Scan(&stageID)
	})
	if err != nil {
		return err
	}
	proof, err := s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeCommitProof, Commit: &privateconnector.ChecksArgs{Repository: g.Delivery, CommitSHA: p.CandidateSHA}}, check)
	if err != nil {
		return err
	}
	if proof.Commit == nil || proof.Commit.SHA != p.CandidateSHA || len(proof.Commit.Parents) != 1 || proof.Commit.Parents[0] != g.TargetSHA {
		return auth.ErrConflict
	}
	message := strings.TrimSpace(proof.Commit.Message)
	if !strings.HasSuffix(message, "[reforge-operation:"+stageID+"]") && !strings.HasSuffix(message, "<!-- reforge-operation-id:"+stageID+" -->") {
		return auth.ErrConflict
	}
	manifests := []forge.SourceManifest{}
	for _, sha := range []string{g.TargetSHA, p.CandidateSHA} {
		result, err := s.providers.Read(ctx, org, g.DeliveryConnectionID, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeSourceManifest, Source: &privateconnector.ChecksArgs{Repository: g.Delivery, CommitSHA: sha}}, check)
		if err != nil {
			return err
		}
		if result.Manifest == nil || result.Manifest.Repository != g.Delivery || result.Manifest.CommitSHA != sha || result.Manifest.ObjectFormat != "sha1" {
			return auth.ErrConflict
		}
		if _, err = source.ValidateManifest(ctx, *result.Manifest); err != nil {
			return err
		}
		manifests = append(manifests, *result.Manifest)
	}
	left := map[string]forge.SourceEntry{}
	for _, e := range manifests[0].Entries {
		left[e.Path] = e
	}
	if len(left) != len(manifests[1].Entries) {
		return auth.ErrConflict
	}
	patched := false
	for _, entry := range manifests[1].Entries {
		before, ok := left[entry.Path]
		if !ok || before.Mode != entry.Mode || before.Type != entry.Type {
			return auth.ErrConflict
		}
		if entry.Path == g.Configuration.ManifestPath {
			if entry.Type != "blob" || !source.VerifyBlob("sha1", entry.SHA, g.PatchedManifest) {
				return auth.ErrConflict
			}
			patched = true
			continue
		}
		if entry.Type == "blob" && entry.SHA != before.SHA {
			return auth.ErrConflict
		}
	}
	if !patched {
		return auth.ErrConflict
	}
	return nil
}
