package mergecontrol

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
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/providers"
	"github.com/reforgeapp/reforge/pkg/source"
	"github.com/reforgeapp/reforge/pkg/store"
)

type readinessProvider struct {
	providers.Client
	db           *store.Store
	org          string
	user         string
	connection   string
	rules        forge.Rules
	readErr      error
	changeConfig bool
}

func (p *readinessProvider) ForProtection(string, map[string]string) providers.Client { return p }

func (p *readinessProvider) Read(ctx context.Context, org, connection string, op privateconnector.Operation, check func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	if op.Kind != privateconnector.ForgeReadRules || connection != p.connection {
		return privateconnector.Result{}, privateconnector.ErrUnsupported
	}
	if p.readErr != nil {
		return privateconnector.Result{}, p.readErr
	}
	err := p.db.Tenant(ctx, p.org, p.user, func(tx pgx.Tx) error {
		if p.changeConfig {
			if _, err := tx.Exec(ctx, `UPDATE merge_configurations SET version=2,document=jsonb_set(document,'{version}','2'::jsonb) WHERE org_id=$1`, p.org); err != nil {
				return err
			}
		}
		return check(ctx, tx, connections.Connection{ID: connection, Version: 1, State: "healthy"})
	})
	if err != nil {
		return privateconnector.Result{}, err
	}
	rules := p.rules
	return privateconnector.Result{Rules: &rules}, nil
}

func (*readinessProvider) SourceReader(string, string, func(context.Context, pgx.Tx, connections.Connection) error) source.Reader {
	return source.Reader{}
}

func (*readinessProvider) Write(context.Context, string, string, privateconnector.Operation, func(context.Context, pgx.Tx, connections.Connection) (string, error), func(context.Context, pgx.Tx, connections.Connection) error) (privateconnector.Result, error) {
	return privateconnector.Result{}, privateconnector.ErrUnsupported
}

func TestMaintenanceReadinessUsesCurrentNativeRulesAndQualification(t *testing.T) {
	raw := os.Getenv("REFORGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires disposable PostgreSQL reforge_test")
	}
	if u, err := url.Parse(raw); err != nil || u.Path != "/reforge_test" {
		t.Fatal("requires disposable reforge_test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	identity, err := auth.New(ctx, db, auth.Config{PublicURL: "http://127.0.0.1:8080", Edition: "self-hosted", Development: true, ListenAddress: "127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	org, user, repo, connection, inspector := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	session := auth.Session{ID: domain.NewID(), User: auth.User{ID: user}, CSRFToken: domain.NewID()}
	if err := db.Identity(ctx, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,issuer,subject,name,email) VALUES($1,'merge-readiness-test',$2,'Owner',$3)`, user, user, user+"@example.test"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour')`, session.ID, user, domain.NewID(), session.CSRFToken)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'Merge readiness')`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES($1,$2,'owner',true)`, org, user); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,server_version) VALUES($1,$2,'forge','github','GitHub','https://api.github.com','{}','healthy','v1')`, org, connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,server_version) VALUES($1,$2,'forge','github','Inspector','https://api.github.com','{}','healthy','v1')`, org, inspector); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,connection_id,native_id,name) VALUES($1,$2,$3,'123','team/repo')`, org, repo, connection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_configs(org_id,repository_id,merge_authority) VALUES($1,$2,'reforge')`, org, repo); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO merge_configurations(org_id,repository_id,version,document) VALUES($1,$2,1,'{}')`, org, repo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	connService := connections.New(db, identity, nil, true)
	provider := &readinessProvider{db: db, org: org, user: user, connection: connection, rules: forge.Rules{State: domain.Supported, Hash: strings.Repeat("a", 64), AllowedMergeMethods: []string{"squash"}, RequireStrictTarget: true}}
	service := New(db, identity, connService, nil, provider)
	base := Configuration{RepositoryID: repo, Version: 1, Enabled: true, InspectorConnectionID: inspector, CooperationReference: "qualified-run", Qualification: Qualification{Provider: "github", ServerVersion: "v1", ConnectionVersion: 1, InspectorVersion: 1, ExactHead: true, StrictTarget: true, EvidenceReference: "operator-check", EvidenceSHA256: strings.Repeat("b", 64), VerifiedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}}
	for _, tc := range []struct {
		name       string
		config     Configuration
		rules      forge.Rules
		wantReady  bool
		wantReason string
	}{
		{"qualified protection", base, provider.rules, true, ""},
		{"expired qualification", func() Configuration { c := base; c.Qualification.ExpiresAt = time.Now().Add(-time.Minute); return c }(), provider.rules, false, "Protected merge checks need operator setup"},
		{"server version mismatch", func() Configuration { c := base; c.Qualification.ServerVersion = "old"; return c }(), provider.rules, false, "Protected merge checks need operator setup"},
		{"inspector version mismatch", func() Configuration { c := base; c.Qualification.InspectorVersion = 2; return c }(), provider.rules, false, "Protected merge checks need operator setup"},
		{"bypass does not revive expired enabled configuration", func() Configuration { c := base; c.Qualification.ExpiresAt = time.Now().Add(-time.Minute); return c }(), func() forge.Rules { r := provider.rules; r.Unprotected = true; r.ActorCanBypass = true; return r }(), false, "Protected merge checks need operator setup"},
		{"approval required", base, func() forge.Rules { r := provider.rules; r.RequiredApprovals = 1; return r }(), false, "requires a person's approval"},
		{"rules unavailable", base, func() forge.Rules { r := provider.rules; r.State = domain.Unknown; return r }(), false, "need attention"},
		{"no merge method", base, func() forge.Rules { r := provider.rules; r.AllowedMergeMethods = nil; return r }(), false, "Enable a merge method"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.config)
			if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE merge_configurations SET version=1,document=$3 WHERE org_id=$1 AND repository_id=$2`, org, repo, body)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			provider.rules = tc.rules
			provider.changeConfig = false
			provider.readErr = nil
			got, err := service.MaintenanceReadiness(ctx, session, org, repo)
			if err != nil || got.Ready != tc.wantReady || tc.wantReason != "" && !strings.Contains(got.Reason, tc.wantReason) || got.RulesHash != tc.rules.Hash {
				t.Fatalf("readiness=%+v err=%v", got, err)
			}
		})
	}
	t.Run("configuration changed during native read", func(t *testing.T) {
		body, _ := json.Marshal(base)
		if err := db.Tenant(ctx, org, user, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE merge_configurations SET version=1,document=$3 WHERE org_id=$1 AND repository_id=$2`, org, repo, body)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		provider.changeConfig = true
		got, err := service.MaintenanceReadiness(ctx, session, org, repo)
		if err != nil || got.Ready || !strings.Contains(got.Reason, "Could not verify") {
			t.Fatalf("stale readiness=%+v err=%v", got, err)
		}
	})
	provider.changeConfig = false
	provider.readErr = errors.New("private connector unavailable")
	got, err := service.MaintenanceReadiness(ctx, session, org, repo)
	if err != nil || got.Ready || !strings.Contains(got.Reason, "Could not verify") {
		t.Fatalf("failed native read readiness=%+v err=%v", got, err)
	}
}
