package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/delivery"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/attention"
	"github.com/reforgeapp/reforge/pkg/release"
	"github.com/reforgeapp/reforge/pkg/store"
)

func TestDeliveryHoldIsDurableScopedAndResolvesWithOperation(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", ListenAddress: "127.0.0.1:8080", Edition: "self-hosted", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	org, user, connection := domain.NewID(), domain.NewID(), domain.NewID()
	repos := []string{domain.NewID(), domain.NewID()}
	ids := []string{domain.NewID(), domain.NewID()}
	session := auth.Session{ID: domain.NewID(), User: auth.User{ID: user}}
	if err = db.Identity(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1::uuid,'delivery-hold-test',$1::text,'Owner','owner@example.test')`, user)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, session.ID, user, domain.NewID(), domain.NewID())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Delivery holds')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',false)`, org, user); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state) VALUES($1,$2,'forge','gitea','Fixture','https://forge.example','{}','healthy')`, org, connection); err != nil {
			return err
		}
		for i, repo := range repos {
			if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name,connection_id,default_branch) VALUES($1,$2::uuid,$2::text,$2::text,$3,'main')`, org, repo, connection); err != nil {
				return err
			}
			p := deployment.Provenance{OrgID: org, RepositoryID: repo, Kind: "native", Environment: "production", SourceSHA: strings.Repeat("a", 40), ArtifactDigest: "sha256:" + strings.Repeat("b", 64), BuildID: "ci-1", ConfigurationVersion: 1, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
			body, _ := json.Marshal(deployment.SignedProvenance{Document: p})
			if _, err := tx.Exec(ctx, `INSERT INTO release_receipts(org_id,id,repository_id,environment,kind,configuration_version,source_sha,artifact_digest,build_id,document) VALUES($1,$2,$3,'production','native',1,$4,$5,'ci-1',$6)`, org, ids[i], repo, p.SourceSHA, p.ArtifactDigest, body); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, org, user, repos[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{db: db, auth: identity, Delivery: &delivery.Service{Releases: &release.Service{DB: db, Auth: identity}}}
	hold := &domain.ProviderError{Kind: "unsupported", Message: "Operator: qualify deployment workflow"}
	for i := 0; i < 2; i++ {
		if err = service.deliveryOutcome(ctx, session, org, ids[0], "", hold); err != nil {
			t.Fatal(err)
		}
	}
	if err = service.deliveryOutcome(ctx, session, org, ids[1], "", hold); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("hidden delivery exposed: %v", err)
	}
	if err = identity.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		page, err := attention.ListTx(ctx, tx, a, 10, "")
		if err != nil {
			return err
		}
		if len(page.Items) != 1 || page.Items[0].RepositoryID != repos[0] || page.Items[0].Label != "Connect deployment" || page.Items[0].Owner != "operator" {
			t.Fatalf("delivery hold missing from owner queue: %+v", page)
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM maintenance_findings WHERE org_id=$1 AND source='release_delivery' AND version=1`, org).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatal("same hold duplicated or reopened")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	operation := domain.NewID()
	if err = service.deliveryOutcome(ctx, session, org, ids[0], operation, nil); err != nil {
		t.Fatal(err)
	}
	if err = identity.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		page, err := attention.ListTx(ctx, tx, a, 10, "")
		if err != nil {
			return err
		}
		if len(page.Items) != 0 {
			t.Fatalf("resolved hold retained: %+v", page)
		}
		var ref string
		if err = tx.QueryRow(ctx, `SELECT evidence->>'delivery_operation' FROM maintenance_findings WHERE org_id=$1 AND source='release_delivery' AND state='resolved'`, org).Scan(&ref); err != nil {
			return err
		}
		if ref != operation {
			t.Fatal("resolution lost operation attribution")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
