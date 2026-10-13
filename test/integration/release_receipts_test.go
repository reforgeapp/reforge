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
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/release"
)

func TestSignedReleaseReceiptsFenceReplayWorkflowTargetAndRotation(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	public, private, _ := ed25519.GenerateKey(nil)
	workflow := strings.Repeat("c", 64)
	cfg := deployment.Configuration{RepositoryID: f.repo, Environment: "staging", Version: 1, Enabled: true, ProvenancePublicKey: base64.StdEncoding.EncodeToString(public), BuildWorkflowSHA256: workflow}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		raw, _ := json.Marshal(cfg)
		_, err := tx.Exec(ctx, `INSERT INTO deployment_configurations(org_id,environment,repository_id,version,document) VALUES($1,'staging',$2,1,$3)`, f.org, f.repo, raw)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := &release.Service{DB: f.db, Auth: f.identity}
	p := deployment.Provenance{OrgID: f.org, RepositoryID: f.repo, Environment: "staging", Kind: "native", ConfigurationVersion: 1, SourceSHA: strings.Repeat("a", 40), ArtifactDigest: "sha256:" + strings.Repeat("b", 64), WorkflowSHA256: workflow, BuildID: "ci-1", IssuedAt: time.Now().Add(-time.Second), ExpiresAt: time.Now().Add(time.Hour), Validation: &deployment.BuildValidation{TargetSHA: strings.Repeat("d", 40), Builds: 1, Complete: true, Cases: map[string]string{"mobile checkout": "pass"}, Outputs: []string{strings.Repeat("e", 64)}}}
	sign := func(p deployment.Provenance) deployment.SignedProvenance {
		raw, _ := json.Marshal(p)
		return deployment.SignedProvenance{Document: p, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))}
	}
	r, err := service.Receive(ctx, f.org, f.repo, sign(p))
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Receive(ctx, f.org, f.repo, sign(p))
	if err != nil || replay.ID != r.ID {
		t.Fatalf("receipt replay changed: %+v %v", replay, err)
	}
	changed := p
	changed.ArtifactDigest = "sha256:" + strings.Repeat("f", 64)
	if _, err = service.Receive(ctx, f.org, f.repo, sign(changed)); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("receipt identity overwritten: %v", err)
	}
	wrong := p
	wrong.BuildID = "ci-wrong"
	wrong.WorkflowSHA256 = strings.Repeat("f", 64)
	if _, err = service.Receive(ctx, f.org, f.repo, sign(wrong)); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("unapproved workflow accepted: %v", err)
	}
	if _, err = service.Receive(ctx, f.org, domain.NewID(), sign(p)); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("other repository accepted: %v", err)
	}
	proof := func(target string) string {
		t.Helper()
		var ref string
		if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
			var err error
			ref, err = release.BehaviourReferenceTx(ctx, tx, f.org, f.repo, p.SourceSHA, target, time.Now())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	if proof(p.Validation.TargetSHA) == "" || proof(strings.Repeat("f", 40)) != "" {
		t.Fatal("native proof lost exact target binding")
	}
	for _, state := range []string{"skip", "fail"} {
		bad := p
		copied := *p.Validation
		copied.Cases = map[string]string{"checkout": state}
		bad.Validation = &copied
		if release.Meaningful(bad) {
			t.Fatalf("%s counted as behavioural proof", state)
		}
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		cfg.Version = 2
		raw, _ := json.Marshal(cfg)
		_, err := tx.Exec(ctx, `UPDATE deployment_configurations SET version=2,document=$3 WHERE org_id=$1 AND repository_id=$2`, f.org, f.repo, raw)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if proof(p.Validation.TargetSHA) != "" {
		t.Fatal("old configuration receipt remained authoritative")
	}
	if _, err = service.Receive(ctx, f.org, f.repo, sign(p)); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("old config replay accepted: %v", err)
	}
	page, err := service.List(ctx, f.owner, f.org, f.repo, "", 1)
	if err != nil || len(page.Items) != 1 || !page.Complete {
		t.Fatalf("scoped release list: %+v %v", page, err)
	}
}
