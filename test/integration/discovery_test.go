package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/maintenance/detectors"
	"github.com/reforgeapp/reforge/internal/maintenance/discovery"
)

type discoveryFixture struct {
	*inventoryFixture
	service *discovery.Service
	repo    string
}

func newDiscoveryFixture(t *testing.T) *discoveryFixture {
	t.Helper()
	f := newInventoryFixture(t, 1)
	f.importAll(t)
	repo := f.repository(t)
	ctx := context.Background()
	if err := f.service.Maintain(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	lease, err := f.service.Claim(ctx, f.org, "discovery-fixture")
	if err != nil || lease == nil {
		t.Fatalf("claim inventory maintenance: %v", err)
	}
	if err = f.service.Step(ctx, *lease); err != nil {
		t.Fatal(err)
	}
	lease, err = f.service.Claim(ctx, f.org, "discovery-changes")
	if err != nil || lease == nil {
		t.Fatalf("claim changes: %v", err)
	}
	if err = f.service.Step(ctx, *lease); err != nil {
		t.Fatal(err)
	}
	d := discovery.New(f.db, f.identity, nil)
	f.server.RegisterDiscovery(d)
	return &discoveryFixture{inventoryFixture: f, service: d, repo: repo.ID}
}

func (f *discoveryFixture) observation(sourceID, target, head string, deps []detectors.Dependency) discovery.Observation {
	return discovery.Observation{
		RepositoryID: f.repo,
		Source:       "forge_change",
		SourceID:     sourceID,
		Category:     "dependency_update",
		Severity:     "high",
		Title:        "Dependency update",
		Evidence: discovery.Evidence{
			ConnectionID:      f.connection.ID,
			ConnectionVersion: f.connection.Version,
			HeadSHA:           head,
			TargetSHA:         strings.Repeat("b", 40),
			TargetBranch:      target,
			Dependencies:      deps,
			Complete:          true,
		},
	}
}

func (f *discoveryFixture) observe(t *testing.T, in discovery.Observation) discovery.Finding {
	t.Helper()
	var out discovery.Finding
	err := f.db.Tenant(context.Background(), f.org, "", func(tx pgx.Tx) error {
		var err error
		out, err = discovery.ObserveTx(context.Background(), tx, f.org, in, f.owner.User.ID, "discovery-test")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDiscoveryFingerprintLifecycleAndConcurrentObservation(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	dep := []detectors.Dependency{{Ecosystem: "go", Manifest: "go.mod", Name: "example", From: "v1"}}
	in := f.observation("change-1", "main", strings.Repeat("a", 40), dep)
	first := f.observe(t, in)

	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
				_, err := discovery.ObserveTx(ctx, tx, f.org, in, f.owner.User.ID, "concurrent")
				return err
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.service.List(ctx, f.owner, f.org, 20, "", discovery.Filter{RepositoryID: f.repo})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("duplicate observations: %v %+v", err, page)
	}

	expires := time.Now().Add(time.Hour)
	snoozed, err := f.service.Update(ctx, f.owner, f.org, first.ID, discovery.Update{Action: "snooze", Reason: "later", SnoozeUntil: &expires}, first.Version, "snooze")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Update(ctx, f.owner, f.org, first.ID, discovery.Update{Action: "dismiss", Reason: "ignore"}, first.Version, "stale"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("old If-Match accepted: %v", err)
	}
	if snoozed.State != "snoozed" {
		t.Fatalf("snooze state: %s", snoozed.State)
	}
	f.observe(t, in)
	current, err := f.service.Get(ctx, f.owner, f.org, first.ID)
	if err != nil || current.State != "snoozed" {
		t.Fatalf("same evidence reopened snooze: %v %s", err, current.State)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE maintenance_findings SET snooze_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, first.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.observe(t, in)
	current, err = f.service.Get(ctx, f.owner, f.org, first.ID)
	if err != nil || current.State != "open" {
		t.Fatalf("expired snooze did not reopen: %v %s", err, current.State)
	}

	dismissed, err := f.service.Update(ctx, f.owner, f.org, current.ID, discovery.Update{Action: "dismiss", Reason: "reviewed"}, current.Version, "dismiss")
	if err != nil {
		t.Fatal(err)
	}
	unchanged := f.observe(t, in)
	if unchanged.State != "dismissed" || unchanged.Version != dismissed.Version {
		t.Fatal("unchanged evidence reopened dismissal")
	}
	changed := f.observation("change-1", "main", strings.Repeat("c", 40), dep)
	f.observe(t, changed)
	current, _ = f.service.Get(ctx, f.owner, f.org, first.ID)
	if current.State != "open" || current.Version != dismissed.Version+1 {
		t.Fatalf("new head did not preserve open state: %s", current.State)
	}
	otherBranch := f.observation("change-1", "release", strings.Repeat("d", 40), dep)
	second := f.observe(t, otherBranch)
	old, err := f.service.Get(ctx, f.owner, f.org, first.ID)
	if err != nil || old.State != "superseded" || old.SupersededBy != second.ID {
		t.Fatalf("source change did not supersede old finding: %+v %v", old, err)
	}
	returned := f.observe(t, changed)
	if returned.ID != first.ID || returned.State != "open" || returned.SupersededBy != "" {
		t.Fatalf("restored target remained superseded: %+v", returned)
	}
}

func TestDiscoveryHTTPIdentityAndMutationScope(t *testing.T) {
	f := newDiscoveryFixture(t)
	finding := f.observe(t, f.observation("http-change", "main", strings.Repeat("e", 40), nil))
	userID, scopedCookie := fixtureIdentity(t, f.db)
	member, err := f.identity.PutMember(context.Background(), f.owner, f.org, userID, auth.Membership{Role: domain.Viewer, RepositoryIDs: []string{f.repo}}, 0, "scope")
	if err != nil {
		t.Fatal(err)
	}
	scopedSession, err := f.identity.Authenticate(context.Background(), scopedCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Get(context.Background(), scopedSession, f.org, finding.ID); err != nil {
		t.Fatalf("scoped member finding read: %v", err)
	}
	assigned, err := f.service.Update(context.Background(), f.owner, f.org, finding.ID, discovery.Update{Action: "assign", AssignedTo: userID}, finding.Version, "assign")
	if err != nil || assigned.AssignedTo != userID {
		t.Fatalf("assign scoped member: %v %+v", err, assigned)
	}
	member, err = f.identity.PutMember(context.Background(), f.owner, f.org, userID, auth.Membership{Role: domain.Viewer}, member.Version, "scope-remove")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Update(context.Background(), f.owner, f.org, finding.ID, discovery.Update{Action: "assign", AssignedTo: userID}, assigned.Version, "assign-out-of-scope"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("out-of-scope assignee accepted: %v", err)
	}
	if _, err = f.service.Get(context.Background(), scopedSession, f.org, finding.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("removed scope retained read: %v", err)
	}
	expected := fmt.Sprintf("\"%d\"", assigned.Version)
	next := fmt.Sprintf("\"%d\"", assigned.Version+1)
	path := "/api/v1/orgs/" + f.org + "/findings/" + finding.ID
	got := identityRequest(f.server, http.MethodGet, path, "", f.cookie, nil)
	if got.Code != 200 || got.Header().Get("ETag") != expected {
		t.Fatalf("finding GET: %d %s %s", got.Code, got.Body.String(), got.Header().Get("ETag"))
	}
	body, _ := json.Marshal(discovery.Update{Action: "dismiss", Reason: "reviewed"})
	updated := identityRequest(f.server, http.MethodPatch, path, string(body), f.cookie, map[string]string{"Content-Type": "application/json", "If-Match": expected, "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": f.owner.CSRFToken})
	if updated.Code != 200 || updated.Header().Get("ETag") != next {
		t.Fatalf("finding PATCH: %d %s", updated.Code, updated.Body.String())
	}
	stale := identityRequest(f.server, http.MethodPatch, path, string(body), f.cookie, map[string]string{"Content-Type": "application/json", "If-Match": expected, "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": f.owner.CSRFToken})
	if stale.Code != 409 {
		t.Fatalf("stale PATCH: %d %s", stale.Code, stale.Body.String())
	}
	other := newInventoryFixture(t, 0)
	otherService := discovery.New(other.db, other.identity, nil)
	if _, err := otherService.Get(context.Background(), other.owner, other.org, finding.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("cross tenant finding read: %v", err)
	}
}

func TestDiscoveryPrepareRepairRejectsStaleEvidence(t *testing.T) {
	f := newDiscoveryFixture(t)
	finding := f.observe(t, f.observation("repair-change", "main", strings.Repeat("f", 40), nil))
	err := f.db.Tenant(context.Background(), f.org, "", func(tx pgx.Tx) error {
		if _, err := discovery.PrepareRepairTx(context.Background(), tx, f.org, finding.ID, finding.Version); err != nil {
			return fmt.Errorf("fresh repair rejected: %w", err)
		}
		if _, err := tx.Exec(context.Background(), `UPDATE maintenance_findings SET last_seen=clock_timestamp()-interval '16 minutes' WHERE org_id=$1 AND id=$2`, f.org, finding.ID); err != nil {
			return err
		}
		_, err := discovery.PrepareRepairTx(context.Background(), tx, f.org, finding.ID, finding.Version)
		return err
	})
	if !errors.Is(err, discovery.ErrStale) {
		t.Fatalf("stale evidence accepted for repair: %v", err)
	}
}

func TestDiscoveryGroupedOverlapAndExistingRepair(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	dep := []detectors.Dependency{{Ecosystem: "go", Manifest: "go.mod", Name: "example/a", From: "v1", To: "v2"}}
	input := f.observation("group-1", "main", strings.Repeat("a", 40), dep)
	input.Evidence.Bot = "renovate"
	first := f.observe(t, input)
	input.SourceID = "group-2"
	input.Evidence.Dependencies = append(dep, detectors.Dependency{Ecosystem: "go", Manifest: "go.mod", Name: "example/b", From: "v2", To: "v3"})
	second := f.observe(t, input)
	prepare := func() error {
		return f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			_, err := discovery.PrepareRepairTx(ctx, tx, f.org, first.ID, first.Version)
			return err
		})
	}
	if err := prepare(); err != nil {
		t.Fatalf("overlapping open finding without active repair blocked: %v", err)
	}
	task := domain.NewID()
	err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workflow_tasks(org_id,id,repository_id,operation_id,idempotency_key,request_hash,recipe,recipe_version,target_branch,policy_hash,starting_policy_hash,state,max_attempts,created_by,model_route) VALUES($1,$2::uuid,$3,$4,$2::text,'fixture','go','v1','main','fixture','fixture','completed',1,$5,'none')`, f.org, task, f.repo, domain.NewID(), f.owner.User.ID)
		if err != nil {
			return err
		}
		return discovery.BindRepairTx(ctx, tx, first, task)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = prepare(); !errors.Is(err, discovery.ErrDuplicate) {
		t.Fatalf("existing completed repair duplicated: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := discovery.PrepareRepairTx(ctx, tx, f.org, second.ID, second.Version)
		return err
	}); !errors.Is(err, discovery.ErrDuplicate) {
		t.Fatalf("overlap with active repair accepted: %v", err)
	}
}

func TestDiscoveryOwnerRepairCanOverlapBotFindings(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	dep := []detectors.Dependency{{Ecosystem: "npm", Manifest: "web/package.json", Name: "typescript", From: "6.0.3", To: "5.9.3"}}
	dependabotInput := f.observation("dependabot-change", "main", strings.Repeat("a", 40), dep)
	dependabotInput.Evidence.Bot = "dependabot"
	dependabotInput.Evidence.Ownership = "bot"
	f.observe(t, dependabotInput)

	ownerInput := f.observation("reforge-change", "main", strings.Repeat("c", 40), dep)
	ownerInput.Evidence.Ownership = "reforge"
	owner := f.observe(t, ownerInput)
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := discovery.PrepareRepairTx(ctx, tx, f.org, owner.ID, owner.Version)
		return err
	}); err != nil {
		t.Fatalf("Reforge-owned repair blocked by open Dependabot finding: %v", err)
	}

	renovateInput := f.observation("renovate-change", "main", strings.Repeat("d", 40), dep)
	renovateInput.Evidence.Bot = "renovate"
	renovateInput.Evidence.Ownership = "bot"
	renovate := f.observe(t, renovateInput)
	err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := discovery.PrepareRepairTx(ctx, tx, f.org, renovate.ID, renovate.Version)
		return err
	})
	if !errors.Is(err, discovery.ErrDuplicate) {
		t.Fatalf("overlapping Renovate and Dependabot work accepted: %v", err)
	}
}
