package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
)

func TestBugReportPinsRevisionRetainsIssueAndFencesAuthority(t *testing.T) {
	f := newDiscoveryFixture(t)
	service, reader := f.scannerFixture(t, false)
	ctx := context.Background()
	requested := 0
	service.FixRequested = func(_ context.Context, _ auth.Session, org string, finding discovery.Finding, _ string) error {
		if org != f.org || finding.Source != "user_report" || finding.RepositoryID != f.repo {
			t.Fatalf("request widened: %+v", finding)
		}
		requested++
		return nil
	}
	in := discovery.BugReportInput{RepositoryID: f.repo, Kind: "defect", BugReport: discovery.BugReport{Observed: "Checkout button does nothing on mobile", Expected: "Checkout opens payment", URL: "https://shop.example/checkout", Error: "click ignored"}}
	finding, err := service.ReportBug(ctx, f.owner, f.org, in, "reported-defect")
	if err != nil || finding.Evidence.HeadSHA != strings.Repeat("a", 40) || finding.Evidence.Report == nil || finding.Evidence.Report.Expected != in.Expected || requested != 1 {
		t.Fatalf("report: %+v %v", finding, err)
	}
	again, err := service.ReportBug(ctx, f.owner, f.org, in, "repeat-report")
	if err != nil || again.ID != finding.ID {
		t.Fatalf("duplicate report not reused: %+v %v", again, err)
	}
	if _, err := service.StartScan(ctx, f.owner, f.org, f.repo, "report-scan"); err != nil {
		t.Fatal(err)
	}
	if worked, err := service.RunOrganisationOnce(ctx, f.org); err != nil || !worked {
		t.Fatalf("scan: %v %v", worked, err)
	}
	again, err = service.Get(ctx, f.owner, f.org, finding.ID)
	if err != nil || again.State != "open" || again.Evidence.Report == nil {
		t.Fatalf("scan erased user report: %+v %v", again, err)
	}
	in.Kind = "feature"
	if _, err := service.ReportBug(ctx, f.owner, f.org, in, "feature-request"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("feature request admitted: %v", err)
	}
	in.Kind = "defect"
	in.Observed = "Different report"
	reader.stale = true
	if _, err := service.ReportBug(ctx, f.owner, f.org, in, "connection-moved"); !errors.Is(err, discovery.ErrStale) {
		t.Fatalf("moved authority admitted: %v", err)
	}
	if err := f.db.Tenant(ctx, f.org, f.owner.User.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE memberships SET role='viewer' WHERE org_id=$1 AND user_id=$2`, f.org, f.owner.User.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReportBug(ctx, f.owner, f.org, in, "read-only"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("read-only report mutation accepted: %v", err)
	}
}
