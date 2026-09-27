package runner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"reforge/internal/artifact"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

func TestPreparedUploadRejectedAfterRunnerRevocation(t *testing.T) {
	databaseURL := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REFORGE_TEST_DATABASE_URL required")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || parsed.Path != "/reforge_test" {
		t.Fatal("runner tests require disposable reforge_test database")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	org, repo := domain.NewID(), domain.NewID()
	session := auth.Session{ID: domain.NewID(), User: auth.User{ID: domain.NewID()}, CSRFToken: "fixture-csrf"}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	cookie := base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(cookie))
	if err = db.Identity(ctx, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'runner-test',$1::text,'Runner owner','owner@example.test')`, session.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,'fixture-csrf',clock_timestamp()+interval '1 hour')`, session.ID, session.User.ID, hex.EncodeToString(digest[:]))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Runner fixture')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, session.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2::uuid,$2::text,'Runner repo')`, org, repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "artifacts")
	artifacts, err := artifact.NewLocal(db, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifacts.Close() })
	workflowService := workflow.New(db, identity, func(context.Context, pgx.Tx, workflow.Task, string) (string, error) { return "runner-policy", nil })
	service := New(db, identity, workflowService, artifacts)
	workflowService.RegisterScopeCheck(service.CheckScopeTx)
	pool, err := service.PutPool(ctx, session, org, "", PoolInput{Name: "Approved pool", RepositoryIDs: []string{repo}}, 0, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := service.EnrollToken(ctx, session, org, pool.ID, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := service.Enroll(ctx, grant.Token, "fixture-worker")
	if err != nil {
		t.Fatal(err)
	}
	task, err := workflowService.Enqueue(ctx, session, org, workflow.EnqueueInput{RepositoryID: repo, RunnerPoolID: pool.ID, Recipe: "build-repair", RecipeVersion: "1", TargetBranch: "main", IdempotencyKey: domain.NewID()}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Claim(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	if job.Task.ID != task.ID {
		t.Fatal("unexpected claimed task")
	}
	prepared, err := artifacts.Prepare(ctx, artifact.Metadata{OrgID: job.Lease.OrgID, RepositoryID: job.Lease.RepositoryID, TaskID: job.Lease.TaskID, Name: "staged.log", MediaType: "text/plain", ExpiresAt: time.Now().Add(time.Hour)}, strings.NewReader("staged output"))
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Revoke(ctx, session, org, credential.Runner.ID, credential.Runner.Version, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.recordPreparedUpload(ctx, job.Credential.Token, prepared, job.Lease.AttemptID); !errors.Is(err, auth.ErrUnauthenticated) && !errors.Is(err, workflow.ErrFence) {
		t.Fatalf("revoked prepared upload result=%v", err)
	}
	if _, err = os.Stat(filepath.Join(directory, org+"-"+prepared.ID+".data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revoked prepared blob remains: %v", err)
	}
}
