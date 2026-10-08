package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"reforge/internal/heartbeat"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/inventory"
	"reforge/internal/maintenance/detectors"
	"reforge/internal/privateconnector"
	"reforge/internal/sandbox/guest"
	"reforge/internal/source"
)

func (s *Service) ScanStatus(ctx context.Context, session auth.Session, org, repo string) (Scan, error) {
	out := Scan{RepositoryID: repo, State: "not_started"}
	if !auth.ValidID(repo) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		err := tx.QueryRow(ctx, `SELECT state,reason,version,observed_at FROM maintenance_scans WHERE org_id=$1 AND repository_id=$2`, org, repo).Scan(&out.State, &out.Reason, &out.Version, &out.ObservedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			var id string
			return hidden(tx.QueryRow(ctx, `SELECT id::text FROM repositories WHERE org_id=$1 AND id=$2`, org, repo).Scan(&id))
		}
		return err
	})
	return out, err
}
func (s *Service) StartScan(ctx context.Context, session auth.Session, org, repo, request string) (Scan, error) {
	out := Scan{RepositoryID: repo}
	if !auth.ValidID(repo) {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a, repo) {
			return auth.ErrForbidden
		}
		if err := inventory.RequireFreshTx(ctx, tx, org, repo); err != nil {
			if !errors.Is(err, inventory.ErrStale) {
				return err
			}
			if err = inventory.RequestRefreshTx(ctx, tx, org, repo); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_scans(org_id,repository_id,requested_by) VALUES($1,$2,$3) ON CONFLICT(org_id,repository_id) DO UPDATE SET state=CASE WHEN maintenance_scans.state='running' AND maintenance_scans.lease_until>clock_timestamp() THEN maintenance_scans.state ELSE 'queued' END,available_at=clock_timestamp(),requested_by=excluded.requested_by,reason='',version=maintenance_scans.version+1`, org, repo, a.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,repository_id,actor_id,action,object_id,request_id) VALUES($1,$2,$3::uuid,$4,'maintenance.scan',$3::text,$5)`, domain.NewID(), org, repo, a.UserID, request)
		return err
	})
	if err != nil {
		return out, err
	}
	return s.ScanStatus(ctx, session, org, repo)
}

type scanLease struct {
	Org, Repo, User, Connection, Branch string
	Ref                                 forge.RepoRef
	Version, Fence, ConfigVersion       int64
}

func (s *Service) claim(ctx context.Context, org string) (*scanLease, error) {
	var out *scanLease
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_scans(org_id,repository_id) SELECT r.org_id,r.id FROM repositories r JOIN inventory_repository_state i ON i.org_id=r.org_id AND i.repository_id=r.id WHERE r.org_id=$1 AND r.accessible AND NOT r.archived AND i.state='fresh' AND i.changes_observed_at>clock_timestamp()-interval '15 minutes' AND NOT EXISTS(SELECT 1 FROM maintenance_scans existing WHERE existing.org_id=r.org_id AND existing.repository_id=r.id) ORDER BY r.id LIMIT 20 ON CONFLICT DO NOTHING`, org); err != nil {
			return err
		}
		var lease scanLease
		lease.Org = org
		err := tx.QueryRow(ctx, `SELECT s.repository_id::text,coalesce(s.requested_by::text,''),r.connection_id::text,r.default_branch,r.native_id,r.name,c.version FROM maintenance_scans s JOIN repositories r ON r.org_id=s.org_id AND r.id=s.repository_id JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE s.org_id=$1 AND (s.state IN ('queued','complete','stale') AND s.available_at<=clock_timestamp() OR s.state='running' AND s.lease_until<=clock_timestamp()) AND r.accessible AND NOT r.archived AND c.state='healthy' ORDER BY s.available_at,s.repository_id LIMIT 1 FOR UPDATE OF s`, org).Scan(&lease.Repo, &lease.User, &lease.Connection, &lease.Branch, &lease.Ref.NativeID, &lease.Ref.FullName, &lease.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		err = inventory.RequireFreshTx(ctx, tx, org, lease.Repo)
		if err == nil {
			var recent bool
			if err = tx.QueryRow(ctx, `SELECT changes_observed_at>clock_timestamp()-interval '5 minutes' FROM inventory_repository_state WHERE org_id=$1 AND repository_id=$2`, org, lease.Repo).Scan(&recent); err == nil && !recent {
				err = inventory.ErrStale
			}
		}
		if err != nil {
			if !errors.Is(err, inventory.ErrStale) {
				return err
			}
			var refreshActive bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running'))`, org, lease.Repo).Scan(&refreshActive); err != nil {
				return err
			}
			if !refreshActive {
				if err = inventory.RequestRefreshTx(ctx, tx, org, lease.Repo); err != nil {
					return err
				}
			}
			_, err = tx.Exec(ctx, `UPDATE maintenance_scans SET state='queued',available_at=GREATEST(clock_timestamp()+interval '5 seconds',COALESCE((SELECT min(CASE WHEN state='running' THEN lease_until ELSE available_at END) FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running')),clock_timestamp())),reason=CASE WHEN EXISTS(SELECT 1 FROM inventory_jobs WHERE org_id=$1 AND repository_id=$2 AND kind='refresh' AND state IN ('queued','running') AND reason='Provider rate limit reached; retry scheduled') THEN 'Waiting for provider rate limit reset before refreshing native inventory' ELSE 'Waiting for a fresh native inventory observation' END,version=version+1 WHERE org_id=$1 AND repository_id=$2`, org, lease.Repo)
			return err
		}
		cfg, err := configTx(ctx, tx, org, lease.Repo)
		if err != nil {
			return err
		}
		lease.ConfigVersion = cfg.Version
		if err = tx.QueryRow(ctx, `UPDATE maintenance_scans SET state='running',fence=fence+1,lease_until=clock_timestamp()+interval '10 minutes',version=version+1 WHERE org_id=$1 AND repository_id=$2 RETURNING fence`, org, lease.Repo).Scan(&lease.Fence); err != nil {
			return err
		}
		out = &lease
		return nil
	})
	return out, err
}
func validateScan(ctx context.Context, tx pgx.Tx, lease scanLease, c connections.Connection) error {
	if c.ID != lease.Connection || c.Version != lease.Version || c.OrgID != lease.Org || c.State != "healthy" {
		return ErrStale
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM maintenance_scans WHERE org_id=$1 AND repository_id=$2 AND state='running' AND fence=$3 AND lease_until>clock_timestamp())`, lease.Org, lease.Repo, lease.Fence).Scan(&active); err != nil {
		return err
	}
	if !active {
		return ErrStale
	}
	if err := inventory.RequireFreshTx(ctx, tx, lease.Org, lease.Repo); err != nil {
		return err
	}
	cfg, err := configTx(ctx, tx, lease.Org, lease.Repo)
	if err != nil {
		return err
	}
	if cfg.Version != lease.ConfigVersion {
		return ErrStale
	}
	if lease.User != "" {
		return memberScope(ctx, tx, lease.Org, lease.User, lease.Repo, true)
	}
	return nil
}
func validChecks(checks []forge.Check, sha string) bool {
	for _, check := range checks {
		if check.HeadSHA != sha || check.ID == "" {
			return false
		}
	}
	return true
}
func failedChecks(checks []forge.Check) bool {
	for _, c := range checks {
		if BotUpdateJob(c.Name) {
			continue
		}
		switch c.Conclusion {
		case "failure", "failed", "timed_out", "action_required", "startup_failure", "missing":
			return true
		}
	}
	return false
}

func RepairConflict(change forge.Change) bool {
	if change.State != "open" && change.State != "opened" || change.Draft || !strings.HasPrefix(change.HeadBranch, "reforge/repair/") || change.Repository.NativeID == "" || change.HeadRepository != change.Repository || change.TargetRepository != change.Repository {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(change.MergeStatus)) {
	case "dirty", "conflict":
		return true
	default:
		return false
	}
}

func repairConflictCheck(change forge.Change) forge.Check {
	return forge.Check{ID: "reforge-conflict:" + change.ID, Name: "Reforge branch conflict", HeadSHA: change.HeadSHA, Status: "completed", Conclusion: "failure"}
}

const (
	WaitingForTarget = "Waiting for the default branch to pass CI"
	WaitingForRebase = "Waiting for the bot to rebase"
)

func failingChecks(checks []forge.Check) map[string]bool {
	out := map[string]bool{}
	for _, c := range checks {
		if !BotUpdateJob(c.Name) && failedChecks([]forge.Check{c}) {
			out[c.Name] = true
		}
	}
	return out
}

func waitsForTarget(target, change []forge.Check) bool {
	broken, failing := failingChecks(target), failingChecks(change)
	if len(broken) == 0 || len(failing) == 0 {
		return false
	}
	for name := range failing {
		if !broken[name] {
			return false
		}
	}
	return true
}

func botConfigGap(files map[string][]byte, cfg detectors.BotConfig) string {
	if !cfg.Renovate.Present && !cfg.Dependabot.Present {
		return "Add automated dependency updates"
	}
	for name, body := range files {
		base := path.Base(name)
		if (base == "dependabot.yml" || base == "dependabot.yaml") && !strings.Contains(string(body), "groups:") {
			return "Group Dependabot updates to cut pull request noise"
		}
	}
	return ""
}

func providerObservations(read func(privateconnector.Operation) (privateconnector.Result, error), lease scanLease, initial Evidence) ([]Observation, error) {
	out := []Observation{}
	missing := []string{}
	optional := func(kind privateconnector.Kind, access string) (privateconnector.Result, error) {
		result, err := read(privateconnector.Operation{Kind: kind, Repository: &privateconnector.RepositoryArgs{Repository: lease.Ref}})
		var providerError *domain.ProviderError
		if errors.Is(err, privateconnector.ErrUnsupported) || errors.As(err, &providerError) && (providerError.Kind == "forbidden" || providerError.Kind == "unsupported" || providerError.Kind == "scope" || providerError.Kind == "not_found") {
			missing = append(missing, access)
			return result, nil
		}
		return result, err
	}
	issues, err := optional(privateconnector.ForgeIssues, "issues")
	if err != nil {
		return nil, err
	}
	for _, issue := range issues.Issues {
		e := initial
		e.ReferenceURL = issue.URL
		e.Review = &ReviewEvidence{Confidence: "medium", Objective: "Resolve issue #" + issue.Number + ": " + issue.Title, Detail: issue.Body}
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "forge_issue", SourceID: "issue:" + issue.Number, Category: "issue", Severity: "medium", Title: bounded(issue.Title, 512), Evidence: e})
	}
	advisories, err := optional(privateconnector.ForgeAdvisories, "Dependabot alerts")
	if err != nil {
		return nil, err
	}
	for _, advisory := range advisories.Advisories {
		e := initial
		e.AdvisoryID, e.ReferenceURL = advisory.ID, advisory.URL
		e.Dependencies = []detectors.Dependency{{Ecosystem: advisory.Ecosystem, Manifest: advisory.Manifest, Name: advisory.Package, From: advisory.Vulnerable, To: advisory.Patched}}
		e.Review = &ReviewEvidence{Confidence: "high", Objective: "Upgrade " + advisory.Package + " to a version outside " + advisory.Vulnerable + patchedHint(advisory.Patched), Detail: advisory.Summary}
		severity := advisory.Severity
		if severity == "moderate" {
			severity = "medium"
		}
		if !slices.Contains([]string{"low", "medium", "high", "critical"}, severity) {
			severity = "medium"
		}
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "forge_advisory", SourceID: "advisory:" + advisory.ID + ":" + advisory.Manifest, Category: "security_advisory", Severity: severity, Title: bounded(advisory.ID+" in "+advisory.Package+": "+advisory.Summary, 512), Evidence: e})
	}
	if len(missing) > 0 {
		e := initial
		e.Blockers = []string{"Needs a person: grant Reforge's forge access read permission for " + strings.Join(missing, " and ")}
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "repository", SourceID: "provider-access", Category: "provider_access", Severity: "low", Title: "Grant read access to " + strings.Join(missing, " and "), Evidence: e})
	}
	return out, nil
}

func patchedHint(version string) string {
	if version == "" {
		return ""
	}
	return " (first patched: " + version + ")"
}

func bounded(value string, limit int) string {
	for len(value) > limit {
		value = value[:len(value)-1]
	}
	return strings.ToValidUTF8(value, "")
}

func refreshReviewFindings(ctx context.Context, tx pgx.Tx, lease scanLease, observations []Observation) error {
	var current *Evidence
	for i := range observations {
		if observations[i].Category == "repository_review" {
			current = &observations[i].Evidence
		}
	}
	if current == nil {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM maintenance_findings WHERE org_id=$1 AND repository_id=$2 AND source='repository_review' AND state='open'`, lease.Org, lease.Repo)
	if err != nil {
		return err
	}
	var open []Finding
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			rows.Close()
			return err
		}
		open = append(open, f)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, f := range open {
		o := f.Observation
		o.Evidence.ConnectionID, o.Evidence.ConnectionVersion, o.Evidence.ConfigVersion = current.ConnectionID, current.ConnectionVersion, current.ConfigVersion
		o.Evidence.HeadSHA, o.Evidence.TargetSHA, o.Evidence.TargetBranch = current.HeadSHA, current.TargetSHA, current.TargetBranch
		if _, err = ObserveTx(ctx, tx, lease.Org, o, lease.User, "discovery-"+lease.Repo); err != nil {
			return err
		}
	}
	return nil
}

const reviewCadence = 24 * time.Hour

func reviewDue(ctx context.Context, tx pgx.Tx, org, repo, head string) (string, bool, error) {
	var id, state, reviewed string
	var seen time.Time
	err := tx.QueryRow(ctx, `SELECT id::text,state,coalesce(evidence->>'head_sha',''),last_seen FROM maintenance_findings WHERE org_id=$1 AND repository_id=$2 AND source='repository' AND source_id='repository-review'`, org, repo).Scan(&id, &state, &reviewed, &seen)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, state == "open" || reviewed != head && time.Since(seen) > reviewCadence, nil
}

var DefaultBots = map[string]string{"dependabot[bot]\x0049699333": "dependabot", "renovate[bot]\x0029139614": "renovate"}

func docsDrift(ctx context.Context, reader source.Reader, m forge.SourceManifest) (string, error) {
	entries := m.Entries
	exists := map[string]bool{}
	docs := []forge.SourceEntry{}
	for _, entry := range entries {
		exists[entry.Path] = true
		for dir := path.Dir(entry.Path); dir != "." && !exists[dir]; dir = path.Dir(dir) {
			exists[dir] = true
		}
		if entry.Type == "blob" && strings.EqualFold(path.Ext(entry.Path), ".md") && entry.Size <= 256<<10 && len(docs) < 20 && !strings.Contains("/"+entry.Path, "/node_modules/") && !strings.Contains("/"+entry.Path, "/vendor/") {
			docs = append(docs, entry)
		}
	}
	broken := []string{}
	for _, entry := range docs {
		body, err := reader.Blob(ctx, m.Repository, entry, m.ObjectFormat, m.CommitSHA)
		if err != nil {
			return "", err
		}
		for _, link := range guest.BrokenLinks(entry.Path, body, func(file string) bool { return exists[file] }) {
			if len(broken) < 20 {
				broken = append(broken, entry.Path+" -> "+link)
			}
		}
	}
	return strings.Join(broken, "\n"), nil
}

func validationGap(entries []forge.SourceEntry) string {
	ci, tests := false, false
	for _, entry := range entries {
		if entry.Type == "blob" {
			ci = ci || guest.CIPath(entry.Path)
			tests = tests || guest.TestPath(entry.Path)
		}
	}
	switch {
	case !ci && !tests:
		return "Add continuous integration and tests"
	case !ci:
		return "Add continuous integration"
	case !tests:
		return "Add tests"
	}
	return ""
}

func largeFileObservations(entries []forge.SourceEntry, repositoryID string, initial Evidence) []Observation {
	out := []Observation{}
	for _, entry := range entries {
		if entry.Type != "blob" || entry.Size < 10<<20 {
			continue
		}
		e := initial
		e.TrackedFiles = []forge.SourceEntry{entry}
		title := fmt.Sprintf("Review tracked %.1f MiB file %s", float64(entry.Size)/(1<<20), entry.Path)
		if len(title) > 512 {
			end := 512
			for !utf8.RuneStart(title[end]) {
				end--
			}
			title = title[:end]
		}
		out = append(out, Observation{RepositoryID: repositoryID, Source: "repository", SourceID: "tracked-large-file:" + digest(entry.Path), Category: "repository_maintenance", Severity: "low", Title: title, Evidence: e})
	}
	return out
}

func BotUpdateJob(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "dependabot" || name == "renovate" || name == ".github/dependabot.yml" || name == ".github/dependabot.yaml" || strings.HasPrefix(name, "dependabot ") || strings.HasPrefix(name, "renovate ")
}
func manifestFiles(ctx context.Context, reader source.Reader, repo forge.RepoRef, commit string) (map[string][]byte, forge.SourceManifest, error) {
	m, err := reader.Manifest(ctx, repo, commit)
	if err != nil {
		return nil, m, err
	}
	if m.Repository != repo || m.CommitSHA != commit {
		return nil, m, ErrStale
	}
	if _, err = source.ValidateManifest(ctx, m); err != nil {
		return nil, m, err
	}
	files := map[string][]byte{}
	total := 0
	for _, entry := range m.Entries {
		if entry.Type != "blob" {
			continue
		}
		base := path.Base(entry.Path)
		if base != "go.mod" && base != "package.json" && base != "pyproject.toml" && !(strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt")) && !strings.Contains(strings.ToLower(base), "renovate") && base != "dependabot.yml" && base != "dependabot.yaml" {
			continue
		}
		if len(files) >= 100 {
			return nil, m, errors.New("manifest count exceeds discovery bound")
		}
		content, err := reader.Blob(ctx, repo, entry, m.ObjectFormat, commit)
		if err != nil {
			return nil, m, err
		}
		if len(content) > 1<<20 || total+len(content) > 4<<20 {
			return nil, m, ErrStale
		}
		total += len(content)
		files[entry.Path] = content
	}
	return files, m, nil
}
func (s *Service) collect(ctx context.Context, lease scanLease) ([]Observation, error) {
	authorize := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		return validateScan(ctx, tx, lease, c)
	}
	read := func(op privateconnector.Operation) (privateconnector.Result, error) {
		op.ID = domain.NewID()
		result, err := s.reader.Read(ctx, lease.Org, lease.Connection, op, authorize)
		return result, readFailure(op.Kind, err)
	}
	resolved, err := read(privateconnector.Operation{Kind: privateconnector.ForgeResolveRef, Ref: &privateconnector.RefArgs{Repository: lease.Ref, Ref: lease.Branch}})
	if err != nil {
		return nil, err
	}
	if !source.ValidSHA(resolved.SHA, "sha1") {
		return nil, ErrStale
	}
	sourceReader := s.reader.SourceReader(lease.Org, lease.Connection, authorize)
	base, manifest, err := manifestFiles(ctx, sourceReader, lease.Ref, resolved.SHA)
	entries := manifest.Entries
	if err != nil {
		return nil, readFailure(privateconnector.ForgeSourceManifest, err)
	}
	botConfig := detectors.BotConfiguration(base)
	var cfg Config
	var cached []forge.Change
	err = s.db.Tenant(ctx, lease.Org, "", func(tx pgx.Tx) error {
		var err error
		cfg, err = configTx(ctx, tx, lease.Org, lease.Repo)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT snapshot FROM inventory_changes WHERE org_id=$1 AND repository_id=$2 AND snapshot->>'state' IN ('open','opened') ORDER BY native_id LIMIT 201`, lease.Org, lease.Repo)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var body []byte
			var change forge.Change
			if err = rows.Scan(&body); err != nil {
				return err
			}
			if err = json.Unmarshal(body, &change); err != nil {
				return err
			}
			cached = append(cached, change)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if len(cached) > 200 {
		return nil, errors.New("open changes exceed discovery bound")
	}
	initial := Evidence{ConnectionID: lease.Connection, ConnectionVersion: lease.Version, ConfigVersion: lease.ConfigVersion, HeadSHA: resolved.SHA, TargetSHA: resolved.SHA, TargetBranch: lease.Branch, BotConfig: botConfig, Ownership: "unknown", Complete: true, Provenance: "canonical provider reads at pinned commits"}
	out := []Observation{}
	if title := botConfigGap(base, botConfig); title != "" {
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "repository", SourceID: "dependency-bot-configuration", Category: "dependency_bots", Severity: "low", Title: title, Evidence: initial})
	}
	if title := validationGap(entries); title != "" {
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "repository", SourceID: "validation-bootstrap", Category: "missing_validation", Severity: "medium", Title: title, Evidence: initial})
	}
	if gap, err := docsDrift(ctx, sourceReader, manifest); err != nil {
		return nil, err
	} else if gap != "" {
		e := initial
		e.Review = &ReviewEvidence{Confidence: "high", Objective: "Fix or remove every broken relative link in the repository's Markdown files", Detail: gap}
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "repository", SourceID: "docs-links", Category: "docs_drift", Severity: "low", Title: "Documentation links to files that do not exist", Evidence: e})
	}
	out = append(out, Observation{RepositoryID: lease.Repo, Source: "repository", SourceID: "repository-review", Category: "repository_review", Severity: "info", Title: "Review repository for issues no rule detects", Evidence: initial})
	out = append(out, largeFileObservations(entries, lease.Repo, initial)...)
	provided, err := providerObservations(read, lease, initial)
	if err != nil {
		return nil, err
	}
	out = append(out, provided...)
	checks, err := read(privateconnector.Operation{Kind: privateconnector.ForgeChecks, Checks: &privateconnector.ChecksArgs{Repository: lease.Ref, CommitSHA: resolved.SHA}})
	if err != nil {
		return nil, err
	}
	if !validChecks(checks.Checks, resolved.SHA) {
		return nil, ErrStale
	}
	if failedChecks(checks.Checks) {
		e := initial
		e.Checks = checks.Checks
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "native_ci", SourceID: lease.Branch, Category: "ci_failure", Severity: "medium", Title: "Default branch checks failed", Evidence: e})
	}
	targetChecks := checks.Checks
	for _, candidate := range cached {
		current, err := read(privateconnector.Operation{Kind: privateconnector.ForgeReadChange, Change: &privateconnector.ChangeArgs{Repository: lease.Ref, ChangeID: candidate.ID}})
		if err != nil {
			return nil, err
		}
		if current.Change == nil {
			return nil, ErrStale
		}
		change := *current.Change
		if change.State != "open" && change.State != "opened" || strings.HasPrefix(change.HeadBranch, "reforge/") && !strings.HasPrefix(change.HeadBranch, "reforge/repair/") {
			continue
		}
		if change.Repository != lease.Ref || change.HeadRepository.NativeID == "" || change.TargetRepository != lease.Ref || !source.ValidSHA(change.HeadSHA, "sha1") || !source.ValidSHA(change.TargetSHA, "sha1") {
			return nil, ErrStale
		}
		checks, err := read(privateconnector.Operation{Kind: privateconnector.ForgeChecks, Checks: &privateconnector.ChecksArgs{Repository: change.HeadRepository, CommitSHA: change.HeadSHA}})
		if err != nil {
			return nil, err
		}
		if !validChecks(checks.Checks, change.HeadSHA) {
			return nil, ErrStale
		}
		observed := checks.Checks
		conflict := RepairConflict(change)
		if strings.HasPrefix(change.HeadBranch, "reforge/repair/") {
			missing, pending := forge.MissingChecks(targetChecks, observed, BotUpdateJob)
			if pending && !failedChecks(observed) && !conflict {
				continue
			}
			for _, name := range missing {
				observed = append(observed, forge.Check{ID: "missing:" + name, Name: name, HeadSHA: change.HeadSHA, Status: "completed", Conclusion: "missing"})
			}
		}
		if conflict {
			observed = append(observed, repairConflictCheck(change))
		}
		if !failedChecks(observed) {
			continue
		}
		baseline := base
		if change.TargetSHA != resolved.SHA {
			baseline, _, err = manifestFiles(ctx, sourceReader, lease.Ref, change.TargetSHA)
			if err != nil {
				return nil, readFailure(privateconnector.ForgeSourceManifest, err)
			}
		}
		head, _, err := manifestFiles(ctx, sourceReader, change.HeadRepository, change.HeadSHA)
		if err != nil {
			return nil, readFailure(privateconnector.ForgeSourceManifest, err)
		}
		deps := detectors.Compare(baseline, head)
		e := initial
		e.HeadSHA = change.HeadSHA
		e.TargetSHA = change.TargetSHA
		e.TargetBranch = change.TargetBranch
		e.Change = &change
		e.Checks = observed
		e.Dependencies = deps.Changes
		e.Complete = deps.Complete
		e.Blockers = deps.Reasons
		e.Ownership = "human"
		e.HeadOwnership = "unverified; changed heads invalidate previous repair evidence"
		for _, bot := range cfg.TrustedBots {
			if bot.ActorID == change.AuthorID {
				e.Bot = bot.Kind
				e.Ownership = "bot"
			}
		}
		if len(cfg.TrustedBots) == 0 && strings.EqualFold(change.AuthorType, "Bot") {
			if kind := DefaultBots[change.AuthorLogin+"\x00"+change.AuthorID]; kind != "" {
				e.Bot, e.Ownership = kind, "bot"
			}
		}
		if strings.HasPrefix(change.HeadBranch, "reforge/repair/") {
			e.Bot, e.Ownership = "", "reforge"
		} else if strings.EqualFold(change.AuthorType, "Bot") && e.Bot == "" {
			e.Ownership = "unknown"
			e.Blockers = append(e.Blockers, "Confirm immutable dependency bot actor identity in repository maintenance settings")
		}
		if e.Bot != "" {
			compared, err := read(privateconnector.Operation{Kind: privateconnector.ForgeBehind, Compare: &privateconnector.CompareArgs{Repository: lease.Ref, Base: resolved.SHA, Head: change.HeadSHA}})
			if err != nil {
				return nil, err
			}
			e.Behind = *compared.Behind
			switch {
			case waitsForTarget(targetChecks, observed):
				e.Blockers = append(e.Blockers, WaitingForTarget)
			case e.Behind > 0:
				e.Blockers = append(e.Blockers, WaitingForRebase)
			}
		}
		if cfg.MergeAuthority == "reforge" && e.Bot != "" {
			e.MergeBlockers = []string{"Verify bot automerge is disabled or constrained by a certified native Reforge gate before claiming merge authority"}
		}
		category, severity, title := "ci_failure", "medium", change.Title
		if conflict {
			category, severity, title = "branch_conflict", "high", "Reforge repair branch conflicts with target"
		} else if len(deps.Changes) > 0 {
			category = "dependency_update"
		}
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "forge_change", SourceID: change.ID, Category: category, Severity: severity, Title: title, Evidence: e})
	}
	return sharedBotFailures(out, initial, lease.Repo, lease.Branch, targetChecks), nil
}

func sharedBotFailures(out []Observation, initial Evidence, repo, branch string, target []forge.Check) []Observation {
	passing := map[string]bool{}
	for _, c := range target {
		if c.Conclusion == "success" {
			passing[c.Name] = true
		}
	}
	seen := map[string]int{}
	for _, o := range out {
		if o.Evidence.Bot != "" && o.Evidence.Behind == 0 {
			for name := range failingChecks(o.Evidence.Checks) {
				seen[name]++
			}
		}
	}
	shared := map[string]bool{}
	for name, count := range seen {
		if count >= 2 && !passing[name] {
			shared[name] = true
		}
	}
	if len(shared) == 0 {
		return out
	}
	var sample *Observation
	for i := range out {
		o := &out[i]
		if o.Evidence.Bot == "" || o.Evidence.Behind > 0 || slices.Contains(o.Evidence.Blockers, WaitingForTarget) {
			continue
		}
		failing := failingChecks(o.Evidence.Checks)
		covered := len(failing) > 0
		for name := range failing {
			covered = covered && shared[name]
		}
		if covered {
			o.Evidence.Blockers = append(o.Evidence.Blockers, WaitingForTarget)
			if sample == nil {
				sample = o
			}
		}
	}
	if sample == nil || failedChecks(target) {
		return out
	}
	e := initial
	e.Checks = sample.Evidence.Checks
	return append(out, Observation{RepositoryID: repo, Source: "native_ci", SourceID: branch, Category: "ci_failure", Severity: "high", Title: "Default branch fails CI on every dependency update", Evidence: e})
}

type scanReadError struct {
	kind privateconnector.Kind
	err  error
}

func (e *scanReadError) Error() string { return string(e.kind) + ": " + e.err.Error() }
func (e *scanReadError) Unwrap() error { return e.err }
func (e *scanReadError) access() string {
	switch e.kind {
	case privateconnector.ForgeResolveRef, privateconnector.ForgeSourceManifest, privateconnector.ForgeReadFile, privateconnector.ForgeBehind:
		return "repository contents"
	case privateconnector.ForgeChecks:
		return "check results"
	case privateconnector.ForgeReadChange:
		return "pull requests"
	}
	return ""
}
func readFailure(kind privateconnector.Kind, err error) error {
	if err == nil {
		return nil
	}
	return &scanReadError{kind: kind, err: err}
}
func rateLimitRetry(err error) (time.Duration, bool) {
	var providerError *domain.ProviderError
	if !errors.As(err, &providerError) || providerError.Kind != "rate_limit" && providerError.Kind != "rate_limited" {
		return 0, false
	}
	return min(max(5*time.Minute, providerError.RetryAfter), 24*time.Hour), true
}

func (s *Service) step(ctx context.Context, lease scanLease) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	observations, readErr := s.collect(ctx, lease)
	if readErr != nil {
		code, detail, operation := "internal", "", ""
		var providerError *domain.ProviderError
		var failed *scanReadError
		if errors.As(readErr, &providerError) {
			code, detail = providerError.Kind, providerError.Message
		} else if errors.Is(readErr, ErrStale) {
			code = "stale"
		}
		if errors.As(readErr, &failed) {
			operation = string(failed.kind)
		}
		slog.WarnContext(ctx, "discovery scan failed", "org_id", lease.Org, "repository_id", lease.Repo, "operation", operation, "code", code, "detail", detail, "error", readErr.Error())
	}
	persistCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	persistErr := s.db.Tenant(persistCtx, lease.Org, "", func(tx pgx.Tx) error {
		if err := lockOrg(persistCtx, tx, lease.Org); err != nil {
			return err
		}
		var version int64
		var state string
		if err := tx.QueryRow(persistCtx, `SELECT version,state FROM connections WHERE org_id=$1 AND id=$2 FOR SHARE`, lease.Org, lease.Connection).Scan(&version, &state); err != nil {
			return err
		}
		if err := validateScan(persistCtx, tx, lease, connections.Connection{ID: lease.Connection, OrgID: lease.Org, Version: version, State: state}); err != nil {
			readErr = err
		}
		if readErr != nil {
			state := "stale"
			reason := "Provider read failed; verify connection and refresh inventory before retrying discovery"
			retryDelay := 5 * time.Minute
			if delay, limited := rateLimitRetry(readErr); limited {
				retryDelay = delay
				reason = "Provider rate limit reached; retry scheduled"
			}
			var providerError *domain.ProviderError
			if errors.As(readErr, &providerError) && (providerError.Kind == "unauthenticated" || providerError.Kind == "forbidden" || providerError.Kind == "unsupported" || providerError.Kind == "auth" || providerError.Kind == "scope") {
				state = "failed"
				reason = "Provider access or capability unavailable; verify connection permissions and supported server version, then restart discovery"
				var failed *scanReadError
				if errors.As(readErr, &failed) && failed.access() != "" {
					reason = "The forge token cannot read " + failed.access() + "; give it read access to " + failed.access() + ", then start the scan again"
				}
			}
			if errors.Is(readErr, auth.ErrForbidden) {
				state = "failed"
				reason = "Discovery requester lost repository access"
			}
			_, err := tx.Exec(persistCtx, `UPDATE maintenance_scans SET state=$4,reason=$5,lease_until=NULL,available_at=clock_timestamp()+$6::double precision * interval '1 second',version=version+1 WHERE org_id=$1 AND repository_id=$2 AND fence=$3 AND state='running'`, lease.Org, lease.Repo, lease.Fence, state, reason, int64(retryDelay/time.Second))
			return err
		}
		seen := []string{}
		for _, observation := range observations {
			if observation.Category == "repository_review" {
				id, due, err := reviewDue(persistCtx, tx, lease.Org, lease.Repo, observation.Evidence.HeadSHA)
				if err != nil {
					return err
				}
				if !due {
					if id != "" {
						seen = append(seen, id)
					}
					continue
				}
			}
			f, err := ObserveTx(persistCtx, tx, lease.Org, observation, lease.User, "discovery-"+lease.Repo)
			if err != nil {
				return err
			}
			seen = append(seen, f.ID)
		}
		if err := refreshReviewFindings(persistCtx, tx, lease, observations); err != nil {
			return err
		}
		rows, err := tx.Query(persistCtx, `UPDATE maintenance_findings SET state='resolved',reason='No longer present in complete canonical discovery',version=version+1 WHERE org_id=$1 AND repository_id=$2 AND source NOT IN ('imported_advisory','repository_review') AND state='open' AND NOT(id=ANY($3::uuid[])) RETURNING `+columns, lease.Org, lease.Repo, seen)
		if err != nil {
			return err
		}
		var resolved []Finding
		for rows.Next() {
			f, e := scanFinding(rows)
			if e != nil {
				rows.Close()
				return e
			}
			resolved = append(resolved, f)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, f := range resolved {
			if err = record(persistCtx, tx, f, lease.User, "resolved", "discovery-"+lease.Repo); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(persistCtx, `UPDATE maintenance_scans SET state='complete',reason='',lease_until=NULL,observed_at=clock_timestamp(),available_at=clock_timestamp()+interval '5 minutes',version=version+1 WHERE org_id=$1 AND repository_id=$2 AND fence=$3 AND state='running' AND lease_until>clock_timestamp()`, lease.Org, lease.Repo, lease.Fence)
		if err == nil && tag.RowsAffected() != 1 {
			return ErrStale
		}
		return err
	})
	if persistErr != nil {
		return persistErr
	}
	return readErr
}
func (s *Service) nextTenant(ctx context.Context) (string, error) {
	var org string
	err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('reforge.maintenance_scheduler','true',true)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance_scheduler_cursor(id,org_id) VALUES(true,'00000000-0000-0000-0000-000000000000') ON CONFLICT DO NOTHING`); err != nil {
			return err
		}
		var cursor string
		if err := tx.QueryRow(ctx, `SELECT org_id::text FROM maintenance_scheduler_cursor WHERE id=true FOR UPDATE`).Scan(&cursor); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT org_id::text FROM inventory_tenants ORDER BY CASE WHEN org_id>$1::uuid THEN 0 ELSE 1 END,org_id LIMIT 1`, cursor).Scan(&org)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE maintenance_scheduler_cursor SET org_id=$1 WHERE id=true`, org)
		return err
	})
	return org, err
}
func (s *Service) RunOnce(ctx context.Context) (bool, error) {
	if s.reader == nil {
		return false, ErrBlocked
	}
	first := ""
	for i := 0; i < 50; i++ {
		org, err := s.nextTenant(ctx)
		if err != nil || org == "" || org == first {
			return false, err
		}
		if first == "" {
			first = org
		}
		lease, err := s.claim(ctx, org)
		if err != nil {
			return false, err
		}
		if lease != nil {
			return true, s.step(ctx, *lease)
		}
	}
	return false, nil
}

func (s *Service) RunOrganisationOnce(ctx context.Context, org string) (bool, error) {
	if !auth.ValidID(org) || s.reader == nil {
		return false, auth.ErrInvalid
	}
	lease, err := s.claim(ctx, org)
	if err != nil || lease == nil {
		return false, err
	}
	return true, s.step(ctx, *lease)
}
func (s *Service) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		worked, err := s.RunOnce(ctx)
		heartbeat.Beat("discovery", 3*time.Minute, err)
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return ctx.Err()
}
