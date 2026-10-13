package discovery

import (
	"context"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/source"
)

func reportURL(value string) bool {
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && len(value) <= 2048 && u.Hostname() != "" && u.User == nil && (u.Scheme == "https" || u.Scheme == "http")
}

func (s *Service) ReportBug(ctx context.Context, session auth.Session, org string, in BugReportInput, request string) (Finding, error) {
	var out Finding
	in.Observed, in.Expected = strings.TrimSpace(in.Observed), strings.TrimSpace(in.Expected)
	if !auth.ValidID(in.RepositoryID) || in.Kind != "defect" || in.Observed == "" || in.Expected == "" || len(in.Observed) > 2000 || len(in.Expected) > 2000 || len(in.Error) > 4096 || !reportURL(in.URL) || !reportURL(in.ScreenshotURL) {
		return out, auth.ErrInvalid
	}
	var ref forge.RepoRef
	var connection, branch string
	var version int64
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, in.RepositoryID) {
			return auth.ErrForbidden
		}
		return hidden(tx.QueryRow(ctx, `SELECT r.native_id,r.name,r.default_branch,c.id::text,c.version FROM repositories r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2 AND r.accessible AND NOT r.archived AND c.state='healthy'`, org, in.RepositoryID).Scan(&ref.NativeID, &ref.FullName, &branch, &connection, &version))
	})
	if err != nil {
		return out, err
	}
	if s.reader == nil {
		return out, privateconnector.ErrUnsupported
	}
	authorize := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !manage(a, in.RepositoryID) || c.ID != connection || c.Version != version || c.State != "healthy" {
			return auth.ErrConflict
		}
		var current forge.RepoRef
		var currentBranch string
		if err := tx.QueryRow(ctx, `SELECT native_id,name,default_branch FROM repositories WHERE org_id=$1 AND id=$2 AND connection_id=$3 AND accessible AND NOT archived`, org, in.RepositoryID, connection).Scan(&current.NativeID, &current.FullName, &currentBranch); err != nil {
			return hidden(err)
		}
		if current != ref || currentBranch != branch {
			return ErrStale
		}
		return nil
	}
	resolved, err := s.reader.Read(ctx, org, connection, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: ref, Ref: branch}}, authorize)
	if err != nil {
		return out, err
	}
	if !source.ValidSHA(resolved.SHA, "sha1") {
		return out, ErrStale
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var c connections.Connection
		c.ID, c.Version, c.State = connection, version, "healthy"
		var actualVersion int64
		if err := tx.QueryRow(ctx, `SELECT version FROM connections WHERE org_id=$1 AND id=$2 AND state='healthy'`, org, connection).Scan(&actualVersion); err != nil {
			return hidden(err)
		}
		if actualVersion != version {
			return ErrStale
		}
		if err := authorize(ctx, tx, c); err != nil {
			return err
		}
		cfg, err := configTx(ctx, tx, org, in.RepositoryID)
		if err != nil {
			return err
		}
		title := []rune(in.Observed)
		if len(title) > 120 {
			title = append(title[:117], '.', '.', '.')
		}
		observation := Observation{RepositoryID: in.RepositoryID, Source: "user_report", SourceID: digest(in.BugReport), Category: "reported_defect", Severity: "medium", Title: string(title), Evidence: Evidence{Provenance: "Owner-reported defect; reproduction required", ConnectionID: connection, ConnectionVersion: version, ConfigVersion: cfg.Version, HeadSHA: resolved.SHA, TargetSHA: resolved.SHA, TargetBranch: branch, Ownership: "human", Report: &in.BugReport, Complete: true}}
		out, err = ObserveTx(ctx, tx, org, observation, a.UserID, request)
		return err
	})
	if err == nil && out.State == "open" && s.FixRequested != nil {
		if queuedErr := s.FixRequested(ctx, session, org, out, request); queuedErr != nil {
			out.Reason = "Reported; fix needs setup or a fresh scan"
		}
	}
	return out, err
}
