package privateconnector_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"reforge/internal/auth"
	"reforge/internal/config"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/httpapi"
	"reforge/internal/network"
	"reforge/internal/privateconnector"
	"reforge/internal/runner"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

type realFixture struct {
	db                    *store.Store
	service               *runner.Service
	connector             *privateconnector.Connector
	server                *httptest.Server
	session               auth.Session
	org, repo, connection string
	pool                  runner.Pool
	supervisor            runner.Credential
	providerSecret        string
}

func real(t *testing.T) *realFixture {
	t.Helper()
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	root := os.Getenv("REFORGE_TEST_ROOT")
	if raw == "" || os.Getenv("REFORGE_PRIVATE_GITEA_TEST") != "1" {
		t.Skip("explicit disposable database and local Gitea fixture required")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Path != "/reforge_test" {
		t.Fatal("private connector requires disposable reforge_test database")
	}
	if root == "" {
		t.Fatal("REFORGE_TEST_ROOT required")
	}
	db, e := store.Open(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	identity, e := auth.New(context.Background(), db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if e != nil {
		t.Fatal(e)
	}
	f := &realFixture{db: db, org: domain.NewID(), repo: domain.NewID(), connection: domain.NewID(), session: auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}, CSRFToken: "private-fixture"}}
	random := make([]byte, 32)
	rand.Read(random)
	digest := sha256.Sum256([]byte(base64.RawURLEncoding.EncodeToString(random)))
	ctx := context.Background()
	e = db.Identity(ctx, f.session.User.ID, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'private-fixture',$1::text,'Private fixture owner','private@example.test')`, f.session.User.ID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,'private-fixture',clock_timestamp()+interval '1 hour')`, f.session.ID, f.session.User.ID, hex.EncodeToString(digest[:]))
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	e = db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Private connector fixture')`, f.org); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, f.org, f.session.User.ID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Private source')`, f.org, f.repo)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	jobs := workflow.New(db, identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return "private-fixture-policy", nil
	})
	f.service = runner.New(db, identity, jobs, nil)
	f.pool, e = f.service.PutPool(ctx, f.session, f.org, "", runner.PoolInput{Name: "Private pool", RepositoryIDs: []string{f.repo}}, 0, "private-fixture")
	if e != nil {
		t.Fatal(e)
	}
	f.supervisor = f.enroll(t)
	b, e := os.ReadFile(filepath.Join(root, ".local/gitea/reforge-bot.token"))
	if e != nil {
		t.Fatal(e)
	}
	f.providerSecret = strings.TrimSpace(string(b))
	e = db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'forge','gitea','Private Gitea fixture','http://127.0.0.1:53000','{}','healthy')`, f.org, f.connection); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO connection_routes(org_id,connection_id,runner_id,hostname,cidrs,approved_by) VALUES($1,$2,$3,'127.0.0.1',ARRAY['127.0.0.1/32'],$4)`, f.org, f.connection, f.supervisor.Runner.ID, f.session.User.ID)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	f.connector, e = privateconnector.New(privateconnector.Config{Authenticate: f.service.AuthenticateSupervisor, TTL: 3 * time.Second, Development: true})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(f.connector.Close)
	f.server = httptest.NewUnstartedServer(nil)
	api := httpapi.New(config.Config{PublicURL: "http://" + f.server.Listener.Addr().String(), Development: true}, db)
	api.RegisterPrivateConnector(f.connector)
	f.server.Config.Handler = api.Router
	f.server.Start()
	t.Cleanup(f.server.Close)
	return f
}
func (f *realFixture) enroll(t *testing.T) runner.Credential {
	t.Helper()
	grant, e := f.service.EnrollToken(context.Background(), f.session, f.org, f.pool.ID, "private-fixture")
	if e != nil {
		t.Fatal(e)
	}
	identity, e := f.service.Enroll(context.Background(), grant.Token, "private-fixture-supervisor")
	if e != nil {
		t.Fatal(e)
	}
	return identity
}
func (f *realFixture) target() privateconnector.Target {
	return privateconnector.Target{OrgID: f.org, RunnerID: f.supervisor.Runner.ID}
}
func (f *realFixture) authorize(op privateconnector.Operation, afterReady func()) privateconnector.Authorize {
	return func(ctx context.Context, ready privateconnector.Ready, deliver privateconnector.Deliver) error {
		if afterReady != nil {
			afterReady()
		}
		return f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
			var org string
			if e := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, f.org).Scan(&org); e != nil {
				return e
			}
			if e := f.service.ValidatePrivateSupervisorTx(ctx, tx, ready.Runner, ready.CredentialHash); e != nil {
				return e
			}
			connection := privateconnector.Connection{OrgID: f.org, ID: f.connection, Secret: f.providerSecret}
			var runnerID, host string
			var cidrs []string
			if e := tx.QueryRow(ctx, `SELECT c.provider,c.endpoint,c.version,c.credential_version,r.runner_id::text,r.hostname,r.cidrs FROM connections c JOIN connection_routes r ON r.org_id=c.org_id AND r.connection_id=c.id WHERE c.org_id=$1 AND c.id=$2 AND c.state='healthy' AND r.revoked_at IS NULL FOR SHARE OF c,r`, f.org, f.connection).Scan(&connection.Provider, &connection.Endpoint, &connection.Version, &connection.CredentialVersion, &runnerID, &host, &cidrs); e != nil {
				return e
			}
			if runnerID != ready.ID {
				return auth.ErrForbidden
			}
			connection.Route = network.PrivateRoute{OrgID: f.org, ConnectionID: f.connection, RunnerID: runnerID, Host: host, CIDRs: cidrs}
			if _, e := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id) VALUES($1::uuid,$2,$3,'private.fixture.authorized',$4,$1::text)`, op.ID, f.org, "runner:"+ready.ID, f.connection); e != nil {
				return e
			}
			_, e := deliver(privateconnector.GrantSpec{RunnerVersion: ready.Version, CredentialHash: ready.CredentialHash, OperationID: op.ID, AuthorityID: op.ID, Connection: connection})
			return e
		})
	}
}

type helperConfig struct {
	Endpoint   string
	Credential string
	Target     privateconnector.Target
	Count      int
}

func TestPrivateSupervisorProcess(t *testing.T) {
	if os.Getenv("REFORGE_PRIVATE_SUPERVISOR_HELPER") != "1" {
		t.Skip("isolated supervisor helper")
	}
	var cfg helperConfig
	if e := json.NewDecoder(os.Stdin).Decode(&cfg); e != nil {
		t.Fatal("invalid helper input")
	}
	client, e := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: cfg.Endpoint, Credential: cfg.Credential, Target: cfg.Target, Development: true})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for done := 0; done < cfg.Count; {
		e := client.RunOnce(ctx)
		if e == nil {
			done++
			continue
		}
		if (errors.Is(e, privateconnector.ErrUnavailable) || errors.Is(e, privateconnector.ErrConflict)) && ctx.Err() == nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		t.Fatal(e)
	}
}
func TestRealEnrolledSupervisorGiteaAndLockedAuthorization(t *testing.T) {
	f := real(t)
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cfg, _ := json.Marshal(helperConfig{Endpoint: f.server.URL, Credential: f.supervisor.Token, Target: f.target(), Count: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestPrivateSupervisorProcess$", "-test.count=1")
	child.Env = []string{"PATH=/usr/bin:/bin", "REFORGE_PRIVATE_SUPERVISOR_HELPER=1"}
	child.Stdin = bytes.NewReader(cfg)
	var output bytes.Buffer
	child.Stdout = &output
	child.Stderr = &output
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if child.Process != nil {
			_ = child.Process.Kill()
		}
	})
	probe := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.GiteaProbe}
	result, e := f.connector.Dispatch(ctx, f.target(), probe, f.authorize(probe, nil))
	if e != nil || result.Capabilities == nil || result.Capabilities.ServerVersion != "1.27.3" {
		t.Fatalf("real private probe failed: %v", e)
	}
	inventory := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.GiteaInventory, Inventory: &privateconnector.InventoryArgs{Limit: 100}}
	result, e = f.connector.Dispatch(ctx, f.target(), inventory, f.authorize(inventory, nil))
	if e != nil || result.Inventory == nil {
		t.Fatalf("real private inventory failed: %v", e)
	}
	if e = child.Wait(); e != nil {
		t.Fatalf("separate supervisor failed: %v", e)
	}
	if bytes.Contains(output.Bytes(), []byte(f.providerSecret)) || bytes.Contains(output.Bytes(), []byte(f.supervisor.Token)) {
		t.Fatal("subprocess logs leaked a credential")
	}
	if result.OperationID != inventory.ID {
		t.Fatal("operation identity lost")
	}
}
func TestRealRevokedAndWrongSupervisorCannotReceiveGrant(t *testing.T) {
	f := real(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	other := f.enroll(t)
	client, e := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: f.server.URL, Credential: other.Token, Target: privateconnector.Target{OrgID: f.org, RunnerID: other.Runner.ID}, Development: true})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	polled := make(chan error, 1)
	go func() { _, e := client.Poll(ctx); polled <- e }()
	var authorized atomic.Bool
	op := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.GiteaProbe}
	short, stop := context.WithTimeout(ctx, 150*time.Millisecond)
	defer stop()
	if _, e = f.connector.Dispatch(short, f.target(), op, func(context.Context, privateconnector.Ready, privateconnector.Deliver) error {
		authorized.Store(true)
		return nil
	}); !errors.Is(e, privateconnector.ErrUnavailable) || authorized.Load() {
		t.Fatalf("other runner received operation: %v", e)
	}
	if e = f.service.Revoke(ctx, f.session, f.org, f.supervisor.Runner.ID, f.supervisor.Runner.Version, "private-fixture"); e != nil {
		t.Fatal(e)
	}
	revoked, e := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: f.server.URL, Credential: f.supervisor.Token, Target: f.target(), Development: true})
	if e != nil {
		t.Fatal(e)
	}
	defer revoked.Close()
	if _, e = revoked.Poll(ctx); !errors.Is(e, auth.ErrUnauthenticated) {
		t.Fatalf("revoked supervisor polled: %v", e)
	}
	if e = <-polled; !errors.Is(e, privateconnector.ErrUnavailable) {
		t.Fatalf("other runner received grant: %v", e)
	}
}
func TestRealResultCompletesWhileRevocationWaits(t *testing.T) {
	f := real(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client, e := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: f.server.URL, Credential: f.supervisor.Token, Target: f.target(), Development: true})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	op := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.GiteaProbe}
	finished := make(chan error, 1)
	go func() { _, e := f.connector.Dispatch(ctx, f.target(), op, f.authorize(op, nil)); finished <- e }()
	grant, e := client.Poll(ctx)
	if e != nil {
		t.Fatal(e)
	}
	revoked := make(chan error, 1)
	go func() {
		revoked <- f.service.Revoke(ctx, f.session, f.org, f.supervisor.Runner.ID, f.supervisor.Runner.Version, "private-fixture")
	}()
	select {
	case e := <-revoked:
		t.Fatalf("revocation crossed active authorization lock: %v", e)
	case <-time.After(40 * time.Millisecond):
	}
	result := privateconnector.Result{OperationID: op.ID, Capabilities: &forge.Capabilities{Provider: "fixture-result-channel-only"}}
	if e = client.Complete(ctx, grant, result); e != nil {
		t.Fatal(e)
	}
	if e = <-finished; e != nil {
		t.Fatal(e)
	}
	if e = <-revoked; e != nil {
		t.Fatal(e)
	}
}

func TestRealSixMiBResultBoundary(t *testing.T) {
	f := real(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client, e := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: f.server.URL, Credential: f.supervisor.Token, Target: f.target(), Development: true})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	op := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.GiteaReadFile, File: &privateconnector.FileArgs{Repository: forge.RepoRef{NativeID: "1", FullName: "fixture/source"}, CommitSHA: strings.Repeat("a", 40), Path: "large.bin"}}
	finished := make(chan error, 1)
	go func() {
		result, e := f.connector.Dispatch(ctx, f.target(), op, f.authorize(op, nil))
		if e == nil && (result.File == nil || len(result.File.Content) != 4<<20) {
			e = errors.New("large result incomplete")
		}
		finished <- e
	}()
	grant, e := client.Poll(ctx)
	if e != nil {
		t.Fatal(e)
	}
	result := privateconnector.Result{OperationID: op.ID, File: &forge.File{Path: "large.bin", SHA: strings.Repeat("b", 40), Content: bytes.Repeat([]byte("x"), 4<<20)}}
	if e = client.Complete(ctx, grant, result); e != nil {
		t.Fatal(e)
	}
	if e = <-finished; e != nil {
		t.Fatal(e)
	}
}

func TestRealResultCompletesWithSaturatedDatabasePool(t *testing.T) {
	f := real(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client, err := privateconnector.NewClient(privateconnector.ClientConfig{Endpoint: f.server.URL, Credential: f.supervisor.Token, Target: f.target(), Development: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	op := privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.GiteaProbe}
	done := make(chan error, 1)
	base := f.authorize(op, nil)
	go func() {
		_, err := f.connector.Dispatch(ctx, f.target(), op, func(ctx context.Context, ready privateconnector.Ready, deliver privateconnector.Deliver) error {
			return base(ctx, ready, func(spec privateconnector.GrantSpec) (privateconnector.Result, error) {
				var borrowed []*pgxpool.Conn
				defer func() {
					for _, conn := range borrowed {
						conn.Release()
					}
				}()
				for i := int32(1); i < f.db.Pool.Config().MaxConns; i++ {
					conn, err := f.db.Pool.Acquire(ctx)
					if err != nil {
						return privateconnector.Result{}, err
					}
					borrowed = append(borrowed, conn)
				}
				return deliver(spec)
			})
		})
		done <- err
	}()
	grant, err := client.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if f.db.Pool.Stat().IdleConns() != 0 {
		t.Fatal("pool was not saturated during grant")
	}
	if err = client.Complete(ctx, grant, privateconnector.Result{OperationID: op.ID, Capabilities: &forge.Capabilities{Provider: "gitea"}}); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
