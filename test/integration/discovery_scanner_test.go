package integration

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/inventory"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/source"
)

type discoveryScanReader struct {
	db interface {
		Tenant(context.Context, string, string, func(pgx.Tx) error) error
	}
	connections *connections.Service
	ref         forge.RepoRef
	change      forge.Change
	commits     map[string][]byte
	stale       bool
	passChecks  bool
}

func (r *discoveryScanReader) authorize(ctx context.Context, org, id string, fn func(context.Context, pgx.Tx, connections.Connection) error) error {
	return r.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		c, err := r.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		return fn(ctx, tx, c)
	})
}

func (r *discoveryScanReader) Read(ctx context.Context, org, id string, op privateconnector.Operation, fn func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	var out privateconnector.Result
	err := r.authorize(ctx, org, id, fn)
	if err != nil {
		return out, err
	}
	if r.stale {
		r.stale = false
		if err = r.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE connections SET version=version+1 WHERE org_id=$1 AND id=$2`, org, id)
			return err
		}); err != nil {
			return out, err
		}
	}
	switch op.Kind {
	case privateconnector.ForgeResolveRef:
		out.SHA = strings.Repeat("a", 40)
	case privateconnector.ForgeChecks:
		conclusion := "failure"
		if r.passChecks {
			conclusion = "success"
		}
		out.Checks = []forge.Check{{ID: "check-1", Name: "required", HeadSHA: op.Checks.CommitSHA, Status: "completed", Conclusion: conclusion}}
	case privateconnector.ForgeReadChange:
		out.Change = &r.change
	case privateconnector.ForgeBehind:
		behind := 0
		out.Behind = &behind
	default:
		return out, fmt.Errorf("unexpected discovery operation %s", op.Kind)
	}
	out.OperationID = op.ID
	return out, nil
}

func (r *discoveryScanReader) SourceReader(string, string, func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	return source.Reader{
		Manifest: func(ctx context.Context, repo forge.RepoRef, commit string) (forge.SourceManifest, error) {
			content, ok := r.commits[commit]
			if !ok {
				return forge.SourceManifest{}, discovery.ErrStale
			}
			return forge.SourceManifest{Repository: repo, CommitSHA: commit, ObjectFormat: "sha1", Proof: "immutable_ref_api", Complete: true, Entries: []forge.SourceEntry{{Path: "go.mod", SHA: blobSHA(content), Mode: "100644", Type: "blob"}}}, nil
		},
		File: func(ctx context.Context, repo forge.RepoRef, path, commit string) (forge.File, error) {
			content, ok := r.commits[commit]
			if !ok {
				return forge.File{}, discovery.ErrStale
			}
			return forge.File{Path: path, SHA: blobSHA(content), Content: content}, nil
		},
	}
}

func blobSHA(content []byte) string {
	h := sha1.New()
	_, _ = fmt.Fprintf(h, "blob %d\x00", len(content))
	_, _ = h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func (f *discoveryFixture) scannerFixture(t *testing.T, stale bool) (*discovery.Service, *discoveryScanReader) {
	t.Helper()
	ctx := context.Background()
	var ref forge.RepoRef
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE connections SET state='healthy',reason='' WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT native_id,name FROM repositories WHERE org_id=$1 AND id=$2`, f.org, f.repo).Scan(&ref.NativeID, &ref.FullName)
	}); err != nil {
		t.Fatal(err)
	}
	change := forge.Change{ID: "change-1", Repository: ref, HeadRepository: ref, TargetRepository: ref, Title: "Update example", URL: "https://example.test/change-1", HeadSHA: strings.Repeat("b", 40), TargetSHA: strings.Repeat("a", 40), TargetBranch: "main", AuthorID: "bot-1", AuthorLogin: "renovate", AuthorType: "Bot", State: "open"}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		var job string
		if err := tx.QueryRow(ctx, `SELECT id FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' ORDER BY created_at DESC LIMIT 1`, f.org, f.repo).Scan(&job); err != nil {
			return err
		}
		body, _ := json.Marshal(change)
		_, err := tx.Exec(ctx, `INSERT INTO inventory_changes(org_id,repository_id,native_id,snapshot,job_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,repository_id,native_id) DO UPDATE SET snapshot=excluded.snapshot,job_id=excluded.job_id`, f.org, f.repo, change.ID, body, job)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	config, err := f.service.GetConfig(ctx, f.owner, f.org, f.repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PutConfig(ctx, f.owner, f.org, f.repo, discovery.Config{TrustedBots: []discovery.BotIdentity{{Kind: "renovate", ActorID: "bot-1"}}, MergeAuthority: "reforge"}, config.Version, "scanner-config"); err != nil {
		t.Fatal(err)
	}
	reader := &discoveryScanReader{db: f.db, connections: f.connections, ref: ref, change: change, stale: stale, commits: map[string][]byte{strings.Repeat("a", 40): []byte("module example\n\ngo 1.22\n\nrequire example.test/old v1.0.0\n"), strings.Repeat("b", 40): []byte("module example\n\ngo 1.22\n\nrequire example.test/new v2.0.0\n")}}
	return discovery.New(f.db, f.identity, reader), reader
}

func TestDiscoveryScannerCanonicalFindingsAndLeaseRecovery(t *testing.T) {
	f := newDiscoveryFixture(t)
	service, _ := f.scannerFixture(t, false)
	ctx := context.Background()
	if _, err := service.StartScan(ctx, f.owner, f.org, f.repo, "scanner"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_scans SET state='running',lease_until=clock_timestamp()-interval '1 minute' WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	worked, err := service.RunOrganisationOnce(ctx, f.org)
	if err != nil || !worked {
		t.Fatalf("scanner restart: %v %v", err, worked)
	}
	page, err := service.List(ctx, f.owner, f.org, 20, "", discovery.Filter{RepositoryID: f.repo})
	if err != nil || len(page.Items) < 2 {
		t.Fatalf("scanner findings: %v %+v", err, page)
	}
	var dependency bool
	for _, finding := range page.Items {
		if finding.Category == "dependency_update" {
			dependency = true
			if finding.Evidence.Bot != "renovate" || len(finding.Evidence.Dependencies) != 2 || len(finding.Evidence.MergeBlockers) != 1 || !finding.Evidence.Complete {
				t.Fatalf("canonical dependency evidence: %+v", finding.Evidence)
			}
		}
	}
	if !dependency {
		t.Fatal("scanner omitted dependency finding")
	}
}

func TestDiscoveryScannerStaleConnectionFailsSafe(t *testing.T) {
	f := newDiscoveryFixture(t)
	service, _ := f.scannerFixture(t, true)
	ctx := context.Background()
	if _, err := service.StartScan(ctx, f.owner, f.org, f.repo, "scanner-stale"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunOrganisationOnce(ctx, f.org); err == nil {
		t.Fatal("stale connection scan succeeded")
	}
	status, err := service.ScanStatus(ctx, f.owner, f.org, f.repo)
	if err != nil || status.State != "stale" {
		t.Fatalf("stale scan state: %v %+v", err, status)
	}
}

var _ inventory.Reader = (*discoveryScanReader)(nil)

func TestDiscoveryScannerSurfacesConflictDespitePassingCI(t *testing.T) {
	f := newDiscoveryFixture(t)
	service, reader := f.scannerFixture(t, false)
	reader.passChecks = true
	reader.change.State = "opened"
	reader.change.HeadBranch = "reforge/repair/conflict-smoke"
	reader.change.MergeStatus = "conflict"
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		body, err := json.Marshal(reader.change)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE inventory_changes SET snapshot=$1 WHERE org_id=$2 AND repository_id=$3 AND native_id=$4`, body, f.org, f.repo, reader.change.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartScan(ctx, f.owner, f.org, f.repo, "scanner-conflict"); err != nil {
		t.Fatal(err)
	}
	worked, err := service.RunOrganisationOnce(ctx, f.org)
	if err != nil || !worked {
		t.Fatalf("scanner conflict run: %v %v", err, worked)
	}
	page, err := service.List(ctx, f.owner, f.org, 20, "", discovery.Filter{RepositoryID: f.repo})
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range page.Items {
		if finding.Category != "branch_conflict" {
			continue
		}
		for _, check := range finding.Evidence.Checks {
			if check.Name == "Reforge branch conflict" && check.HeadSHA == reader.change.HeadSHA && check.Conclusion == "failure" {
				return
			}
		}
		t.Fatalf("conflict finding lacks head-bound synthetic failure: %+v", finding.Evidence.Checks)
	}
	t.Fatal("passing CI hid a conflicting Reforge repair branch")
}
