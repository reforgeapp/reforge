package integration

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"reforge/internal/connections"
	"reforge/internal/forge"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

type botCooperationReader struct {
	base    *discoveryScanReader
	changes map[string]forge.Change
	files   map[string]map[string][]byte
}

func (r *botCooperationReader) Read(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if op.Kind == privateconnector.ForgeReadChange && op.Change != nil {
		var out privateconnector.Result
		err := r.base.authorize(ctx, org, id, authorize)
		if err != nil {
			return out, err
		}
		change, ok := r.changes[op.Change.ChangeID]
		if !ok {
			return out, discovery.ErrStale
		}
		out.Change = &change
		out.OperationID = op.ID
		return out, nil
	}
	return r.base.Read(ctx, org, id, op, authorize)
}

func (r *botCooperationReader) SourceReader(org, id string, authorize func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	base := r.base.SourceReader(org, id, authorize)
	return source.Reader{
		Manifest: func(ctx context.Context, repo forge.RepoRef, commit string) (forge.SourceManifest, error) {
			manifest, err := base.Manifest(ctx, repo, commit)
			if err != nil {
				return forge.SourceManifest{}, err
			}
			if content, ok := r.files[commit][".github/dependabot.yml"]; ok {
				blob := blobSHA(content)
				manifest.Entries = append(manifest.Entries,
					forge.SourceEntry{Path: ".github", SHA: dependabotTreeSHA(blob), Mode: "040000", Type: "tree"},
					forge.SourceEntry{Path: ".github/dependabot.yml", SHA: blob, Mode: "100644", Type: "blob"},
				)
			}
			if content, ok := r.files[commit]["renovate.json"]; ok {
				manifest.Entries = append(manifest.Entries, forge.SourceEntry{Path: "renovate.json", SHA: blobSHA(content), Mode: "100644", Type: "blob"})
			}
			return manifest, nil
		},
		File: func(ctx context.Context, repo forge.RepoRef, path, commit string) (forge.File, error) {
			if content, ok := r.files[commit][path]; ok {
				return forge.File{Path: path, SHA: blobSHA(content), Content: content}, nil
			}
			return base.File(ctx, repo, path, commit)
		},
	}
}

func dependabotTreeSHA(blob string) string {
	content := []byte("100644 dependabot.yml\x00")
	decoded, _ := hex.DecodeString(blob)
	content = append(content, decoded...)
	return fmt.Sprintf("%x", sha1.Sum(append([]byte(fmt.Sprintf("tree %d\x00", len(content))), content...)))
}

func TestDiscoveryRenovateDependabotCooperation(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	service, base := f.scannerFixture(t, false)
	mainSHA := strings.Repeat("a", 40)
	renovateSHA := strings.Repeat("b", 40)
	dependabotSHA := strings.Repeat("c", 40)
	baseline := []byte("module example\n\ngo 1.22\n\nrequire example.test/shared v1.0.0\n")
	updated := []byte("module example\n\ngo 1.22\n\nrequire example.test/shared v2.0.0\n")
	base.commits[mainSHA] = baseline
	base.commits[renovateSHA] = updated
	base.commits[dependabotSHA] = []byte("module example\n\ngo 1.22\n\nrequire (\n\texample.test/shared v2.0.0\n\texample.test/other v1.5.0\n)\n")
	changes := map[string]forge.Change{
		"renovate-change": {
			ID: "renovate-change", Repository: base.ref, HeadRepository: base.ref, TargetRepository: base.ref,
			Title: "Update shared module", HeadSHA: renovateSHA, TargetSHA: mainSHA, TargetBranch: "main",
			AuthorID: "renovate-actor-41", AuthorLogin: "renovate[bot]", AuthorType: "Bot", State: "open",
		},
		"dependabot-change": {
			ID: "dependabot-change", Repository: base.ref, HeadRepository: base.ref, TargetRepository: base.ref,
			Title: "Bump shared and other modules", HeadSHA: dependabotSHA, TargetSHA: mainSHA, TargetBranch: "main",
			AuthorID: "dependabot-actor-19", AuthorLogin: "dependabot[bot]", AuthorType: "Bot", State: "open",
		},
	}
	reader := &botCooperationReader{base: base, changes: changes, files: map[string]map[string][]byte{
		mainSHA: {
			"renovate.json":          []byte(`{"rangeStrategy":"bump"}`),
			".github/dependabot.yml": []byte("version: 2\nupdates:\n- package-ecosystem: gomod\n  directory: /\n  schedule:\n    interval: weekly\n"),
		},
	}}
	service = discovery.New(f.db, f.identity, reader)
	config, err := f.service.GetConfig(ctx, f.owner, f.org, f.repo)
	if err != nil {
		t.Fatal(err)
	}
	config.TrustedBots = []discovery.BotIdentity{{Kind: "renovate", ActorID: "renovate-actor-41"}}
	config.MergeAuthority = "observe"
	if _, err = f.service.PutConfig(ctx, f.owner, f.org, f.repo, config, config.Version, "bot-cooperation-one-bot"); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM inventory_changes WHERE org_id=$1 AND repository_id=$2 AND native_id='change-1'`, f.org, f.repo); err != nil {
			return err
		}
		var job string
		if err := tx.QueryRow(ctx, `SELECT id FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' ORDER BY created_at DESC LIMIT 1`, f.org, f.repo).Scan(&job); err != nil {
			return err
		}
		for _, change := range changes {
			body, err := json.Marshal(change)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO inventory_changes(org_id,repository_id,native_id,snapshot,job_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,repository_id,native_id) DO UPDATE SET snapshot=excluded.snapshot,job_id=excluded.job_id`, f.org, f.repo, change.ID, body, job); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.StartScan(ctx, f.owner, f.org, f.repo, "bot-cooperation-untrusted"); err != nil {
		t.Fatal(err)
	}
	if worked, scanErr := service.RunOrganisationOnce(ctx, f.org); scanErr != nil || !worked {
		status, statusErr := service.ScanStatus(ctx, f.owner, f.org, f.repo)
		t.Fatalf("initial bot scan: worked=%v err=%v status=%+v statusErr=%v", worked, scanErr, status, statusErr)
	}
	page, err := service.List(ctx, f.owner, f.org, 20, "", discovery.Filter{RepositoryID: f.repo, Category: "dependency_update"})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("bot findings: err=%v count=%d", err, len(page.Items))
	}
	findings := map[string]discovery.Finding{}
	for _, finding := range page.Items {
		findings[finding.SourceID] = finding
	}
	renovate := findings["renovate-change"]
	dependabot := findings["dependabot-change"]
	if renovate.ID == "" || dependabot.ID == "" {
		t.Fatalf("source identity not preserved: %+v", findings)
	}
	if renovate.Source != "forge_change" || renovate.Evidence.Bot != "renovate" || renovate.Evidence.Ownership != "bot" || renovate.Evidence.Change == nil || renovate.Evidence.Change.AuthorID != "renovate-actor-41" {
		t.Fatalf("Renovate adoption identity: %+v", renovate)
	}
	if dependabot.Source != "forge_change" || dependabot.Evidence.Bot != "" || dependabot.Evidence.Ownership != "unknown" || dependabot.Evidence.Change == nil || dependabot.Evidence.Change.AuthorID != "dependabot-actor-19" || len(dependabot.Evidence.Blockers) == 0 {
		t.Fatalf("untrusted Dependabot identity was adopted: %+v", dependabot)
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := discovery.PrepareRepairTx(ctx, tx, f.org, dependabot.ID, dependabot.Version)
		return err
	})
	if !errors.Is(err, discovery.ErrBlocked) {
		t.Fatalf("untrusted bot repair admission = %v", err)
	}
	config, err = f.service.GetConfig(ctx, f.owner, f.org, f.repo)
	if err != nil {
		t.Fatal(err)
	}
	config.TrustedBots = append(config.TrustedBots, discovery.BotIdentity{Kind: "dependabot", ActorID: "dependabot-actor-19"})
	config.MergeAuthority = "reforge"
	if _, err = f.service.PutConfig(ctx, f.owner, f.org, f.repo, config, config.Version, "bot-cooperation-both-bots"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.StartScan(ctx, f.owner, f.org, f.repo, "bot-cooperation-trusted"); err != nil {
		t.Fatal(err)
	}
	if worked, err := service.RunOrganisationOnce(ctx, f.org); err != nil || !worked {
		t.Fatalf("trusted bot scan: worked=%v err=%v", worked, err)
	}
	page, err = service.List(ctx, f.owner, f.org, 20, "", discovery.Filter{RepositoryID: f.repo, Category: "dependency_update"})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("trusted bot findings: err=%v count=%d", err, len(page.Items))
	}
	trustedFindings := map[string]discovery.Finding{}
	for _, finding := range page.Items {
		trustedFindings[finding.SourceID] = finding
		kind := finding.Evidence.Bot
		if kind != "renovate" && kind != "dependabot" {
			t.Fatalf("trusted bot not adopted: %+v", finding.Evidence)
		}
		if finding.Evidence.Ownership != "bot" || finding.Evidence.TargetBranch != "main" || len(finding.Evidence.Dependencies) == 0 {
			t.Fatalf("bot ownership or target evidence missing: %+v", finding.Evidence)
		}
		if !finding.Evidence.BotConfig.Renovate.Present || !finding.Evidence.BotConfig.Dependabot.Present {
			t.Fatalf("both native bot configurations not observed: %+v", finding.Evidence.BotConfig)
		}
		if len(finding.Evidence.MergeBlockers) == 0 {
			t.Fatalf("Reforge merge authority claimed without conflict blocker: %+v", finding.Evidence)
		}
	}
	previousRenovate := trustedFindings["renovate-change"]
	newHead := strings.Repeat("d", 40)
	base.commits[newHead] = []byte("module example\n\ngo 1.22\n\nrequire example.test/shared v2.0.0\n")
	moved := changes["renovate-change"]
	moved.HeadSHA = newHead
	changes["renovate-change"] = moved
	reader.changes = changes
	body, err := json.Marshal(moved)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE inventory_changes SET snapshot=$4 WHERE org_id=$1 AND repository_id=$2 AND native_id=$3`, f.org, f.repo, moved.ID, body)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.StartScan(ctx, f.owner, f.org, f.repo, "bot-cooperation-head-moved"); err != nil {
		t.Fatal(err)
	}
	if worked, err := service.RunOrganisationOnce(ctx, f.org); err != nil || !worked {
		t.Fatalf("current-head scan: worked=%v err=%v", worked, err)
	}
	page, err = service.List(ctx, f.owner, f.org, 20, "", discovery.Filter{RepositoryID: f.repo, Category: "dependency_update"})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("current-head findings: err=%v count=%d", err, len(page.Items))
	}
	findings = map[string]discovery.Finding{}
	for _, finding := range page.Items {
		findings[finding.SourceID] = finding
	}
	currentRenovate := findings["renovate-change"]
	if currentRenovate.ID != previousRenovate.ID || currentRenovate.Version <= previousRenovate.Version || currentRenovate.Evidence.HeadSHA != newHead {
		t.Fatalf("moved native head did not replace stale evidence: before=%+v after=%+v", previousRenovate, currentRenovate)
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := discovery.PrepareRepairTx(ctx, tx, f.org, currentRenovate.ID, previousRenovate.Version)
		return err
	})
	if !errors.Is(err, discovery.ErrStale) {
		t.Fatalf("repair using pre-move head version = %v", err)
	}
	for _, finding := range page.Items {
		err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			_, err := discovery.PrepareRepairTx(ctx, tx, f.org, finding.ID, finding.Version)
			return err
		})
		if !errors.Is(err, discovery.ErrDuplicate) {
			t.Fatalf("overlapping bot group admitted duplicate repair for %s: %v", finding.SourceID, err)
		}
	}
	var repairs, tasks int
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM maintenance_repairs WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo).Scan(&repairs); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM workflow_tasks WHERE org_id=$1 AND repository_id=$2 AND state NOT IN ('failed','cancelled')`, f.org, f.repo).Scan(&tasks)
	}); err != nil {
		t.Fatal(err)
	}
	if repairs != 0 || tasks != 0 {
		t.Fatalf("duplicate bot work persisted: repairs=%d tasks=%d", repairs, tasks)
	}
}
