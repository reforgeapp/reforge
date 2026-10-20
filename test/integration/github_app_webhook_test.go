package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/githubapp"
	"github.com/reforgeapp/reforge/pkg/httpapi"
	"github.com/reforgeapp/reforge/pkg/inventory"
	"github.com/reforgeapp/reforge/pkg/providers"
	"github.com/reforgeapp/reforge/pkg/secrets"
	"github.com/reforgeapp/reforge/pkg/store"
)

type webhookHarness struct {
	t          *testing.T
	db         *store.Store
	server     *httpapi.Server
	identity   *auth.Service
	owner      auth.Session
	cookie     *http.Cookie
	org        string
	conns      *connections.Service
	inv        *inventory.Service
	gh         *githubapp.Service
	vault      *secrets.Vault
	secret     []byte
	appID      int64
	instID     int64
	accountID  int64
	connection string
	webhookID  string
}

func newWebhookHarness(t *testing.T) *webhookHarness {
	t.Helper()
	db := authDB(t)
	identity, server := identityServer(t, db, authConfig())
	cookie, owner := identityLogin(t, identity, server)
	vault, err := secrets.New("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("g", 32)))})
	if err != nil {
		t.Fatal(err)
	}
	conns := connections.New(db, identity, vault, true)
	server.RegisterConnections(conns)
	inv := inventory.New(db, identity, vault, nil, providers.DecodeWebhook)
	server.RegisterInventory(inv)
	seed := time.Now().UnixNano()
	h := &webhookHarness{t: t, db: db, server: server, identity: identity, owner: owner, cookie: cookie, org: owner.Organisations[0].ID, conns: conns, inv: inv, vault: vault, secret: []byte("hosted-webhook-secret-value"), appID: seed%1000000000 + 1, instID: seed%1000000000000 + 1, accountID: seed%1000000000 + 7}
	hosted := &githubapp.Hosted{AppID: h.appID, Slug: "reforge-hosted", ClientID: "Iv1.hosted", ClientSecret: []byte("hosted-client-secret-value"), PrivateKeyPEM: []byte(rsaPEM(t)), WebhookSecret: h.secret}
	h.gh = githubapp.New(db, identity, vault, conns, inv, githubapp.Options{PublicURL: "http://127.0.0.1:8080", Edition: "hosted", Development: true, Hosted: hosted})
	server.RegisterGitHubApp(h.gh)
	h.connect()
	return h
}

func (h *webhookHarness) actor(fn func(pgx.Tx, domain.Actor) error) {
	h.t.Helper()
	if err := h.identity.WithActor(context.Background(), h.owner, h.org, fn); err != nil {
		h.t.Fatal(err)
	}
}

func (h *webhookHarness) actorOrg(org string, fn func(pgx.Tx, domain.Actor) error) {
	h.t.Helper()
	if err := h.identity.WithActor(context.Background(), h.owner, org, fn); err != nil {
		h.t.Fatal(err)
	}
}

func (h *webhookHarness) addOrg() string {
	h.t.Helper()
	org := domain.NewID()
	err := h.db.Tenant(context.Background(), org, h.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `INSERT INTO organisations(id,name) VALUES($1,'Tenant B')`, org); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, h.owner.User.ID)
		return err
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return org
}

func (h *webhookHarness) addConnection(org string, instID, accountID int64) (string, string) {
	h.t.Helper()
	ctx := context.Background()
	connection, webhookID := domain.NewID(), domain.NewID()
	c := connections.Connection{ID: connection, OrgID: org, Kind: "forge", Provider: "github", Name: "managed hosted", Endpoint: connections.GitHubAPI, State: "healthy", Settings: connections.Settings{BillingRoute: "forge", AuthKind: connections.PlatformAuthKind, Managed: "github_hosted", AppID: strconv.FormatInt(h.appID, 10), InstallationID: strconv.FormatInt(instID, 10)}}
	h.actorOrg(org, func(tx pgx.Tx, a domain.Actor) error {
		if err := h.conns.CreateManagedTx(ctx, tx, a, &c, "", "fixture"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO github_installation_bindings(app_id,installation_id,account_id,org_id,connection_id) VALUES($1,$2,$3,$4,$5)`, h.appID, instID, accountID, org, c.ID); err != nil {
			return err
		}
		_, err := h.inv.AdoptWebhookTx(ctx, tx, a, org, c.ID, webhookID, string(h.secret), "fixture")
		return err
	})
	return connection, webhookID
}

func (h *webhookHarness) connect() {
	h.t.Helper()
	h.connection, h.webhookID = h.addConnection(h.org, h.instID, h.accountID)
}

func (h *webhookHarness) addManifestConnection(org string, instID, accountID int64, webhookSecret string) (string, string) {
	h.t.Helper()
	ctx := context.Background()
	connection, webhookID := domain.NewID(), domain.NewID()
	c := connections.Connection{ID: connection, OrgID: org, Kind: "forge", Provider: "github", Name: "managed manifest", Endpoint: connections.GitHubAPI, State: "healthy", Settings: connections.Settings{BillingRoute: "forge", AuthKind: "github_app", Managed: "github_manifest", AppID: strconv.FormatInt(h.appID, 10), InstallationID: strconv.FormatInt(instID, 10)}}
	h.actorOrg(org, func(tx pgx.Tx, a domain.Actor) error {
		if err := h.conns.CreateManagedTx(ctx, tx, a, &c, rsaPEM(h.t), "fixture"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO github_installation_bindings(app_id,installation_id,account_id,org_id,connection_id) VALUES($1,$2,$3,$4,$5)`, h.appID, instID, accountID, org, c.ID); err != nil {
			return err
		}
		_, err := h.inv.AdoptWebhookTx(ctx, tx, a, org, c.ID, webhookID, webhookSecret, "fixture")
		return err
	})
	return connection, webhookID
}

func (h *webhookHarness) restart(webhookSecret []byte) *githubapp.Service {
	h.t.Helper()
	hosted := &githubapp.Hosted{AppID: h.appID, Slug: "reforge-hosted", ClientID: "Iv1.hosted", ClientSecret: []byte("hosted-client-secret-value"), PrivateKeyPEM: []byte(rsaPEM(h.t)), WebhookSecret: webhookSecret}
	return githubapp.New(h.db, h.identity, h.vault, h.conns, h.inv, githubapp.Options{PublicURL: "http://127.0.0.1:8080", Edition: "hosted", Development: true, Hosted: hosted})
}

func (h *webhookHarness) seedRepository(org, connection, nativeID string) string {
	h.t.Helper()
	ctx := context.Background()
	repoID, jobID := domain.NewID(), domain.NewID()
	h.actorOrg(org, func(tx pgx.Tx, a domain.Actor) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name,provider) VALUES($1,$2,$3,$4,'acme/repo','github')`, org, repoID, connection, nativeID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_jobs(org_id,id,connection_id,connection_version,namespace,kind,state) VALUES($1,$2,$3,1,'installation','scan','complete')`, org, jobID, connection); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO inventory_repository_state(org_id,repository_id,connection_id,namespace,connection_version,scan_id) VALUES($1,$2,$3,'installation',1,$4)`, org, repoID, connection, jobID)
		return err
	})
	return repoID
}

func githubSignatureHeaders(event, delivery, body string, secret []byte) http.Header {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	return http.Header{"Content-Type": {"application/json"}, "X-Github-Event": {event}, "X-Github-Delivery": {delivery}, "X-Hub-Signature-256": {"sha256=" + hex.EncodeToString(mac.Sum(nil))}}
}

func (h *webhookHarness) post(event, delivery, body string, secret []byte) int {
	h.t.Helper()
	headers := map[string]string{"Content-Type": "application/json", "X-GitHub-Event": event, "X-GitHub-Delivery": delivery, "X-Hub-Signature-256": "sha256=" + hex.EncodeToString(githubMAC(body, secret))}
	return identityRequest(h.server, "POST", "/hooks/github/app", body, nil, headers).Code
}

func (h *webhookHarness) postTenant(org, endpoint, event, delivery, body string, secret []byte) int {
	h.t.Helper()
	headers := map[string]string{"Content-Type": "application/json", "X-GitHub-Event": event, "X-GitHub-Delivery": delivery, "X-Hub-Signature-256": "sha256=" + hex.EncodeToString(githubMAC(body, secret))}
	return identityRequest(h.server, "POST", "/hooks/v1/"+org+"/"+endpoint, body, nil, headers).Code
}

func githubMAC(body string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	return mac.Sum(nil)
}

func (h *webhookHarness) state(id string) string {
	h.t.Helper()
	var state string
	err := h.db.Tenant(context.Background(), h.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT state FROM connections WHERE org_id=$1 AND id=$2`, h.org, id).Scan(&state)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return state
}

func (h *webhookHarness) bindingCount(connectionID string) int {
	h.t.Helper()
	var count int
	err := h.db.Tenant(context.Background(), h.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM github_installation_bindings WHERE org_id=$1 AND connection_id=$2`, h.org, connectionID).Scan(&count)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return count
}

func (h *webhookHarness) version(id string) int64 {
	h.t.Helper()
	var version int64
	err := h.db.Tenant(context.Background(), h.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT version FROM connections WHERE org_id=$1 AND id=$2`, h.org, id).Scan(&version)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return version
}

func (h *webhookHarness) auditCount() int {
	h.t.Helper()
	var count int
	err := h.db.Tenant(context.Background(), h.org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE org_id=$1 AND action LIKE 'connection.github_installation_%'`, h.org).Scan(&count)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return count
}

func (h *webhookHarness) stateOrg(org, id string) string {
	h.t.Helper()
	var state string
	err := h.db.Tenant(context.Background(), org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT state FROM connections WHERE org_id=$1 AND id=$2`, org, id).Scan(&state)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return state
}

func (h *webhookHarness) bindingCountOrg(org, connectionID string) int {
	h.t.Helper()
	var count int
	err := h.db.Tenant(context.Background(), org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM github_installation_bindings WHERE org_id=$1 AND connection_id=$2`, org, connectionID).Scan(&count)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return count
}

func (h *webhookHarness) refreshCount(org, repoID string) int {
	h.t.Helper()
	var count int
	err := h.db.Tenant(context.Background(), org, "", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh'`, org, repoID).Scan(&count)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return count
}

func TestHostedWebhookLifecycleFencing(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	deleted := `{"action":"deleted","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + strconv.FormatInt(h.accountID, 10) + `}}}`

	if code := h.post("installation", "d-forged", deleted, []byte("wrong-secret-value-000")); code != 401 {
		t.Fatalf("forged signature accepted: %d", code)
	}
	if code := h.post("installation", "d-unknown", `{"action":"deleted","installation":{"id":999999}}`, h.secret); code != 202 {
		t.Fatalf("unknown installation: %d", code)
	}
	if code := h.post("installation", "d-1", deleted, h.secret); code != 202 {
		t.Fatalf("uninstall: %d", code)
	}
	if got := h.state(h.connection); got != "revoked" {
		t.Fatalf("uninstall state: %s", got)
	}
	if h.bindingCount(h.connection) != 0 {
		t.Fatal("uninstall must release the binding")
	}
	version, audits := h.version(h.connection), h.auditCount()
	if code := h.post("installation", "d-1", deleted, h.secret); code != 202 {
		t.Fatalf("replay: %d", code)
	}
	if h.version(h.connection) != version || h.auditCount() != audits {
		t.Fatal("replayed delivery must not repeat version or audit")
	}

	other, _ := h.addConnection(h.org, h.instID, h.accountID)
	suspended := `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + strconv.FormatInt(h.accountID, 10) + `}}}`
	unsuspended := `{"action":"unsuspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + strconv.FormatInt(h.accountID, 10) + `}}}`
	if code := h.post("installation", "s-1", suspended, h.secret); code != 202 || h.state(other) != "disabled" {
		t.Fatalf("suspend: %d %s", code, h.state(other))
	}
	if code := h.post("installation", "u-1", unsuspended, h.secret); code != 202 || h.state(other) != "unverified" {
		t.Fatalf("unsuspend: %d %s", code, h.state(other))
	}

	h.actor(func(tx pgx.Tx, a domain.Actor) error {
		_, err := tx.Exec(context.Background(), `UPDATE connections SET state='revoked',revoked_at=now() WHERE org_id=$1 AND id=$2`, h.org, other)
		return err
	})
	if code := h.post("installation", "u-2", unsuspended, h.secret); code != 202 || h.state(other) != "revoked" {
		t.Fatalf("manually revoked connection was revived: %d %s", code, h.state(other))
	}
}

func TestHostedWebhookSignatureBindsBodyShape(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	before := h.state(h.connection)
	repoScoped := `{"action":"deleted","installation":{"id":` + inst + `,"account":{"id":` + strconv.FormatInt(h.accountID, 10) + `}},"repository":{"id":77,"full_name":"acme/repo"}}`
	if code := h.post("installation", "forge-1", repoScoped, h.secret); code != 202 {
		t.Fatalf("repository body with forged header: %d", code)
	}
	if h.state(h.connection) != before {
		t.Fatal("repository event must not be reinterpreted as App uninstall")
	}
	if h.bindingCount(h.connection) != 1 {
		t.Fatal("repository event must not release the binding")
	}
}

func TestHostedWebhookLifecycleIdentityBinding(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	acc := strconv.FormatInt(h.accountID, 10)
	before := h.state(h.connection)
	for _, tc := range []struct {
		name string
		body string
	}{
		{"app id mismatch", `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID+1, 10) + `,"account":{"id":` + acc + `}}}`},
		{"account mismatch", `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + strconv.FormatInt(h.accountID+1, 10) + `}}}`},
		{"account missing", `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := h.post("installation", "id-"+tc.name, tc.body, h.secret); code != 401 {
				t.Fatalf("forged identity accepted: %d", code)
			}
			if h.state(h.connection) != before {
				t.Fatalf("identity mismatch changed state: %s", h.state(h.connection))
			}
			if h.bindingCount(h.connection) != 1 {
				t.Fatal("identity mismatch released the binding")
			}
		})
	}
}

func TestHostedWebhookRepositoryDeltaNeverRevokes(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	acc := strconv.FormatInt(h.accountID, 10)
	before := h.state(h.connection)
	delta := `{"action":"deleted","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + acc + `}},"repositories_removed":[{"id":77}]}`
	if code := h.post("installation_repositories", "delta-1", delta, h.secret); code != 202 {
		t.Fatalf("repository delta: %d", code)
	}
	if h.state(h.connection) != before || h.bindingCount(h.connection) != 1 {
		t.Fatal("repository delta must not revoke or release the binding")
	}
	forged := `{"action":"deleted","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + acc + `}},"repositories_removed":[{"id":77}]}`
	if code := h.post("installation", "delta-2", forged, h.secret); code != 401 {
		t.Fatalf("delta with installation header must fail closed: %d", code)
	}
	if h.state(h.connection) != before || h.bindingCount(h.connection) != 1 {
		t.Fatal("forged delta header must not revoke")
	}
}

func TestHostedWebhookDeliveryConflict(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	acc := strconv.FormatInt(h.accountID, 10)
	suspended := `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + acc + `}}}`
	unsuspended := `{"action":"unsuspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + acc + `}}}`
	if code := h.post("installation", "dup-1", suspended, h.secret); code != 202 || h.state(h.connection) != "disabled" {
		t.Fatalf("suspend: %d %s", code, h.state(h.connection))
	}
	if code := h.post("installation", "dup-1", unsuspended, h.secret); code != 409 {
		t.Fatalf("changed body under same delivery id must conflict: %d", code)
	}
	if h.state(h.connection) != "disabled" {
		t.Fatalf("conflicting delivery applied: %s", h.state(h.connection))
	}
}

func TestHostedWebhookDeliveryIDBound(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	acc := strconv.FormatInt(h.accountID, 10)
	suspended := `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + acc + `}}}`
	if code := h.post("installation", strings.Repeat("a", 257), suspended, h.secret); code != 400 {
		t.Fatalf("oversize delivery id: %d", code)
	}
	if h.state(h.connection) != "healthy" {
		t.Fatalf("oversize delivery id applied: %s", h.state(h.connection))
	}
}

func TestHostedWebhookMissingEndpointFailsClosed(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	acc := strconv.FormatInt(h.accountID, 10)
	suspended := `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + acc + `}}}`
	h.actor(func(tx pgx.Tx, a domain.Actor) error {
		_, err := tx.Exec(context.Background(), `DELETE FROM inventory_webhooks WHERE org_id=$1 AND connection_id=$2`, h.org, h.connection)
		return err
	})
	if code := h.post("installation", "no-endpoint", suspended, h.secret); code != 409 {
		t.Fatalf("missing endpoint must fail closed and actionably, got %d", code)
	}
	if h.state(h.connection) != "healthy" {
		t.Fatalf("missing endpoint changed state: %s", h.state(h.connection))
	}
	h.actor(func(tx pgx.Tx, a domain.Actor) error {
		_, err := tx.Exec(context.Background(), `UPDATE connections SET state='revoked',revoked_at=now() WHERE org_id=$1 AND id=$2`, h.org, h.connection)
		return err
	})
	if code := h.post("installation", "revoked-endpoint", suspended, h.secret); code != 202 {
		t.Fatalf("revoked connection with missing endpoint must be ignored: %d", code)
	}
}

func TestHostedWebhookPullRequestTenantIsolation(t *testing.T) {
	h := newWebhookHarness(t)
	ctx := context.Background()
	orgB := h.addOrg()
	instB := h.instID + 1
	accB := h.accountID + 1
	connB, _ := h.addConnection(orgB, instB, accB)
	instA := strconv.FormatInt(h.instID, 10)
	repoID, jobID := domain.NewID(), domain.NewID()
	h.actor(func(tx pgx.Tx, a domain.Actor) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name,provider) VALUES($1,$2,$3,'77','acme/repo','github')`, h.org, repoID, h.connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_jobs(org_id,id,connection_id,connection_version,namespace,kind,state) VALUES($1,$2,$3,1,'installation','scan','complete')`, h.org, jobID, h.connection); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO inventory_repository_state(org_id,repository_id,connection_id,namespace,connection_version,scan_id) VALUES($1,$2,$3,'installation',1,$4)`, h.org, repoID, h.connection, jobID)
		return err
	})
	body := `{"action":"opened","installation":{"id":` + instA + `},"repository":{"id":77,"full_name":"acme/repo"},"pull_request":{"number":5,"head":{"sha":"abc"}}}`
	if code := h.post("pull_request", "pr-tenant", body, h.secret); code != 202 {
		t.Fatalf("pull request: %d", code)
	}
	if got := h.refreshCount(h.org, repoID); got != 1 {
		t.Fatalf("tenant A refresh: %d", got)
	}
	if got := h.refreshCount(orgB, repoID); got != 0 {
		t.Fatalf("tenant B must not be touched: %d", got)
	}
	if got := h.stateOrg(orgB, connB); got != "healthy" {
		t.Fatalf("tenant B connection changed: %s", got)
	}
}

func TestHostedWebhookPullRequestParityAndDedup(t *testing.T) {
	h := newWebhookHarness(t)
	ctx := context.Background()
	inst := strconv.FormatInt(h.instID, 10)
	repoID, jobID := domain.NewID(), domain.NewID()
	h.actor(func(tx pgx.Tx, a domain.Actor) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name,provider) VALUES($1,$2,$3,'77','acme/repo','github')`, h.org, repoID, h.connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_jobs(org_id,id,connection_id,connection_version,namespace,kind,state) VALUES($1,$2,$3,1,'installation','scan','complete')`, h.org, jobID, h.connection); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO inventory_repository_state(org_id,repository_id,connection_id,namespace,connection_version,scan_id) VALUES($1,$2,$3,'installation',1,$4)`, h.org, repoID, h.connection, jobID)
		return err
	})
	body := `{"action":"opened","installation":{"id":` + inst + `},"repository":{"id":77,"full_name":"acme/repo"},"pull_request":{"number":5,"head":{"sha":"abc"}}}`
	if code := h.post("pull_request", "pr-1", body, h.secret); code != 202 {
		t.Fatalf("pull request: %d", code)
	}
	count := func() int {
		var n int
		if err := h.db.Tenant(ctx, h.org, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh'`, h.org, repoID).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 1 {
		t.Fatalf("pull request must queue one refresh, got %d", count())
	}
	if code := h.post("pull_request", "pr-1", body, h.secret); code != 202 {
		t.Fatalf("pull request replay: %d", code)
	}
	if count() != 1 {
		t.Fatal("replayed delivery must be deduplicated")
	}
}

func TestHostedWebhookSecretRotation(t *testing.T) {
	h := newWebhookHarness(t)
	ctx := context.Background()
	inst := strconv.FormatInt(h.instID, 10)
	acc := strconv.FormatInt(h.accountID, 10)
	repoID := h.seedRepository(h.org, h.connection, "77")
	pr := `{"action":"opened","installation":{"id":` + inst + `},"repository":{"id":77,"full_name":"acme/repo"},"pull_request":{"number":5,"head":{"sha":"abc"}}}`

	rotated := []byte("rotated-hosted-webhook-secret")
	next := h.restart(rotated)

	if err := next.HandleWebhook(ctx, githubSignatureHeaders("pull_request", "rot-old", pr, h.secret), []byte(pr)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("old signature accepted after rotation: %v", err)
	}
	if err := next.HandleWebhook(ctx, githubSignatureHeaders("pull_request", "rot-new", pr, rotated), []byte(pr)); err != nil {
		t.Fatalf("rotated secret rejected for normal event: %v", err)
	}
	if got := h.refreshCount(h.org, repoID); got != 1 {
		t.Fatalf("rotated secret must queue exactly one refresh, got %d", got)
	}
	suspended := `{"action":"suspend","installation":{"id":` + inst + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + acc + `}}}`
	if err := next.HandleWebhook(ctx, githubSignatureHeaders("installation", "rot-suspend", suspended, rotated), []byte(suspended)); err != nil {
		t.Fatalf("rotated secret rejected for lifecycle: %v", err)
	}
	if got := h.state(h.connection); got != "disabled" {
		t.Fatalf("rotated lifecycle state: %s", got)
	}
}

func TestHostedWebhookPerTenantRouteRejected(t *testing.T) {
	h := newWebhookHarness(t)
	inst := strconv.FormatInt(h.instID, 10)
	repoID := h.seedRepository(h.org, h.connection, "77")
	pr := `{"action":"opened","installation":{"id":` + inst + `},"repository":{"id":77,"full_name":"acme/repo"},"pull_request":{"number":5,"head":{"sha":"abc"}}}`
	for _, tc := range []struct {
		name   string
		secret []byte
	}{
		{"stored secret", h.secret},
		{"rotated secret", []byte("rotated-hosted-webhook-secret")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := h.postTenant(h.org, h.webhookID, "pull_request", "tenant-"+tc.name, pr, tc.secret); code != 401 {
				t.Fatalf("per-tenant hosted route accepted %s: %d", tc.name, code)
			}
		})
	}
	if got := h.refreshCount(h.org, repoID); got != 0 {
		t.Fatalf("per-tenant hosted route queued refresh: %d", got)
	}
}

func TestManifestWebhookTenantIdentityBinding(t *testing.T) {
	h := newWebhookHarness(t)
	manifestSecret := "manifest-webhook-secret-value"
	manifestInst, manifestAcct := h.instID+1000, h.accountID+1000
	connection, webhookID := h.addManifestConnection(h.org, manifestInst, manifestAcct, manifestSecret)
	repoID := h.seedRepository(h.org, connection, "77")
	inst := strconv.FormatInt(manifestInst, 10)
	pr := `{"action":"opened","installation":{"id":` + inst + `},"repository":{"id":77,"full_name":"acme/repo"},"pull_request":{"number":5,"head":{"sha":"abc"}}}`
	if code := h.postTenant(h.org, webhookID, "pull_request", "m-pr", pr, []byte(manifestSecret)); code != 202 {
		t.Fatalf("manifest pull request: %d", code)
	}
	if got := h.refreshCount(h.org, repoID); got != 1 {
		t.Fatalf("manifest refresh: %d", got)
	}
	if code := h.postTenant(h.org, webhookID, "pull_request", "m-pr", pr, []byte(manifestSecret)); code != 202 {
		t.Fatalf("manifest replay: %d", code)
	}
	if got := h.refreshCount(h.org, repoID); got != 1 {
		t.Fatalf("manifest replay must deduplicate: %d", got)
	}
	audits := h.auditCount()
	foreign := `{"action":"opened","installation":{"id":` + strconv.FormatInt(manifestInst+1, 10) + `},"repository":{"id":77,"full_name":"acme/repo"},"pull_request":{"number":6,"head":{"sha":"def"}}}`
	if code := h.postTenant(h.org, webhookID, "pull_request", "m-foreign", foreign, []byte(manifestSecret)); code != 401 {
		t.Fatalf("foreign installation accepted: %d", code)
	}
	foreignLifecycle := `{"action":"suspend","installation":{"id":` + strconv.FormatInt(manifestInst+1, 10) + `,"app_id":` + strconv.FormatInt(h.appID, 10) + `,"account":{"id":` + strconv.FormatInt(manifestAcct, 10) + `}}}`
	if code := h.postTenant(h.org, webhookID, "installation", "m-foreign-life", foreignLifecycle, []byte(manifestSecret)); code != 401 {
		t.Fatalf("foreign lifecycle accepted: %d", code)
	}
	if got := h.refreshCount(h.org, repoID); got != 1 {
		t.Fatalf("foreign installation queued refresh: %d", got)
	}
	if got := h.stateOrg(h.org, connection); got != "healthy" {
		t.Fatalf("foreign installation changed state: %s", got)
	}
	if got := h.auditCount(); got != audits {
		t.Fatalf("foreign installation wrote audit: %d -> %d", audits, got)
	}
}
