package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/connections"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/httpapi"
	"github.com/reforgeapp/reforge/internal/inventory"
	"github.com/reforgeapp/reforge/internal/privateconnector"
	"github.com/reforgeapp/reforge/internal/providers"
	"github.com/reforgeapp/reforge/internal/secrets"
	"github.com/reforgeapp/reforge/internal/store"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type inventoryReader struct {
	db          *store.Store
	connections *connections.Service
	read        func(privateconnector.Operation) (privateconnector.Result, error)
	after       func()
	calls       atomic.Int64
}

func (r *inventoryReader) Read(ctx context.Context, org, id string, op privateconnector.Operation, authorize func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	var out privateconnector.Result
	err := r.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR SHARE`, org).Scan(&locked); err != nil {
			return err
		}
		c, err := r.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if err = authorize(ctx, tx, c); err != nil {
			return err
		}
		r.calls.Add(1)
		out, err = r.read(op)
		out.OperationID = op.ID
		return err
	})
	if err == nil && r.after != nil {
		r.after()
	}
	return out, err
}

type inventoryFixture struct {
	db          *store.Store
	identity    *auth.Service
	server      *httpapi.Server
	owner       auth.Session
	cookie      *http.Cookie
	org         string
	vault       *secrets.Vault
	connections *connections.Service
	connection  connections.Connection
	reader      *inventoryReader
	service     *inventory.Service
}

func newInventoryFixture(t *testing.T, count int, orgs ...string) *inventoryFixture {
	t.Helper()
	ctx := context.Background()
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	cookie, owner := identityLogin(t, identity, server)
	f := &inventoryFixture{db: db, identity: identity, server: server, owner: owner, cookie: cookie, org: domain.NewID()}
	if len(orgs) > 0 {
		f.org = orgs[0]
	}
	err := db.Tenant(ctx, f.org, owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Inventory fixture')`, f.org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, f.org, owner.User.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	f.vault, err = secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	if err != nil {
		t.Fatal(err)
	}
	f.connections = connections.New(db, identity, f.vault, true)
	f.connection, err = f.connections.Create(ctx, owner, f.org, connections.CreateRequest{Kind: "forge", Provider: "github", Name: "Inventory", Endpoint: "https://api.github.com", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge", Namespace: "acme"}, Secret: "inventory-fixture-forge-secret"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	f.reader = &inventoryReader{db: db, connections: f.connections}
	f.reader.read = func(op privateconnector.Operation) (privateconnector.Result, error) {
		switch op.Kind {
		case privateconnector.GiteaInventory:
			n := 1
			if op.Inventory.Cursor != "" {
				n, _ = strconv.Atoi(op.Inventory.Cursor)
			}
			page := domain.Page[forge.Repository]{Items: []forge.Repository{}, Complete: n*100 >= count}
			for i := (n - 1) * 100; i < min(n*100, count); i++ {
				page.Items = append(page.Items, inventoryRepository(i))
			}
			if !page.Complete {
				page.NextCursor = strconv.Itoa(n + 1)
			}
			return privateconnector.Result{Inventory: &page}, nil
		case privateconnector.GiteaRepository:
			i, _ := strconv.Atoi(op.Repository.Repository.NativeID)
			r := inventoryRepository(i - 1)
			return privateconnector.Result{Repository: &r}, nil
		case privateconnector.ForgeReconcileChanges:
			page := domain.Page[forge.Change]{Items: []forge.Change{}, Complete: true}
			return privateconnector.Result{Changes: &page}, nil
		default:
			return privateconnector.Result{}, fmt.Errorf("unexpected operation %s", op.Kind)
		}
	}
	f.service = inventory.New(db, identity, f.vault, f.reader, providers.DecodeWebhook)
	server.RegisterInventory(f.service)
	return f
}
func inventoryRepository(i int) forge.Repository {
	return forge.Repository{RepoRef: forge.RepoRef{NativeID: strconv.Itoa(i + 1), FullName: fmt.Sprintf("acme/repository-%04d", i)}, URL: fmt.Sprintf("https://github.com/acme/repository-%04d", i), DefaultBranch: "main"}
}
func (f *inventoryFixture) sync(t *testing.T) inventory.Job {
	t.Helper()
	j, err := f.service.StartSync(context.Background(), f.owner, f.org, inventory.SyncInput{ConnectionID: f.connection.ID}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func (f *inventoryFixture) drain(t *testing.T, id string) inventory.Job {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 1100; i++ {
		j, err := f.service.Job(ctx, f.owner, f.org, id)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == "complete" || j.State == "stale" || j.State == "failed" {
			return j
		}
		lease, err := f.service.Claim(ctx, f.org, "fixture")
		if err != nil || lease == nil {
			t.Fatalf("claim: %v", err)
		}
		if err = f.service.Step(ctx, *lease); err != nil {
			t.Fatalf("step %s: %v", lease.Kind, err)
		}
	}
	t.Fatal("job did not finish")
	return inventory.Job{}
}
func (f *inventoryFixture) importAll(t *testing.T) inventory.Job {
	t.Helper()
	scan := f.drain(t, f.sync(t).ID)
	j, err := f.service.StartImport(context.Background(), f.owner, f.org, scan.ID, inventory.ImportInput{All: true}, scan.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return f.drain(t, j.ID)
}
func (f *inventoryFixture) repository(t *testing.T) inventory.Repository {
	t.Helper()
	page, err := f.service.Repositories(context.Background(), f.owner, f.org, 200, "")
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("repositories: %v", err)
	}
	return page.Items[0]
}
func TestInventoryThousandRepositoryAsyncImportAndScopedPagination(t *testing.T) {
	f := newInventoryFixture(t, 1000)
	ctx := context.Background()
	body, _ := json.Marshal(inventory.SyncInput{ConnectionID: f.connection.ID})
	response := identityRequest(f.server, "POST", "/api/v1/orgs/"+f.org+"/inventory-syncs", string(body), f.cookie, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "X-CSRF-Token": f.owner.CSRFToken})
	if response.Code != 202 || f.reader.calls.Load() != 0 {
		t.Fatalf("sync was not asynchronous: %d", response.Code)
	}
	var j inventory.Job
	if err := json.Unmarshal(response.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	duplicate := f.sync(t)
	if duplicate.ID != j.ID {
		t.Fatal("duplicate active scan")
	}
	scan := f.drain(t, j.ID)
	if scan.State != "complete" || scan.Processed != 1000 || scan.Pages != 10 {
		t.Fatalf("scan: %+v", scan)
	}
	page, err := f.service.Repositories(ctx, f.owner, f.org, 200, "")
	if err != nil || len(page.Items) != 0 {
		t.Fatal("preview implicitly imported")
	}
	imported, err := f.service.StartImport(ctx, f.owner, f.org, scan.ID, inventory.ImportInput{All: true}, scan.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	page, _ = f.service.Repositories(ctx, f.owner, f.org, 200, "")
	if len(page.Items) != 0 {
		t.Fatal("import ran synchronously")
	}
	done := f.drain(t, imported.ID)
	if done.Processed != 1000 || done.Pages != 10 {
		t.Fatalf("import %+v", done)
	}
	ids := map[string]bool{}
	cursor := ""
	for {
		p, err := f.service.Repositories(ctx, f.owner, f.org, 137, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range p.Items {
			if ids[r.ID] || r.SyncState != "fresh" {
				t.Fatal("unstable repository page")
			}
			ids[r.ID] = true
		}
		if p.Complete {
			break
		}
		cursor = p.NextCursor
	}
	if len(ids) != 1000 {
		t.Fatal("inventory truncated")
	}
	filtered, err := f.service.RepositoriesFiltered(ctx, f.owner, f.org, 1, "", inventory.RepositoryFilter{Query: "REPOSITORY-0099", Provider: "github"})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].NativeID != "100" {
		t.Fatal("server search did not filter before pagination", err)
	}
	literal, err := f.service.RepositoriesFiltered(ctx, f.owner, f.org, 10, "", inventory.RepositoryFilter{Query: "%"})
	if err != nil || len(literal.Items) != 0 {
		t.Fatal("search interpreted wildcard", err)
	}
	repo := f.repository(t)
	user, cookie := fixtureIdentity(t, f.db)
	session, err := f.identity.Authenticate(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.identity.PutMember(ctx, f.owner, f.org, user, auth.Membership{Role: domain.Viewer, RepositoryIDs: []string{repo.ID}}, 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.service.Repositories(ctx, session, f.org, 200, "")
	if err != nil || len(p.Items) != 1 || p.Items[0].ID != repo.ID {
		t.Fatal("scoped inventory exposed other repositories", err)
	}
	if _, err = f.service.Candidates(ctx, session, f.org, scan.ID, 10, ""); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("scoped viewer saw global preview")
	}
	events, err := workflow.New(f.db, f.identity, nil).Replay(ctx, session, f.org, 0, 200)
	if err != nil || len(events.Items) != 1 || events.Items[0].RepositoryID != repo.ID || events.Items[0].Type != "repository.synced" {
		t.Fatal("inventory SSE event scope", err)
	}
	if err = f.identity.Logout(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Repositories(ctx, session, f.org, 100, ""); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("revoked session retained inventory access")
	}
	other := newInventoryFixture(t, 0)
	if _, err = f.service.Repository(ctx, other.owner, other.org, repo.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("cross tenant repository read")
	}
	err = f.db.Tenant(ctx, other.org, "", func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM inventory_candidates WHERE job_id=$1`, scan.ID).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("candidate RLS bypass")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestInventoryConcurrentClaimRestartAndConnectionFence(t *testing.T) {
	f := newInventoryFixture(t, 1)
	ctx := context.Background()
	j := f.sync(t)
	var won atomic.Int64
	var lease inventory.Job
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l, err := f.service.Claim(ctx, f.org, fmt.Sprintf("worker-%d", i))
			if err != nil {
				t.Error(err)
				return
			}
			if l != nil {
				won.Add(1)
				mu.Lock()
				lease = *l
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("claims=%d", won.Load())
	}
	err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE inventory_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, j.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted := inventory.New(f.db, f.identity, f.vault, f.reader, providers.DecodeWebhook)
	replacement, err := restarted.Claim(ctx, f.org, "replacement")
	if err != nil || replacement == nil || replacement.Fence <= lease.Fence {
		t.Fatal("restart failed", err)
	}
	if err = f.service.Step(ctx, lease); !errors.Is(err, inventory.ErrStale) {
		t.Fatalf("stale fence accepted: %v", err)
	}
	if f.reader.calls.Load() != 0 {
		t.Fatal("stale attempt reached provider")
	}
	f.reader.after = func() {
		err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE connections SET version=version+1 WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
			return err
		})
		if err != nil {
			t.Error(err)
		}
	}
	if err = restarted.Step(ctx, *replacement); !errors.Is(err, inventory.ErrStale) {
		t.Fatalf("rotation gap accepted: %v", err)
	}
	stored, err := f.service.Job(ctx, f.owner, f.org, j.ID)
	if err != nil || stored.State != "stale" {
		t.Fatalf("stale result persisted: %s %v", stored.State, err)
	}
	candidates, err := f.service.Candidates(ctx, f.owner, f.org, j.ID, 100, "")
	if err != nil || len(candidates.Items) != 0 {
		t.Fatal("rotated result leaked into candidates")
	}
}
func TestInventoryPartialScanRenameAndCompleteRemoval(t *testing.T) {
	f := newInventoryFixture(t, 2)
	ctx := context.Background()
	f.importAll(t)
	initial, _ := f.service.Repositories(ctx, f.owner, f.org, 100, "")
	byNative := map[string]string{}
	for _, r := range initial.Items {
		byNative[r.NativeID] = r.ID
	}
	f.reader.read = func(op privateconnector.Operation) (privateconnector.Result, error) {
		r := inventoryRepository(0)
		r.FullName = "acme/renamed"
		return privateconnector.Result{Inventory: &domain.Page[forge.Repository]{Items: []forge.Repository{r}, Complete: false}}, nil
	}
	j := f.sync(t)
	lease, err := f.service.Claim(ctx, f.org, "partial")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *lease); !errors.Is(err, inventory.ErrIncomplete) {
		t.Fatalf("invalid completion accepted: %v", err)
	}
	partial, _ := f.service.Job(ctx, f.owner, f.org, j.ID)
	if partial.State != "stale" {
		t.Fatal("partial scan falsely healthy")
	}
	rows, _ := f.service.Repositories(ctx, f.owner, f.org, 100, "")
	for _, r := range rows.Items {
		if !r.Accessible || r.SyncState != "stale" {
			t.Fatal("partial scan removed access or remained healthy")
		}
	}
	f.reader.read = func(op privateconnector.Operation) (privateconnector.Result, error) {
		r := inventoryRepository(0)
		r.FullName = "acme/renamed"
		return privateconnector.Result{Inventory: &domain.Page[forge.Repository]{Items: []forge.Repository{r}, Complete: true}}, nil
	}
	done := f.drain(t, f.sync(t).ID)
	if done.State != "complete" {
		t.Fatal("complete sync failed")
	}
	first, err := f.service.Repository(ctx, f.owner, f.org, byNative["1"])
	if err != nil || first.Name != "acme/renamed" || first.ID != byNative["1"] || !first.Accessible {
		t.Fatal("rename changed immutable identity", err)
	}
	missing, _ := f.service.Repository(ctx, f.owner, f.org, byNative["2"])
	if missing.Accessible || missing.SyncState != "missing" {
		t.Fatal("complete scan did not remove access")
	}
}
func TestInventoryImportTeamVersionAndOwnerRevocation(t *testing.T) {
	f := newInventoryFixture(t, 2)
	ctx := context.Background()
	scan := f.drain(t, f.sync(t).ID)
	team, err := f.identity.PutTeam(ctx, f.owner, f.org, domain.NewID(), auth.Team{Name: "Inventory team"}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	job, err := f.service.StartImport(ctx, f.owner, f.org, scan.ID, inventory.ImportInput{NativeIDs: []string{"1"}, TeamIDs: []string{team.ID}}, scan.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.identity.PutTeam(ctx, f.owner, f.org, team.ID, auth.Team{Name: "Changed team"}, team.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.service.Claim(ctx, f.org, "import")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *lease); !errors.Is(err, inventory.ErrStale) {
		t.Fatal("changed team grant accepted", err)
	}
	got, _ := f.service.Job(ctx, f.owner, f.org, job.ID)
	if got.State != "stale" {
		t.Fatal("team edit did not block job")
	}
	job, err = f.service.StartImport(ctx, f.owner, f.org, scan.ID, inventory.ImportInput{All: true}, scan.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role='viewer',version=version+1 WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	lease, err = f.service.Claim(ctx, f.org, "revoked")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *lease); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("revoked owner import accepted", err)
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM repositories WHERE org_id=$1`, f.org).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("revoked import created repositories")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func inventorySignature(secret string, body []byte, delivery string) http.Header {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return http.Header{"X-Hub-Signature-256": []string{"sha256=" + hex.EncodeToString(mac.Sum(nil))}, "X-Github-Delivery": []string{delivery}, "X-Github-Event": []string{"pull_request"}}
}
func TestInventoryUnconfiguredWebhookRemainsScoped(t *testing.T) {
	f := newInventoryFixture(t, 0)
	path := "/api/v1/orgs/" + f.org + "/connections/" + f.connection.ID + "/webhook"
	response := identityRequest(f.server, "GET", path, "", f.cookie, nil)
	if response.Code != 404 || !strings.Contains(response.Body.String(), "webhook_unconfigured") {
		t.Fatalf("unconfigured webhook HTTP%d", response.Code)
	}
	response = identityRequest(f.server, "GET", "/api/v1/orgs/"+f.org+"/connections/"+domain.NewID()+"/webhook", "", f.cookie, nil)
	if response.Code != 403 {
		t.Fatalf("missing connection HTTP%d", response.Code)
	}
}
func TestInventorySignedWebhookReplayRotationAndPolling(t *testing.T) {
	f := newInventoryFixture(t, 1)
	ctx := context.Background()
	f.importAll(t)
	repo := f.repository(t)
	issued, err := f.service.ConfigureWebhook(ctx, f.owner, f.org, f.connection.ID, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"repository":{"id":1,"full_name":"attacker/untrusted-rename"},"pull_request":{"number":7,"head":{"sha":"untrusted-event-sha"}}}`)
	headers := inventorySignature(issued.Secret, body, "delivery-1")
	if err = f.service.HandleWebhook(ctx, domain.NewID(), issued.ID, headers, body); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("webhook tenant routing accepted", err)
	}
	if err = f.service.HandleWebhook(ctx, f.org, issued.ID, inventorySignature("wrong", body, "delivery-1"), body); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("forged signature accepted")
	}
	for _, delivery := range []string{"delivery-1", "delivery-1", "changed-unsigned-header"} {
		if err = f.service.HandleWebhook(ctx, f.org, issued.ID, inventorySignature(issued.Secret, body, delivery), body); err != nil {
			t.Fatal(err)
		}
	}
	var deliveries int
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM inventory_deliveries WHERE org_id=$1`, f.org).Scan(&deliveries)
	})
	if err != nil || deliveries != 1 {
		t.Fatalf("replay digest not deduplicated: %d %v", deliveries, err)
	}
	actualSHA := strings.Repeat("a", 40)
	f.reader.read = func(op privateconnector.Operation) (privateconnector.Result, error) {
		if op.Kind == privateconnector.GiteaRepository {
			r := inventoryRepository(0)
			return privateconnector.Result{Repository: &r}, nil
		}
		if op.Kind == privateconnector.ForgeReconcileChanges {
			return privateconnector.Result{Changes: &domain.Page[forge.Change]{Items: []forge.Change{{ID: "7", Repository: forge.RepoRef{NativeID: "1", FullName: "acme/repository-0000"}, HeadSHA: actualSHA, State: "open"}}, Complete: true}}, nil
		}
		return privateconnector.Result{}, errors.New("unexpected read")
	}
	runRefresh := func() {
		for i := 0; i < 10; i++ {
			lease, err := f.service.Claim(ctx, f.org, "refresh")
			if err != nil {
				t.Fatal(err)
			}
			if lease == nil {
				return
			}
			if err = f.service.Step(ctx, *lease); err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("refresh loop")
	}
	f.reader.after = func() {
		f.reader.after = nil
		during := []byte(`{"repository":{"id":1,"full_name":"acme/repository-0000"},"pull_request":{"number":7,"head":{"sha":"during-read"}}}`)
		if err := f.service.HandleWebhook(ctx, f.org, issued.ID, inventorySignature(issued.Secret, during, "during-read"), during); err != nil {
			t.Error(err)
		}
	}
	before := f.reader.calls.Load()
	runRefresh()
	changes, err := f.service.Changes(ctx, f.owner, f.org, repo.ID, 100, "")
	if err != nil || len(changes.Items) != 1 || changes.Items[0].HeadSHA != actualSHA {
		t.Fatal("webhook payload trusted instead of provider", err)
	}
	if f.reader.calls.Load()-before != 4 {
		t.Fatal("webhook during refresh did not schedule a second canonical read")
	}
	actualSHA = strings.Repeat("b", 40)
	oldBody := []byte(`{"repository":{"id":1,"full_name":"acme/repository-0000"},"pull_request":{"number":7,"head":{"sha":"older"}}}`)
	if err = f.service.HandleWebhook(ctx, f.org, issued.ID, inventorySignature(issued.Secret, oldBody, "out-of-order"), oldBody); err != nil {
		t.Fatal(err)
	}
	runRefresh()
	changes, _ = f.service.Changes(ctx, f.owner, f.org, repo.ID, 100, "")
	if changes.Items[0].HeadSHA != actualSHA {
		t.Fatal("old delivery regressed canonical state")
	}
	actualSHA = strings.Repeat("c", 40)
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET refresh_due=clock_timestamp()-interval '1 minute' WHERE org_id=$1`, f.org)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Maintain(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	runRefresh()
	changes, _ = f.service.Changes(ctx, f.owner, f.org, repo.ID, 100, "")
	if changes.Items[0].HeadSHA != actualSHA {
		t.Fatal("missing webhook polling failed")
	}
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connections SET settings=jsonb_set(settings,'{namespace}','"other"'::jsonb),version=version+1 WHERE org_id=$1 AND id=$2`, f.org, f.connection.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	outside := []byte(`{"repository":{"id":1,"full_name":"acme/repository-0000"},"pull_request":{"number":7,"head":{"sha":"outside-new-scope"}}}`)
	if err = f.service.HandleWebhook(ctx, f.org, issued.ID, inventorySignature(issued.Secret, outside, "namespace-changed"), outside); err != nil {
		t.Fatal(err)
	}
	if lease, err := f.service.Claim(ctx, f.org, "outside-scope"); err != nil || lease != nil {
		t.Fatal("webhook scheduled a repository outside current namespace", err)
	}
	rotated, err := f.service.ConfigureWebhook(ctx, f.owner, f.org, f.connection.ID, issued.Version, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.HandleWebhook(ctx, f.org, issued.ID, headers, body); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("old webhook credential survived rotation")
	}
	if err = f.service.RevokeWebhook(ctx, f.owner, f.org, f.connection.ID, rotated.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err = f.service.HandleWebhook(ctx, f.org, rotated.ID, inventorySignature(rotated.Secret, body, "revoked"), body); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("revoked webhook accepted")
	}
	var ciphertext []byte
	err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT envelope FROM inventory_webhooks WHERE org_id=$1 AND id=$2`, f.org, issued.ID).Scan(&ciphertext)
	})
	if err != nil || strings.Contains(string(ciphertext), issued.Secret) {
		t.Fatal("webhook plaintext persisted")
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", issued, issued), issued.Secret) {
		t.Fatal("webhook formatting leaked secret")
	}
}
func TestInventoryRateBackoffAndConnectionFairness(t *testing.T) {
	f := newInventoryFixture(t, 200)
	ctx := context.Background()
	first := f.sync(t)
	second, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "forge", Provider: "github", Name: "Second", Endpoint: "https://api.github.com", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge", Namespace: "acme"}, Secret: "second-forge-token"}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	secondJob, err := f.service.StartSync(ctx, f.owner, f.org, inventory.SyncInput{ConnectionID: second.ID}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	one, err := f.service.Claim(ctx, f.org, "one")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *one); err != nil {
		t.Fatal(err)
	}
	two, err := f.service.Claim(ctx, f.org, "two")
	if err != nil {
		t.Fatal(err)
	}
	if one.ConnectionID == two.ConnectionID {
		t.Fatal("busy connection monopolized jobs")
	}
	f.reader.read = func(privateconnector.Operation) (privateconnector.Result, error) {
		return privateconnector.Result{}, &domain.ProviderError{Kind: "rate_limit", RetryAfter: time.Minute, Message: "secret-remote-error"}
	}
	if err = f.service.Step(ctx, *two); err == nil {
		t.Fatal("rate limit hidden")
	}
	stored, _ := f.service.Job(ctx, f.owner, f.org, two.ID)
	if stored.State != "queued" || time.Until(stored.AvailableAt) < 55*time.Second || strings.Contains(stored.Reason, "secret") {
		t.Fatal("rate backoff/error sanitization failed")
	}
	if first.ID == secondJob.ID {
		t.Fatal("connections incorrectly coalesced")
	}
}

func TestInventorySchedulerCursorSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	prefix := domain.NewID()
	prefix = prefix[:len(prefix)-1]
	first := newInventoryFixture(t, 200, prefix+"2")
	second := newInventoryFixture(t, 200, prefix+"3")
	one, two := first.sync(t), second.sync(t)
	err := pgx.BeginFunc(ctx, first.db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.inventory_scheduler','true',true)`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO inventory_scheduler_cursors(id,org_id) VALUES('00000000-0000-4000-8000-000000000011',$1) ON CONFLICT(id) DO UPDATE SET org_id=excluded.org_id`, prefix+"1")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := first.service.RunOnce(ctx, "before-restart"); err != nil || !worked {
		t.Fatal("first scheduling pass", err)
	}
	restarted := inventory.New(first.db, first.identity, first.vault, first.reader, providers.DecodeWebhook)
	if worked, err := restarted.RunOnce(ctx, "after-restart"); err != nil || !worked {
		t.Fatal("restart scheduling pass", err)
	}
	a, err := first.service.Job(ctx, first.owner, first.org, one.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.service.Job(ctx, second.owner, second.org, two.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Pages != 1 || b.Pages != 1 {
		t.Fatalf("restart favored first tenant: %d %d", a.Pages, b.Pages)
	}
	_, err = first.db.Pool.Exec(ctx, `UPDATE inventory_scheduler_cursors SET org_id=$1`, first.org)
	if err == nil {
		var position string
		if err = first.db.Pool.QueryRow(ctx, `SELECT org_id::text FROM inventory_scheduler_cursors`).Scan(&position); err != nil {
			t.Fatal(err)
		}
		if position == first.org {
			t.Fatal("scheduler write accepted without trusted context")
		}
	}
}
func TestInventoryPartialChangeSnapshot(t *testing.T) {
	f := newInventoryFixture(t, 1)
	ctx := context.Background()
	f.importAll(t)
	repo := f.repository(t)
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error { return inventory.RequireFreshTx(ctx, tx, f.org, repo.ID) }); !errors.Is(err, inventory.ErrStale) {
		t.Fatal("no change snapshot treated as fresh", err)
	}
	if err := f.service.Maintain(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	lease, err := f.service.Claim(ctx, f.org, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *lease); err != nil {
		t.Fatal(err)
	}
	f.reader.read = func(op privateconnector.Operation) (privateconnector.Result, error) {
		return privateconnector.Result{Changes: &domain.Page[forge.Change]{Items: []forge.Change{{ID: "1", Repository: forge.RepoRef{NativeID: "1"}, HeadSHA: strings.Repeat("a", 40)}}, Complete: false, NextCursor: "2"}}, nil
	}
	lease, err = f.service.Claim(ctx, f.org, "changes")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *lease); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.Changes(ctx, f.owner, f.org, repo.ID, 100, "")
	if err != nil || len(page.Items) != 1 || page.SnapshotState != "stale" || page.ObservedAt != nil {
		t.Fatal("partial snapshot falsely fresh", err)
	}
	f.reader.read = func(op privateconnector.Operation) (privateconnector.Result, error) {
		return privateconnector.Result{}, &domain.ProviderError{Kind: "forbidden", Message: "scope removed"}
	}
	lease, err = f.service.Claim(ctx, f.org, "failed")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *lease); err == nil {
		t.Fatal("failed page accepted")
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error { return inventory.RequireFreshTx(ctx, tx, f.org, repo.ID) }); !errors.Is(err, inventory.ErrStale) {
		t.Fatal("failed partial change snapshot authorizes effects", err)
	}
}

func TestInventoryGitLabGeneratedSigningSecret(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	c, err := f.connections.Create(ctx, f.owner, f.org, connections.CreateRequest{Kind: "forge", Provider: "gitlab", Name: "GitLab signed webhooks", Endpoint: "https://gitlab.com", Settings: connections.Settings{AuthKind: "token", BillingRoute: "forge", Namespace: "acme"}, Secret: "gitlab-opaque-forge-secret"}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := f.service.ConfigureWebhook(ctx, f.owner, f.org, c.ID, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(issued.Secret, "whsec_"))
	if err != nil || len(key) != 32 {
		t.Fatal("invalid GitLab signing key format", err)
	}
	body := []byte(`{"object_kind":"merge_request","project":{"id":1,"path_with_namespace":"acme/project"},"object_attributes":{"iid":7}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	delivery := "signed-delivery"
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(delivery + "." + timestamp + "."))
	mac.Write(body)
	headers := http.Header{"Webhook-Id": []string{delivery}, "Webhook-Timestamp": []string{timestamp}, "Webhook-Signature": []string{"v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))}}
	if err = f.service.HandleWebhook(ctx, f.org, issued.ID, headers, body); err != nil {
		t.Fatal("generated key rejected by signed GitLab decoder", err)
	}
	headers.Set("Webhook-Timestamp", strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10))
	if err = f.service.HandleWebhook(ctx, f.org, issued.ID, headers, body); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("expired signed delivery accepted")
	}
}

func TestInventoryAccessFailureBlocksPollingUntilExplicitSync(t *testing.T) {
	f := newInventoryFixture(t, 1)
	ctx := context.Background()
	f.importAll(t)
	healthyRead := f.reader.read
	f.reader.read = func(privateconnector.Operation) (privateconnector.Result, error) {
		return privateconnector.Result{}, &domain.ProviderError{Kind: "auth", Message: "invalid credential"}
	}
	job := f.sync(t)
	lease, err := f.service.Claim(ctx, f.org, "unauthorized")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *lease); err == nil {
		t.Fatal("unauthorized inventory accepted")
	}
	stored, _ := f.service.Job(ctx, f.owner, f.org, job.ID)
	if stored.State != "failed" {
		t.Fatal("access failure not blocked")
	}
	if err = f.service.Maintain(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	if lease, err = f.service.Claim(ctx, f.org, "automatic-retry"); err != nil || lease != nil {
		t.Fatal("terminal credential error automatically retried", err)
	}
	if _, err = f.service.StartSync(ctx, f.owner, f.org, inventory.SyncInput{ConnectionID: f.connection.ID}, "explicit-retry"); err != nil {
		t.Fatal(err)
	}
	if lease, err = f.service.Claim(ctx, f.org, "owner-retry"); err != nil || lease == nil {
		t.Fatal("owner could not retry after correcting access", err)
	}
	f.reader.read = healthyRead
	if err = f.service.Step(ctx, *lease); err != nil {
		t.Fatal(err)
	}
	if err = f.service.Maintain(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	refreshed, err := f.service.Claim(ctx, f.org, "resumed-repository")
	if err != nil || refreshed == nil || refreshed.Kind != "refresh" {
		t.Fatal("full sync did not reactivate repository polling", err)
	}
	if err = f.service.Step(ctx, *refreshed); err != nil {
		t.Fatal(err)
	}
	refreshed, err = f.service.Claim(ctx, f.org, "resumed-changes")
	if err != nil || refreshed == nil {
		t.Fatal(err)
	}
	if err = f.service.Step(ctx, *refreshed); err != nil {
		t.Fatal(err)
	}
	repo := f.repository(t)
	for _, state := range []string{"missing", "archived", "stale"} {
		err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE repositories SET accessible=$3,archived=$4 WHERE org_id=$1 AND id=$2`, f.org, repo.ID, state != "missing", state == "archived"); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE inventory_repository_state SET state=$3 WHERE org_id=$1 AND repository_id=$2`, f.org, repo.ID, map[bool]string{true: "stale", false: "fresh"}[state == "stale"])
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		page, err := f.service.Changes(ctx, f.owner, f.org, repo.ID, 10, "")
		if err != nil || page.SnapshotState != "stale" {
			t.Fatalf("%s repository changes reported fresh: %v", state, err)
		}
	}

}
