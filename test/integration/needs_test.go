package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/autopilot"
	"github.com/reforgeapp/reforge/pkg/domain"
)

func TestNeedsFiltersRepositoryScopeBeforeLimit(t *testing.T) {
	f := newInventoryFixture(t, 0)
	ctx := context.Background()
	hiddenRepo, visibleRepo, visibleFinding := domain.NewID(), domain.NewID(), domain.NewID()
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO repositories(org_id,id,native_id,name) VALUES($1,$2,'hidden','a-hidden'),($1,$3,'visible','z-visible')`, f.org, hiddenRepo, visibleRepo); err != nil {
			return err
		}
		for i := 0; i < 110; i++ {
			if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'repository','permission','provider_access','high','Grant access','{"blockers":["Needs a person: grant access"]}',$4)`, f.org, domain.NewID(), hiddenRepo, fmt.Sprintf("%064x", i+1)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest) VALUES($1,$2,$3,$4,'repository','permission','provider_access','high','Grant access','{"blockers":["Needs a person: grant access"]}',$4)`, f.org, visibleFinding, visibleRepo, fmt.Sprintf("%064x", 111)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=false WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, f.org, f.owner.User.ID, visibleRepo)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := autopilot.New(f.db, f.identity, nil, nil, nil, nil)
	needs, err := service.Needs(ctx, f.owner, f.org)
	if err != nil || len(needs) != 1 || needs[0].FindingID != visibleFinding || needs[0].RepositoryID != visibleRepo {
		t.Fatalf("visible decision hidden by inaccessible rows: %+v %v", needs, err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET all_repositories=true WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	needs, err = service.Needs(ctx, f.owner, f.org)
	if err != nil || len(needs) != 100 {
		t.Fatalf("organisation-wide decision limit changed: count=%d err=%v", len(needs), err)
	}
}
