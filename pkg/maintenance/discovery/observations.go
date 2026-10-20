package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/detectors"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
	"github.com/reforgeapp/reforge/pkg/source"
)

func validObservation(in Observation) bool {
	if !auth.ValidID(in.RepositoryID) || !auth.ValidID(in.Evidence.ConnectionID) || len(in.SourceID) < 1 || len(in.SourceID) > 256 || len(in.Title) < 1 || len(in.Title) > 512 || len(in.Evidence.Blockers) > 100 || len(in.Evidence.Dependencies) > 10000 {
		return false
	}
	switch in.Severity {
	case "info", "low", "medium", "high", "critical":
	default:
		return false
	}
	if !categoryPattern.MatchString(in.Category) {
		return false
	}
	switch in.Source {
	case "forge_change", "native_ci", "imported_advisory", "repository", "repository_review", "forge_issue", "forge_advisory":
	default:
		return false
	}
	return source.ValidSHA(in.Evidence.HeadSHA, "sha1") && source.ValidSHA(in.Evidence.TargetSHA, "sha1") && in.Evidence.TargetBranch != "" && len(in.Evidence.TargetBranch) <= 1024
}

var categoryPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,47}$`)

func ObserveTx(ctx context.Context, tx pgx.Tx, org string, in Observation, actor, request string) (Finding, error) {
	var f Finding
	if !validObservation(in) {
		return f, auth.ErrInvalid
	}
	if err := lockOrg(ctx, tx, org); err != nil {
		return f, err
	}
	in = canonical(in)
	body, err := json.Marshal(in.Evidence)
	if err != nil || len(body) > 512<<10 {
		return f, auth.ErrInvalid
	}
	fingerprint := Fingerprint(org, in)
	digestEvidence := in.Evidence
	if in.Evidence.Change != nil {
		change := *in.Evidence.Change
		change.Title = ""
		change.Body = ""
		change.URL = ""
		digestEvidence.Change = &change
	}
	evidenceDigest := digest(digestEvidence)
	f, err = scanFinding(tx.QueryRow(ctx, `SELECT `+columns+` FROM maintenance_findings WHERE org_id=$1 AND repository_id=$2 AND fingerprint=$3`, org, in.RepositoryID, fingerprint))
	created := errors.Is(err, auth.ErrForbidden)
	if err != nil && !created {
		return f, err
	}
	changed := created || f.EvidenceDigest != evidenceDigest
	reopened := !created && (f.State == "superseded" || changed && (f.State == "dismissed" || f.State == "snoozed" || f.State == "resolved" || f.State == "superseded") || f.State == "snoozed" && f.SnoozeUntil != nil && !f.SnoozeUntil.After(time.Now()))
	if created {
		f = Finding{ID: domain.NewID(), OrgID: org, Observation: in, Fingerprint: fingerprint, EvidenceDigest: evidenceDigest, State: "open", Version: 1}
	} else {
		f.Observation = in
		f.EvidenceDigest = evidenceDigest
		if changed || reopened {
			f.Version++
		}
		if reopened {
			f.State = "open"
			f.SnoozeUntil = nil
			f.SupersededBy = ""
			f.Reason = "Evidence changed or snooze expired"
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO maintenance_findings(org_id,id,repository_id,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest,state,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(org_id,repository_id,fingerprint) DO UPDATE SET title=excluded.title,severity=excluded.severity,evidence=excluded.evidence,evidence_digest=excluded.evidence_digest,state=excluded.state,version=excluded.version,last_seen=clock_timestamp(),snooze_until=$14,reason=$15,superseded_by=nullif($16,'')::uuid`, org, f.ID, in.RepositoryID, fingerprint, in.Source, in.SourceID, in.Category, in.Severity, in.Title, body, evidenceDigest, f.State, f.Version, f.SnoozeUntil, f.Reason, f.SupersededBy)
	if err != nil {
		return f, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO maintenance_observations(org_id,id,finding_id,evidence_digest,evidence) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,finding_id,evidence_digest) DO NOTHING`, org, domain.NewID(), f.ID, evidenceDigest, body); err != nil {
		return f, err
	}
	if (created || reopened) && in.Source == "forge_change" {
		rows, err := tx.Query(ctx, `UPDATE maintenance_findings SET state='superseded',superseded_by=$6,version=version+1,reason='Native change now requests a different dependency group or target' WHERE org_id=$1 AND repository_id=$2 AND source=$3 AND source_id=$4 AND category=$5 AND id<>$6 AND state<>'superseded' RETURNING `+columns, org, in.RepositoryID, in.Source, in.SourceID, in.Category, f.ID)
		if err != nil {
			return f, err
		}
		var old []Finding
		for rows.Next() {
			v, e := scanFinding(rows)
			if e != nil {
				rows.Close()
				return f, e
			}
			old = append(old, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return f, err
		}
		for _, v := range old {
			if err = record(ctx, tx, v, actor, "superseded", request); err != nil {
				return f, err
			}
		}
	}
	if changed || reopened {
		action := "observed"
		if reopened {
			action = "reopened"
		}
		if err = record(ctx, tx, f, actor, action, request); err != nil {
			return f, err
		}
	}
	return load(ctx, tx, org, f.ID)
}
func (s *Service) ImportAdvisory(ctx context.Context, session auth.Session, org string, in AdvisoryInput, request string) (Finding, error) {
	var out Finding
	u, err := url.Parse(in.ReferenceURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(in.ReferenceURL) > 2048 || !auth.ValidID(in.RepositoryID) || !guest.ValidPath(in.Path) || !source.ValidSHA(in.CommitSHA, "sha1") || len(in.Package) < 1 || len(in.Package) > 256 || len(in.Ecosystem) < 1 || len(in.Ecosystem) > 32 || len(in.AffectedRange) > 512 {
		return out, auth.ErrInvalid
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, in.RepositoryID) {
			return auth.ErrForbidden
		}
		var connection, branch string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT r.connection_id::text,r.default_branch,c.version FROM repositories r JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2 AND r.accessible AND NOT r.archived`, org, in.RepositoryID).Scan(&connection, &branch, &version); err != nil {
			return hidden(err)
		}
		cfg, err := configTx(ctx, tx, org, in.RepositoryID)
		if err != nil {
			return err
		}
		observation := Observation{RepositoryID: in.RepositoryID, Source: "imported_advisory", SourceID: in.AdvisoryID, Category: "security_advisory", Severity: in.Severity, Title: strings.TrimSpace(in.Title), Evidence: Evidence{ConnectionID: connection, ConnectionVersion: version, ConfigVersion: cfg.Version, HeadSHA: in.CommitSHA, TargetSHA: in.CommitSHA, TargetBranch: branch, Dependencies: []detectors.Dependency{{Ecosystem: in.Ecosystem, Manifest: in.Path, Name: in.Package, From: in.AffectedRange}}, Ownership: "human", Provenance: "reported; requires independent scanner reproduction", AdvisoryID: in.AdvisoryID, ReferenceURL: in.ReferenceURL, Complete: true}}
		out, err = ObserveTx(ctx, tx, org, observation, a.UserID, request)
		return err
	})
	return out, err
}
