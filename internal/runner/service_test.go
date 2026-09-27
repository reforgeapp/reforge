package runner_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/artifact"
	"reforge/internal/auth"
	"reforge/internal/config"
	"reforge/internal/domain"
	"reforge/internal/httpapi"
	"reforge/internal/runner"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

type fixture struct {
	db        *store.Store
	identity  *auth.Service
	workflow  *workflow.Service
	service   *runner.Service
	artifacts *artifact.Local
	session   auth.Session
	org       string
	repos     []string
	pool      runner.Pool
	policy    atomic.Value
	directory string
	cookie    string
	server    *httpapi.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Path != "/reforge_test" {
		t.Fatal("runner tests require disposable reforge_test database")
	}
	db, err := store.Open(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	identity, err := auth.New(context.Background(), db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, identity: identity, org: domain.NewID(), repos: []string{domain.NewID(), domain.NewID()}}
	f.session = auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}, CSRFToken: "fixture-csrf"}
	secret := make([]byte, 32)
	rand.Read(secret)
	f.cookie = base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(f.cookie))
	ctx := context.Background()
	err = db.Identity(ctx, f.session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'runner-test',$1::text,'Runner owner','owner@example.test')`, f.session.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,'fixture-csrf',clock_timestamp()+interval '1 hour')`, f.session.ID, f.session.User.ID, hex.EncodeToString(digest[:]))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Runner fixture')`, f.org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, f.org, f.session.User.ID); err != nil {
			return err
		}
		for _, id := range f.repos {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Runner repo')`, f.org, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.directory = filepath.Join(t.TempDir(), "artifacts")
	f.artifacts, err = artifact.NewLocal(db, f.directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.artifacts.Close() })
	f.policy.Store("runner-policy")
	f.workflow = workflow.New(db, identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) {
		return f.policy.Load().(string), nil
	})
	f.service = runner.New(db, identity, f.workflow, f.artifacts)
	f.workflow.RegisterScopeCheck(f.service.CheckScopeTx)
	f.pool, err = f.service.PutPool(ctx, f.session, f.org, "", runner.PoolInput{Name: "Approved pool", RepositoryIDs: f.repos[:1]}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	f.server = httpapi.New(config.Config{PublicURL: "http://127.0.0.1:8080", Development: true}, db)
	f.server.RegisterIdentity(identity)
	f.server.RegisterRunner(f.service)
	return f
}
func (f *fixture) enroll(t *testing.T) runner.Credential {
	t.Helper()
	grant, err := f.service.EnrollToken(context.Background(), f.session, f.org, f.pool.ID, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.service.Enroll(context.Background(), grant.Token, "fixture-worker")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func (f *fixture) enqueue(t *testing.T, repo, pool, branch string) workflow.Task {
	t.Helper()
	task, err := f.workflow.Enqueue(context.Background(), f.session, f.org, workflow.EnqueueInput{RepositoryID: repo, RunnerPoolID: pool, Recipe: "build-repair", RecipeVersion: "1", TargetBranch: branch, IdempotencyKey: domain.NewID()}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return task
}
func TestEnrollmentOneUseRotationAndTenantScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	grant, err := f.service.EnrollToken(ctx, f.session, f.org, f.pool.ID, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(grant)
	if bytes.Contains(raw, []byte(grant.Token)) || strings.Contains(fmt.Sprintf("%#v", grant), grant.Token) {
		t.Fatal("credential leaked by serialization")
	}
	var wg sync.WaitGroup
	success := make(chan runner.Credential, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := f.service.Enroll(ctx, grant.Token, "competing")
			if err == nil {
				success <- c
			} else if !errors.Is(err, auth.ErrUnauthenticated) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(success)
	if len(success) != 1 {
		t.Fatalf("one-use enrollment accepted %d uses", len(success))
	}
	c := <-success
	bad := strings.Replace(c.Token, f.org, domain.NewID(), 1)
	if _, err = f.service.Claim(ctx, bad); err == nil {
		t.Fatal("cross-tenant credential accepted")
	}
	rotated, err := f.service.Rotate(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Claim(ctx, c.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("old supervisor credential still valid")
	}
	if _, err = f.service.Claim(ctx, rotated.Token); !errors.Is(err, workflow.ErrNoWork) {
		t.Fatalf("rotated credential failed: %v", err)
	}
	if err = f.service.Revoke(ctx, f.session, f.org, c.Runner.ID, rotated.Runner.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Claim(ctx, rotated.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("revoked runner claimed")
	}
	expired, err := f.service.EnrollToken(ctx, f.session, f.org, f.pool.ID, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(expired.Token, "/")
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE runner_enrollments SET expires_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, parts[2])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Enroll(ctx, expired.Token, "expired"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("expired enrollment accepted")
	}
	for _, table := range []string{"runner_pools", "runner_pool_repositories", "runner_enrollments", "runners", "runner_job_credentials", "artifacts"} {
		var count int
		if err = f.db.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unscoped RLS leaked %s: %d %v", table, count, err)
		}
	}
}
func TestJobCredentialsFilterBeforeClaimAndRevokeFences(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	other, err := f.service.PutPool(ctx, f.session, f.org, "", runner.PoolInput{Name: "Other pool", RepositoryIDs: f.repos}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	f.enqueue(t, f.repos[1], f.pool.ID, "outside-repo")
	f.enqueue(t, f.repos[0], other.ID, "outside-pool")
	f.enqueue(t, f.repos[0], "", "unassigned")
	wanted := f.enqueue(t, f.repos[0], f.pool.ID, "main")
	assigned, err := f.service.Claim(ctx, c.Token)
	if err != nil || assigned.Task.ID != wanted.ID {
		t.Fatalf("claim did not filter scope before selection: %+v %v", assigned.Task, err)
	}
	if _, err = f.service.Heartbeat(ctx, c.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("supervisor token accepted as job authority")
	}
	if _, err = f.service.Claim(ctx, assigned.Credential.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("job token accepted for supervisor claim")
	}
	if _, err = f.service.Heartbeat(ctx, assigned.Credential.Token); err != nil {
		t.Fatal(err)
	}
	if err = f.service.Revoke(ctx, f.session, f.org, c.Runner.ID, c.Runner.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Upload(ctx, assigned.Credential.Token, "late.log", "text/plain", strings.NewReader("late")); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("revoked worker uploaded")
	}
	if _, err = f.workflow.Heartbeat(ctx, assigned.Lease, time.Minute); !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("revocation did not invalidate underlying fence: %v", err)
	}
	if _, err = f.service.Complete(ctx, assigned.Credential.Token, workflow.Completion{Outcome: "failed"}); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("revoked worker returned result")
	}
}
func TestArtifactsRefreshMembershipAndJobScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	f.enqueue(t, f.repos[0], f.pool.ID, "main")
	job, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	m, err := f.service.Upload(ctx, job.Credential.Token, "test.log", "text/plain", strings.NewReader(strings.Repeat("safe\n", 20000)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Upload(ctx, job.Credential.Token, "secret.log", "text/plain", strings.NewReader("token: "+job.Credential.Token)); !errors.Is(err, artifact.ErrContent) {
		t.Fatal("job credential persisted as artifact")
	}
	_, reader, err := f.service.Download(ctx, f.session, f.org, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	buffer := make([]byte, 32)
	if _, err = reader.Read(buffer); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, f.org, f.session.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Read(buffer); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("download continued after membership revocation: %v", err)
	}
	_, reader2, err := f.service.DownloadJob(ctx, job.Credential.Token, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reader2.Close()
	if _, err = reader2.Read(buffer); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE workflow_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, job.Lease.JobID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = reader2.Read(buffer); !errors.Is(err, workflow.ErrFence) {
		t.Fatal("artifact stream ignored expired lease")
	}
	if _, err = f.service.Upload(ctx, job.Credential.Token, "stale.log", "text/plain", strings.NewReader("stale")); !errors.Is(err, workflow.ErrFence) {
		t.Fatal("expired fence uploaded")
	}
}
func TestPreparedArtifactStaysUnavailableUntilMetadataIsRecorded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	f.enqueue(t, f.repos[0], f.pool.ID, "main")
	job, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := f.artifacts.Prepare(ctx, artifact.Metadata{OrgID: job.Lease.OrgID, RepositoryID: job.Lease.RepositoryID, TaskID: job.Lease.TaskID, Name: "staged.log", MediaType: "text/plain", ExpiresAt: time.Now().Add(time.Hour)}, strings.NewReader("staged output"))
	if err != nil {
		t.Fatal(err)
	}
	if _, reader, err := f.service.DownloadJob(ctx, job.Credential.Token, prepared.ID); !errors.Is(err, auth.ErrForbidden) || reader != nil {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatalf("unrecorded blob was downloadable: %v", err)
	}
	var recorded artifact.Metadata
	err = f.service.WithJob(ctx, job.Credential.Token, "artifact.upload", func(tx pgx.Tx, lease workflow.Lease, _ workflow.Task) error {
		var recordErr error
		recorded, recordErr = f.artifacts.RecordPreparedTx(ctx, tx, prepared, lease.AttemptID)
		return recordErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, reader, err := f.service.DownloadJob(ctx, job.Credential.Token, recorded.ID); err != nil || reader == nil {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatalf("recorded artifact unavailable: %v", err)
	} else {
		_ = reader.Close()
	}
}

func TestArtifactQuotaFailureRemovesPreparedBlob(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	f.enqueue(t, f.repos[0], f.pool.ID, "main")
	job, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, insertErr := tx.Exec(ctx, `INSERT INTO artifacts(org_id,id,repository_id,task_id,attempt_id,name,media_type,size,sha256,expires_at) SELECT $1,gen_random_uuid(),$2,$3,$4,'quota.log','text/plain',$5,repeat('a',64),clock_timestamp()+interval '1 day' FROM generate_series(1,256)`, f.org, job.Lease.RepositoryID, job.Task.ID, job.Lease.AttemptID, artifact.MaxSize)
		return insertErr
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Upload(ctx, job.Credential.Token, "over-quota.log", "text/plain", strings.NewReader("staged then rejected")); !errors.Is(err, artifact.ErrQuota) {
		t.Fatalf("quota result=%v", err)
	}
	blobs, err := filepath.Glob(filepath.Join(f.directory, f.org+"-*.data"))
	if err != nil || len(blobs) != 0 {
		t.Fatalf("rejected upload left blobs=%v err=%v", blobs, err)
	}
}

func TestWorkerHTTPRejectsBrowserOriginAndArbitraryProxy(t *testing.T) {
	f := newFixture(t)
	c := f.enroll(t)
	request := func(path, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8080"+path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+c.Token)
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		f.server.Router.ServeHTTP(w, r)
		return w
	}
	if got := request("/runner/v1/claim", "https://attacker.test"); got.Code != 403 {
		t.Fatalf("browser origin accepted: %d", got.Code)
	}
	if got := request("/runner/v1/claim", ""); got.Code != 204 {
		t.Fatalf("worker claim failed: %d %s", got.Code, got.Body.String())
	}
	if got := request("/runner/v1/proxy", ""); got.Code != 404 {
		t.Fatal("arbitrary proxy mounted")
	}
	if _, err := f.service.Broker(context.Background(), c.Token, "arbitrary", domain.NewID(), json.RawMessage(`{"url":"http://169.254.169.254"}`)); !errors.Is(err, runner.ErrUnsupported) {
		t.Fatal("unregistered private operation accepted")
	}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/orgs/"+f.org+"/runner-pools", nil)
	r.Header.Set("Authorization", "Bearer "+c.Token)
	w := httptest.NewRecorder()
	f.server.Router.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("runner credential gained browser authority")
	}
}

func TestPrivateOperationRevocationSerializesWithInvocation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	f.enqueue(t, f.repos[0], f.pool.ID, "main")
	job, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	connection := domain.NewID()
	otherConnection := domain.NewID()
	if err = f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		for _, id := range []string{connection, otherConnection} {
			if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'forge','gitea','Private fixture','https://forge.private','{}','healthy')`, f.org, id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO connection_routes(org_id,connection_id,runner_id,hostname,cidrs,approved_by) VALUES($1,$2,$3,'forge.private',ARRAY['10.42.0.0/24'],$4)`, f.org, id, c.Runner.ID, f.session.User.ID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE repositories SET connection_id=$3 WHERE org_id=$1 AND id=$2`, f.org, f.repos[0], connection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	if err = f.service.RegisterOperation("fixture.read", func(ctx context.Context, tx pgx.Tx, operation runner.Operation) (json.RawMessage, error) {
		if operation.RunnerID != c.Runner.ID || operation.PoolID != f.pool.ID || operation.Lease.RepositoryID != f.repos[0] || operation.ConnectionID != connection {
			return nil, auth.ErrForbidden
		}
		if string(operation.Input) != "{}" {
			return nil, auth.ErrInvalid
		}
		close(entered)
		select {
		case <-release:
			return json.RawMessage(`{"fixture":"authority-boundary-only"}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Broker(ctx, job.Credential.Token, "fixture.read", otherConnection, json.RawMessage(`{}`)); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("unbound connection reached private handler")
	}
	invocation := make(chan error, 1)
	go func() {
		_, err := f.service.Broker(ctx, job.Credential.Token, "fixture.read", connection, json.RawMessage(`{}`))
		invocation <- err
	}()
	select {
	case <-entered:
	case err := <-invocation:
		t.Fatalf("registered operation not invoked: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("operation timed out")
	}
	revoked := make(chan error, 1)
	go func() { revoked <- f.service.Revoke(ctx, f.session, f.org, c.Runner.ID, c.Runner.Version, "fixture") }()
	select {
	case err := <-revoked:
		t.Fatalf("revocation crossed in-flight operation boundary: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	if err = <-invocation; err != nil {
		t.Fatal(err)
	}
	if err = <-revoked; err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Broker(ctx, job.Credential.Token, "fixture.read", connection, json.RawMessage(`{}`)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("revoked runner reached private handler")
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error { return f.service.CheckRunnerTx(ctx, tx, f.org, c.Runner.ID) }); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("revoked runner passed private-route enrollment check")
	}
}
func TestCancellationAndPoolGrantChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	f.enqueue(t, f.repos[0], f.pool.ID, "main")
	job, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.workflow.Cancel(ctx, f.session, f.org, job.Task.ID, job.Task.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Progress(ctx, job.Credential.Token, domain.TaskPlanning); !errors.Is(err, workflow.ErrPaused) {
		t.Fatalf("cancelling task accepted execution progress: %v", err)
	}
	artifact, err := f.service.Upload(ctx, job.Credential.Token, "cancelled.log", "text/plain", strings.NewReader("stopped after cancellation"))
	if err != nil {
		t.Fatalf("cancelling task rejected final log: %v", err)
	}
	if _, reader, downloadErr := f.service.DownloadJob(ctx, job.Credential.Token, artifact.ID); !errors.Is(downloadErr, workflow.ErrPaused) || reader != nil {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatalf("cancelling task downloaded artifact: %v", downloadErr)
	}
	if _, err = f.service.Complete(ctx, job.Credential.Token, workflow.Completion{Outcome: "unknown"}); err == nil {
		t.Fatal("cancellation accepted invalid completion outcome")
	}
	stopped, err := f.service.Complete(ctx, job.Credential.Token, workflow.Completion{Outcome: "failed"})
	if err != nil || stopped.State != domain.TaskCancelled {
		t.Fatalf("confirmed stop not recorded: %+v %v", stopped, err)
	}
	if _, err = f.service.Heartbeat(ctx, job.Credential.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("finished credential still active")
	}
	if _, err = f.service.Upload(ctx, job.Credential.Token, "late.log", "text/plain", strings.NewReader("late")); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("finished credential uploaded after cancellation")
	}
	f.enqueue(t, f.repos[0], f.pool.ID, "next")
	next, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PutPool(ctx, f.session, f.org, f.pool.ID, runner.PoolInput{Name: f.pool.Name, RepositoryIDs: f.repos[1:]}, f.pool.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Heartbeat(ctx, next.Credential.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("removed repository grant retained job authority")
	}
	if _, err = f.workflow.Heartbeat(ctx, next.Lease, time.Minute); !errors.Is(err, workflow.ErrFence) {
		t.Fatal("pool grant change retained old fence")
	}
}
func TestArtifactRetentionAndOrphanCleanup(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	f.enqueue(t, f.repos[0], f.pool.ID, "main")
	job, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	m, err := f.service.Upload(ctx, job.Credential.Token, "expire.log", "text/plain", strings.NewReader("expired output"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE artifacts SET expires_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, m.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if count, err := f.artifacts.DeleteByRetention(ctx, f.org, time.Now()); err != nil || count != 1 {
		t.Fatalf("retention failed: %d %v", count, err)
	}
	if _, _, err = f.service.DownloadJob(ctx, job.Credential.Token, m.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("expired artifact still downloadable")
	}
	orphan := filepath.Join(f.directory, f.org+"-"+domain.NewID()+".data")
	if err = os.WriteFile(orphan, []byte("orphaned write after crashed transaction"), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err = os.Chtimes(orphan, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if count, err := f.artifacts.SweepOrphans(ctx, time.Now().Add(-time.Hour)); err != nil || count != 1 {
		t.Fatalf("orphan sweep failed: %d %v", count, err)
	}
	if _, err = os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphan blob remains")
	}
}

func TestJobMethodExpiryAndPolicyRevocation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.enroll(t)
	f.enqueue(t, f.repos[0], f.pool.ID, "main")
	job, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Split(job.Credential.Token, "/")[2]
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE runner_job_credentials SET methods=ARRAY['heartbeat'] WHERE org_id=$1 AND id=$2`, f.org, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Upload(ctx, job.Credential.Token, "unpermitted.log", "text/plain", strings.NewReader("test")); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("job method allowlist bypassed")
	}
	if _, err = f.service.Heartbeat(ctx, job.Credential.Token); err != nil {
		t.Fatal(err)
	}
	f.policy.Store("changed-policy")
	if _, err = f.service.Heartbeat(ctx, job.Credential.Token); !errors.Is(err, workflow.ErrPolicy) {
		t.Fatalf("policy change did not revoke job: %v", err)
	}
	task, err := f.workflow.Get(ctx, f.session, f.org, job.Task.ID)
	if err != nil || task.State != domain.TaskBlocked {
		t.Fatalf("policy block was not durable: %+v %v", task, err)
	}
	if _, err = f.workflow.Heartbeat(ctx, job.Lease, time.Minute); !errors.Is(err, workflow.ErrFence) {
		t.Fatal("policy revocation retained lease")
	}
	f.policy.Store("runner-policy")
	f.enqueue(t, f.repos[0], f.pool.ID, "other")
	next, err := f.service.Claim(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	nextID := strings.Split(next.Credential.Token, "/")[2]
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE runner_job_credentials SET expires_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2`, f.org, nextID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Heartbeat(ctx, next.Credential.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("expired job token renewed itself")
	}
}
func TestScopedOwnerCannotEnrollOrReplaceUnpermittedPool(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.session.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.EnrollToken(ctx, f.session, f.org, f.pool.ID, "fixture"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("scoped owner enrolled inaccessible repositories")
	}
	if _, err := f.service.PutPool(ctx, f.session, f.org, f.pool.ID, runner.PoolInput{Name: f.pool.Name, RepositoryIDs: f.repos[:1]}, f.pool.Version, "fixture"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("scoped owner replaced inaccessible pool")
	}
	if list, err := f.service.Pools(ctx, f.session, f.org, runner.PoolFilter{}, 100, ""); err != nil || len(list.Items) != 0 {
		t.Fatal("scoped owner listed inaccessible pool")
	}
}

func TestEnrollmentRechecksIssuerAndInvalidatesChangedPool(t *testing.T) {
	for _, change := range []string{"logout", "role", "scope", "pool"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			grant, err := f.service.EnrollToken(ctx, f.session, f.org, f.pool.ID, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "logout":
				err = f.identity.Logout(ctx, f.session)
			case "role":
				err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, `UPDATE memberships SET role='admin' WHERE org_id=$1 AND user_id=$2`, f.org, f.session.User.ID)
					return err
				})
			case "scope":
				err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.session.User.ID)
					return err
				})
			case "pool":
				_, err = f.service.PutPool(ctx, f.session, f.org, f.pool.ID, runner.PoolInput{Name: f.pool.Name, RepositoryIDs: f.repos}, f.pool.Version, "fixture")
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.service.Enroll(ctx, grant.Token, "delegated-setup")
			if change == "logout" {
				if err != nil {
					t.Fatalf("browser logout invalidated delegated enrollment: %v", err)
				}
			} else if !errors.Is(err, auth.ErrUnauthenticated) {
				t.Fatalf("changed issuer or pool retained enrollment authority: %v", err)
			}
		})
	}
	f := newFixture(t)
	ctx := context.Background()
	team := domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO teams(org_id,id,name) VALUES($1,$2,'Issuer scope')`, f.org, team); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO team_memberships(org_id,team_id,user_id) VALUES($1,$2,$3)`, f.org, team, f.session.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO team_repositories(org_id,team_id,repository_id) VALUES($1,$2,$3)`, f.org, team, f.repos[0]); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.session.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.enroll(t)
	grant, err := f.service.EnrollToken(ctx, f.session, f.org, f.pool.ID, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM team_memberships WHERE org_id=$1 AND team_id=$2 AND user_id=$3`, f.org, team, f.session.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Enroll(ctx, grant.Token, "removed-team"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("removed issuing-owner team grant remained effective")
	}
}
func TestRunnerInventoryPagination(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := f.service.PutPool(ctx, f.session, f.org, "", runner.PoolInput{Name: "Additional", RepositoryIDs: f.repos}, 0, "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := f.service.Pools(ctx, f.session, f.org, runner.PoolFilter{}, 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || seen[page.Items[0].ID] {
			t.Fatal("pool pagination omitted or repeated inventory")
		}
		seen[page.Items[0].ID] = true
		if page.Complete {
			break
		}
		if page.NextCursor == "" {
			t.Fatal("incomplete pool inventory has no cursor")
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("pool inventory truncated: %d", len(seen))
	}
	for i := 0; i < 3; i++ {
		f.enroll(t)
	}
	cursor = ""
	seen = map[string]bool{}
	for {
		page, err := f.service.Runners(ctx, f.session, f.org, f.pool.ID, 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || seen[page.Items[0].ID] {
			t.Fatal("runner pagination omitted or repeated inventory")
		}
		seen[page.Items[0].ID] = true
		if page.Complete {
			break
		}
		if page.NextCursor == "" {
			t.Fatal("incomplete runner inventory has no cursor")
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("runner inventory truncated: %d", len(seen))
	}
}

func TestBuiltinPoolCoversRepositoriesAndReplacesRunner(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first, err := f.service.EnrollBuiltin(ctx, f.org, "built-in", 1)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := f.service.Pool(ctx, f.session, f.org, first.Runner.PoolID)
	if err != nil || !pool.Builtin || len(pool.RepositoryIDs) != 2 {
		t.Fatalf("built-in pool %+v: %v", pool, err)
	}
	added := domain.NewID()
	if err = f.db.Tenant(ctx, f.org, f.session.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Added repo')`, f.org, added)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if pool, _ = f.service.Pool(ctx, f.session, f.org, pool.ID); len(pool.RepositoryIDs) != 3 {
		t.Fatal("new repository not granted to built-in pool")
	}
	if pool, err = f.service.PutPool(ctx, f.session, f.org, pool.ID, runner.PoolInput{Name: "Renamed", State: "draining"}, pool.Version, "fixture"); err != nil || pool.Name != "Built-in" || len(pool.RepositoryIDs) != 3 {
		t.Fatalf("built-in pool edit %+v: %v", pool, err)
	}
	if _, err = f.service.EnrollBuiltin(ctx, f.org, "built-in", 1); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("draining built-in pool enrolled: %v", err)
	}
	if _, err = f.service.PutPool(ctx, f.session, f.org, pool.ID, runner.PoolInput{Name: "Built-in", State: "revoked"}, pool.Version, "fixture"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("built-in pool revoked: %v", err)
	}
	if _, err = f.service.PutPool(ctx, f.session, f.org, pool.ID, runner.PoolInput{Name: "Built-in", State: "active"}, pool.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	second, err := f.service.EnrollBuiltin(ctx, f.org, "built-in", 1)
	if err != nil || second.Runner.PoolID != pool.ID {
		t.Fatalf("re-enrol: %v", err)
	}
	if _, err = f.service.Claim(ctx, first.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("replaced built-in runner still valid")
	}
	if _, err = f.service.Claim(ctx, second.Token); !errors.Is(err, workflow.ErrNoWork) {
		t.Fatalf("built-in runner claim: %v", err)
	}
	orgs, err := f.service.BuiltinOrgs(ctx)
	if err != nil || !slices.Contains(orgs, f.org) {
		t.Fatalf("built-in orgs: %v", err)
	}
}
