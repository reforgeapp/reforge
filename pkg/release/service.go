package release

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/source"
	"github.com/reforgeapp/reforge/pkg/store"
)

type Receipt struct {
	Mode       string                      `json:"mode,omitempty"`
	ID         string                      `json:"id"`
	Provenance deployment.SignedProvenance `json:"provenance"`
	ReceivedAt time.Time                   `json:"received_at"`
}

type configuration struct {
	Mode                string `json:"mode"`
	RepositoryID        string `json:"repository_id"`
	SourceRepositoryID  string `json:"source_repository_id"`
	Enabled             bool   `json:"enabled"`
	Version             int64  `json:"version"`
	ProvenancePublicKey string `json:"provenance_public_key"`
	BuildWorkflowSHA256 string `json:"build_workflow_sha256"`
}

type Service struct {
	DB   *store.Store
	Auth *auth.Service
}

func configurationTx(ctx context.Context, tx pgx.Tx, org, repo, kind, env string) (configuration, error) {
	var c configuration
	var raw []byte
	table, column := "deployment_configurations", "repository_id"
	if kind == "gitops" {
		table, column = "gitops_configurations", "source_repository_id"
	} else if kind != "native" {
		return c, auth.ErrInvalid
	}
	err := tx.QueryRow(ctx, `SELECT document FROM `+table+` WHERE org_id=$1 AND `+column+`=$2 AND environment=$3`, org, repo, env).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, auth.ErrForbidden
	}
	if err == nil {
		err = json.Unmarshal(raw, &c)
	}
	return c, err
}

func valid(c configuration, org, repo string, signed deployment.SignedProvenance, now time.Time) bool {
	p := signed.Document
	if !c.Enabled || c.Version != p.ConfigurationVersion || !forge.ValidEnvironment(p.Environment) || p.OrgID != org || p.RepositoryID != repo || c.RepositoryID != repo && c.SourceRepositoryID != repo || !source.ValidSHA(p.SourceSHA, "sha1") || !forge.ValidArtifactDigest(p.ArtifactDigest) {
		return false
	}
	if !deployment.ValidProvenance(deployment.Configuration{RepositoryID: repo, ProvenancePublicKey: c.ProvenancePublicKey}, org, deployment.PreviewRequest{SourceSHA: p.SourceSHA, ArtifactDigest: p.ArtifactDigest, Provenance: signed}, now) {
		return false
	}
	if p.Validation != nil && (!source.ValidSHA(c.BuildWorkflowSHA256, "sha256") || c.BuildWorkflowSHA256 != p.WorkflowSHA256) {
		return false
	}
	return true
}

func Meaningful(p deployment.Provenance) bool {
	v := p.Validation
	if v == nil || !v.Complete || v.Builds < 1 || v.Builds > 64 || len(v.Cases) < 1 || len(v.Cases) > 10000 || len(v.Outputs) < 1 || len(v.Outputs) > 64 || !source.ValidSHA(v.TargetSHA, "sha1") {
		return false
	}
	for name, state := range v.Cases {
		if strings.TrimSpace(name) == "" || len(name) > 2048 || state != "pass" {
			return false
		}
	}
	for _, digest := range v.Outputs {
		if !source.ValidSHA(digest, "sha256") {
			return false
		}
	}
	return true
}

func (s *Service) Receive(ctx context.Context, org, repo string, signed deployment.SignedProvenance) (Receipt, error) {
	var out Receipt
	if !auth.ValidID(org) || !auth.ValidID(repo) {
		return out, auth.ErrInvalid
	}
	p := signed.Document
	if p.Kind != "native" && p.Kind != "gitops" || !forge.ValidEnvironment(p.Environment) {
		return out, auth.ErrInvalid
	}
	raw, err := json.Marshal(signed)
	if err != nil || len(raw) > 1<<20 {
		return out, auth.ErrInvalid
	}
	out.ID = domain.StableID("release-receipt", org, repo, p.Kind, p.Environment, p.BuildID, p.SourceSHA)
	err = s.DB.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		c, err := configurationTx(ctx, tx, org, repo, p.Kind, p.Environment)
		if err != nil {
			return err
		}
		if !valid(c, org, repo, signed, time.Now()) {
			return auth.ErrForbidden
		}
		target := ""
		if p.Validation != nil {
			target = p.Validation.TargetSHA
		}
		_, err = tx.Exec(ctx, `INSERT INTO release_receipts(org_id,id,repository_id,environment,kind,configuration_version,source_sha,target_sha,artifact_digest,build_id,document) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(org_id,id) DO NOTHING`, org, out.ID, repo, p.Environment, p.Kind, p.ConfigurationVersion, p.SourceSHA, target, p.ArtifactDigest, p.BuildID, raw)
		if err != nil {
			return err
		}
		var previous []byte
		if err = tx.QueryRow(ctx, `SELECT document,received_at FROM release_receipts WHERE org_id=$1 AND id=$2`, org, out.ID).Scan(&previous, &out.ReceivedAt); err != nil {
			return err
		}
		var existing deployment.SignedProvenance
		if json.Unmarshal(previous, &existing) != nil {
			return auth.ErrInvalid
		}
		canonical, _ := json.Marshal(existing)
		if string(canonical) != string(raw) {
			return auth.ErrConflict
		}
		out.Provenance = existing
		return nil
	})
	return out, err
}

func BehaviourReferenceTx(ctx context.Context, tx pgx.Tx, org, repo, head, target string, now time.Time) (string, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,document FROM release_receipts WHERE org_id=$1 AND repository_id=$2 AND source_sha=$3 AND target_sha=$4 ORDER BY received_at DESC,id LIMIT 100`, org, repo, head, target)
	if err != nil {
		return "", err
	}
	var receipts []Receipt
	for rows.Next() {
		var r Receipt
		var raw []byte
		if err = rows.Scan(&r.ID, &raw); err != nil {
			rows.Close()
			return "", err
		}
		if err = json.Unmarshal(raw, &r.Provenance); err != nil {
			rows.Close()
			return "", err
		}
		receipts = append(receipts, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	for _, r := range receipts {
		p := r.Provenance.Document
		if !Meaningful(p) || now.Sub(p.IssuedAt) > 24*time.Hour {
			continue
		}
		c, err := configurationTx(ctx, tx, org, repo, p.Kind, p.Environment)
		if errors.Is(err, auth.ErrForbidden) {
			continue
		}
		if err != nil {
			return "", err
		}
		if valid(c, org, repo, r.Provenance, now) {
			return "native-build-proof:" + r.ID, nil
		}
	}
	return "", nil
}
