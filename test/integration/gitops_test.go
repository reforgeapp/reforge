package integration

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/gitops"
	"github.com/reforgeapp/reforge/internal/policy"
)

func gitopsConfiguration(org, source, delivery string, provenance, health ed25519.PublicKey) gitops.Configuration {
	return gitops.Configuration{
		Environment: "production", SourceRepositoryID: source, DeliveryRepositoryID: delivery, Enabled: true,
		TargetBranch: "main", ManifestPath: "deploy.yaml", Pointer: "/image", ImageRepository: "registry.example/app",
		ProvenancePublicKey: base64.StdEncoding.EncodeToString(provenance), HealthPublicKey: base64.StdEncoding.EncodeToString(health),
		HealthChecks: []string{"smoke"}, ObservationSeconds: 1, MaxEvidenceAgeSeconds: 300, DeadlineSeconds: 600,
	}
}

func TestGitOpsConfigurationCASDualRepositoryScopeAndRLS(t *testing.T) {
	f := newInventoryFixture(t, 2)
	f.importAll(t)
	ctx := context.Background()
	var repositories []string
	if err := f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text FROM repositories WHERE org_id=$1 ORDER BY name`, f.org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return err
			}
			repositories = append(repositories, id)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 2 {
		t.Fatalf("fixture repositories: %d", len(repositories))
	}
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	service := gitops.New(f.db, f.identity, f.connections, policies, nil, nil)
	provenance, _, _ := ed25519.GenerateKey(nil)
	health, _, _ := ed25519.GenerateKey(nil)
	config := gitopsConfiguration(f.org, repositories[0], repositories[1], provenance, health)
	stored, err := service.PutConfiguration(ctx, f.owner, f.org, config, 0, "gitops-config")
	if err != nil || stored.Version != 1 || stored.SourceRepositoryID != repositories[0] || stored.DeliveryRepositoryID != repositories[1] {
		t.Fatalf("dual repository configuration: %+v %v", stored, err)
	}
	if _, err = service.PutConfiguration(ctx, f.owner, f.org, config, 0, "stale-config"); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale configuration accepted: %v", err)
	}
	for _, repositoryID := range []string{repositories[0], repositories[1]} {
		userID, cookie := fixtureIdentity(t, f.db)
		if _, err = f.identity.PutMember(ctx, f.owner, f.org, userID, auth.Membership{Role: domain.Maintainer, RepositoryIDs: []string{repositoryID}}, 0, "gitops-scope"); err != nil {
			t.Fatal(err)
		}
		scoped, authenticateErr := f.identity.Authenticate(ctx, cookie.Value)
		if authenticateErr != nil {
			t.Fatal(authenticateErr)
		}
		configs, listErr := service.Configurations(ctx, scoped, f.org)
		if listErr != nil {
			t.Fatal(listErr)
		}
		if len(configs) != 0 {
			t.Fatalf("single-repository member saw dual-scope configuration: repository=%s", repositoryID)
		}
	}
	otherOrg := domain.NewID()
	var visible int
	if err = f.db.Tenant(ctx, otherOrg, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM gitops_configurations WHERE org_id=$1`, f.org).Scan(&visible)
	}); err != nil {
		t.Fatal(err)
	}
	if visible != 0 {
		t.Fatalf("cross-tenant configuration visible: %d", visible)
	}
}

func TestGitOpsHealthExactRevisionNonceReplayAndRestart(t *testing.T) {
	f := newInventoryFixture(t, 1)
	f.importAll(t)
	ctx := context.Background()
	policies, err := policy.New(f.db, f.identity, policy.Policy{Schema: "maintenance/v1"})
	if err != nil {
		t.Fatal(err)
	}
	service := gitops.New(f.db, f.identity, f.connections, policies, nil, nil)
	provenance, _, _ := ed25519.GenerateKey(nil)
	healthPublic, healthPrivate, _ := ed25519.GenerateKey(nil)
	repositoryID := f.repository(t).ID
	config := gitopsConfiguration(f.org, repositoryID, repositoryID, provenance, healthPublic)
	config, err = service.PutConfiguration(ctx, f.owner, f.org, config, 0, "gitops-health-config")
	if err != nil {
		t.Fatal(err)
	}
	promotionID, gateID := domain.NewID(), domain.NewID()
	sourceSHA := strings.Repeat("a", 40)
	deliveryRevision := strings.Repeat("b", 40)
	artifact := "sha256:" + strings.Repeat("c", 64)
	gate := gitops.Gate{ID: gateID, OperationID: promotionID, Configuration: config, Request: gitops.PreviewRequest{ChangeID: "1", SourceSHA: sourceSHA, ArtifactDigest: artifact}, TargetSHA: strings.Repeat("d", 40)}
	promotion := gitops.Promotion{ID: promotionID, Environment: config.Environment, SourceRepositoryID: config.SourceRepositoryID, DeliveryRepositoryID: config.DeliveryRepositoryID, GateID: gateID, State: "completed_unverified", RequestedBy: f.owner.User.ID, Branch: "reforge/promote/" + promotionID, CandidateSHA: deliveryRevision, MergeSHA: deliveryRevision, Version: 1}
	gateRaw, _ := json.Marshal(gate)
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO gitops_gates(org_id,id,environment,source_repository_id,delivery_repository_id,document) VALUES($1,$2,$3,$4,$5,$6)`, f.org, gateID, config.Environment, config.SourceRepositoryID, config.DeliveryRepositoryID, gateRaw); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO gitops_promotions(org_id,id,environment,source_repository_id,delivery_repository_id,gate_id,requested_by,idempotency_key,state,branch,target_branch,manifest_path,pointer,candidate_sha,merge_sha) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, f.org, promotion.ID, promotion.Environment, promotion.SourceRepositoryID, promotion.DeliveryRepositoryID, promotion.GateID, promotion.RequestedBy, "health-contract", promotion.State, promotion.Branch, config.TargetBranch, config.ManifestPath, config.Pointer, promotion.CandidateSHA, promotion.MergeSHA)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	report := gitops.HealthReport{OrgID: f.org, PromotionID: promotionID, Environment: config.Environment, ConfigurationVersion: config.Version, SourceSHA: sourceSHA, ArtifactDigest: artifact, DeliveryRevision: deliveryRevision, Healthy: true, Checks: map[string]bool{"smoke": true}, ObservedAt: time.Now(), Nonce: domain.NewID()}
	sign := func(in gitops.HealthReport) string {
		raw, _ := json.Marshal(in)
		return base64.StdEncoding.EncodeToString(ed25519.Sign(healthPrivate, raw))
	}
	if err = service.ReceiveHealth(ctx, f.org, promotionID, report, base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("forged health accepted: %v", err)
	}
	wrong := report
	wrong.DeliveryRevision = strings.Repeat("e", 40)
	if err = service.ReceiveHealth(ctx, f.org, promotionID, wrong, sign(wrong)); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("wrong delivery revision accepted: %v", err)
	}
	wrong = report
	wrong.SourceSHA = strings.Repeat("e", 40)
	if err = service.ReceiveHealth(ctx, f.org, promotionID, wrong, sign(wrong)); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("wrong source revision accepted: %v", err)
	}
	wrong = report
	wrong.ArtifactDigest = "sha256:" + strings.Repeat("e", 64)
	if err = service.ReceiveHealth(ctx, f.org, promotionID, wrong, sign(wrong)); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("wrong artifact digest accepted: %v", err)
	}
	wrong = report
	wrong.ObservedAt = time.Now().Add(-time.Hour)
	if err = service.ReceiveHealth(ctx, f.org, promotionID, wrong, sign(wrong)); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("stale health accepted: %v", err)
	}
	if err = service.ReceiveHealth(ctx, f.org, promotionID, report, sign(report)); err != nil {
		t.Fatal(err)
	}
	if err = service.ReceiveHealth(ctx, f.org, promotionID, report, sign(report)); err != nil {
		t.Fatalf("nonce replay not idempotent: %v", err)
	}
	restarted := gitops.New(f.db, f.identity, f.connections, policies, nil, nil)
	if err = restarted.ReceiveHealth(ctx, f.org, promotionID, report, sign(report)); err != nil {
		t.Fatalf("restart nonce replay: %v", err)
	}
	negative := report
	negative.Nonce = domain.NewID()
	negative.ObservedAt = time.Now().Add(time.Second)
	negative.Healthy = false
	negative.Checks = map[string]bool{"smoke": false}
	if err = restarted.ReceiveHealth(ctx, f.org, promotionID, negative, sign(negative)); err != nil {
		t.Fatalf("negative health report: %v", err)
	}
	otherOrg := domain.NewID()
	if err = f.db.Tenant(ctx, otherOrg, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'GitOps other org')`, otherOrg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, otherOrg, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Get(ctx, f.owner, otherOrg, promotionID); !errors.Is(err, auth.ErrForbidden) && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant promotion read: %v", err)
	}
	if err = f.db.Tenant(ctx, f.org, "", func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT document FROM gitops_health_reports WHERE org_id=$1 AND promotion_id=$2 ORDER BY received_at DESC LIMIT 1`, f.org, promotionID).Scan(&raw); err != nil {
			return err
		}
		var latest gitops.HealthReport
		if err := json.Unmarshal(raw, &latest); err != nil {
			return err
		}
		if latest.Healthy || latest.Checks["smoke"] {
			t.Fatalf("negative health report was not durable: %+v", latest)
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM gitops_health_reports WHERE org_id=$1 AND promotion_id=$2`, f.org, promotionID).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("health report duplicates: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
