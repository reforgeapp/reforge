package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/release"
	"github.com/reforgeapp/reforge/pkg/source"
)

type Workflow struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type Path struct {
	Kind        string   `json:"kind"`
	Environment string   `json:"environment"`
	Ready       bool     `json:"ready"`
	Blockers    []string `json:"blockers"`
}
type Setup struct {
	Workflows []Workflow `json:"workflows"`
	Paths     []Path     `json:"paths"`
	Blockers  []string   `json:"blockers"`
}

func (s *Service) Discover(ctx context.Context, session auth.Session, org, repo string) (Setup, error) {
	out := Setup{Workflows: []Workflow{}, Paths: []Path{}, Blockers: []string{}}
	if !auth.ValidID(repo) {
		return out, auth.ErrInvalid
	}
	if s.Providers == nil {
		out.Blockers = append(out.Blockers, "Operator: restore the code host connection.")
		return out, nil
	}
	var ref forge.RepoRef
	var connection, branch, provider, server string
	var version int64
	err := s.Releases.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		return tx.QueryRow(ctx, `SELECT r.native_id,r.name,r.connection_id::text,r.default_branch,c.version,c.provider,c.server_version FROM repositories r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2 AND r.accessible AND NOT r.archived AND c.state='healthy'`, org, repo).Scan(&ref.NativeID, &ref.FullName, &connection, &branch, &version, &provider, &server)
	})
	if err != nil {
		return out, err
	}
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		if c.ID != connection || c.Version != version || c.State != "healthy" {
			return auth.ErrConflict
		}
		a, err := s.Releases.Auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		var matches bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE org_id=$1 AND id=$2 AND native_id=$3 AND name=$4 AND connection_id=$5 AND default_branch=$6 AND accessible AND NOT archived)`, org, repo, ref.NativeID, ref.FullName, connection, branch).Scan(&matches); err != nil {
			return err
		}
		if !matches {
			return auth.ErrConflict
		}
		return nil
	}
	head, err := s.Providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: ref, Ref: branch}}, check)
	if err != nil {
		return out, err
	}
	manifest, err := s.Providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeSourceManifest, Source: &privateconnector.ChecksArgs{Repository: ref, CommitSHA: head.SHA}}, check)
	if err != nil {
		return out, err
	}
	if manifest.Manifest == nil || manifest.Manifest.Repository != ref || manifest.Manifest.CommitSHA != head.SHA {
		return out, auth.ErrConflict
	}
	if _, err = source.ValidateManifest(ctx, *manifest.Manifest); err != nil {
		return out, err
	}
	for _, entry := range manifest.Manifest.Entries {
		if len(out.Workflows) >= 20 {
			break
		}
		file := entry.Path
		if entry.Type != "blob" || !(file == ".gitlab-ci.yml" || (strings.HasPrefix(file, ".gitea/workflows/") || strings.HasPrefix(file, ".github/workflows/")) && (strings.HasSuffix(file, ".yml") || strings.HasSuffix(file, ".yaml"))) {
			continue
		}
		result, err := s.Providers.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadFile, File: &privateconnector.FileArgs{Repository: ref, CommitSHA: head.SHA, Path: file}}, check)
		if err != nil {
			return out, err
		}
		if result.File == nil || result.File.Path != file || !source.VerifyBlob(manifest.Manifest.ObjectFormat, entry.SHA, result.File.Content) {
			return out, auth.ErrConflict
		}
		sum := sha256.Sum256(result.File.Content)
		out.Workflows = append(out.Workflows, Workflow{Path: file, SHA256: hex.EncodeToString(sum[:])})
	}
	err = s.Releases.Auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT 'native',document FROM deployment_configurations WHERE org_id=$1 AND repository_id=$2 UNION ALL SELECT 'gitops',document FROM gitops_configurations WHERE org_id=$1 AND source_repository_id=$2 AND ($3 OR delivery_repository_id=ANY($4::uuid[])) ORDER BY 1 LIMIT 100`, org, repo, a.AllRepositories, a.RepositoryIDs)
		if err != nil {
			return err
		}
		type item struct {
			kind string
			raw  []byte
		}
		var configs []item
		for rows.Next() {
			var c item
			if err = rows.Scan(&c.kind, &c.raw); err != nil {
				rows.Close()
				return err
			}
			configs = append(configs, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, item := range configs {
			var c struct {
				Environment          string                   `json:"environment"`
				DeliveryRepositoryID string                   `json:"delivery_repository_id"`
				Enabled              bool                     `json:"enabled"`
				Version              int64                    `json:"version"`
				BuildWorkflowPath    string                   `json:"build_workflow_path"`
				BuildWorkflowSHA256  string                   `json:"build_workflow_sha256"`
				Qualification        deployment.Qualification `json:"qualification"`
				Mode                 string                   `json:"mode"`
				ProvenancePublicKey  string                   `json:"provenance_public_key"`
			}
			if err = json.Unmarshal(item.raw, &c); err != nil {
				return err
			}
			path := Path{Kind: item.kind, Environment: c.Environment, Blockers: []string{}}
			for _, repository := range []string{repo, c.DeliveryRepositoryID} {
				if repository == "" {
					continue
				}
				ready, err := qualifiedRepositoryMergeTx(ctx, tx, org, repository)
				if err != nil {
					return err
				}
				if !ready {
					path.Blockers = append(path.Blockers, "Operator: qualify protected merge for the connected delivery path.")
					break
				}
			}
			matches := false
			for _, workflow := range out.Workflows {
				matches = matches || workflow.Path == c.BuildWorkflowPath && workflow.SHA256 == c.BuildWorkflowSHA256
			}
			if !c.Enabled {
				path.Blockers = append(path.Blockers, "Operator: enable the approved deployment path.")
			}
			if !matches {
				path.Blockers = append(path.Blockers, "Operator: approve the current CI evidence reporter through a repository change.")
			}
			if item.kind == "native" && provider == "gitea" {
				path.Blockers = append(path.Blockers, "Native delivery is unqualified for this Gitea instance; use the approved GitOps path.")
			}
			if item.kind == "native" && c.Mode == "pipeline" {
				q := c.Qualification
				if q.Provider != provider || q.ServerVersion != server || q.VerifiedAt.After(time.Now()) || q.ConnectionVersion != version || !q.ExpiresAt.After(time.Now()) || !q.PinnedInputs || !q.NativeEnforcement || !q.EnvironmentSerialization || !q.NoBypass {
					path.Blockers = append(path.Blockers, "Operator: qualify the protected native deployment workflow.")
				}
			}
			var reports [][]byte
			r, err := tx.Query(ctx, `SELECT document FROM release_receipts WHERE org_id=$1 AND repository_id=$2 AND kind=$3 AND environment=$4 AND configuration_version=$5 AND source_sha=$6 ORDER BY received_at DESC LIMIT 10`, org, repo, item.kind, c.Environment, c.Version, head.SHA)
			if err != nil {
				return err
			}
			for r.Next() {
				var body []byte
				if err = r.Scan(&body); err != nil {
					r.Close()
					return err
				}
				reports = append(reports, body)
			}
			err = r.Err()
			r.Close()
			if err != nil {
				return err
			}
			proved := false
			for _, body := range reports {
				var receipt deployment.SignedProvenance
				if json.Unmarshal(body, &receipt) == nil && release.Meaningful(receipt.Document) && receipt.Document.ExpiresAt.After(time.Now()) && time.Since(receipt.Document.IssuedAt) >= 0 && time.Since(receipt.Document.IssuedAt) <= 24*time.Hour && receipt.Document.WorkflowSHA256 == c.BuildWorkflowSHA256 && deployment.VerifySignature(c.ProvenancePublicKey, receipt.Document, receipt.Signature) {
					proved = true
				}
			}
			if !proved {
				path.Blockers = append(path.Blockers, "Waiting for the configured CI build and behavioural check receipt.")
			}
			path.Ready = len(path.Blockers) == 0
			out.Paths = append(out.Paths, path)
		}
		return nil
	})
	if len(out.Paths) == 0 {
		out.Blockers = append(out.Blockers, "Operator: install or approve a CI evidence reporter and deployment path for this repository.")
	}
	return out, err
}
