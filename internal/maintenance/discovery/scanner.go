package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path"
	"reforge/internal/heartbeat"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/inventory"
	"reforge/internal/maintenance/detectors"
	"reforge/internal/privateconnector"
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
			_, err = tx.Exec(ctx, `UPDATE maintenance_scans SET state='queued',available_at=clock_timestamp()+interval '5 seconds',reason='Waiting for a fresh native inventory observation',version=version+1 WHERE org_id=$1 AND repository_id=$2`, org, lease.Repo)
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

func BotUpdateJob(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "dependabot" || name == "renovate" || strings.HasPrefix(name, "dependabot ") || strings.HasPrefix(name, "renovate ")
}
func manifestFiles(ctx context.Context, reader source.Reader, repo forge.RepoRef, commit string) (map[string][]byte, error) {
	m, err := reader.Manifest(ctx, repo, commit)
	if err != nil {
		return nil, err
	}
	if m.Repository != repo || m.CommitSHA != commit {
		return nil, ErrStale
	}
	if _, err = source.ValidateManifest(ctx, m); err != nil {
		return nil, err
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
			return nil, errors.New("manifest count exceeds discovery bound")
		}
		content, err := reader.Blob(ctx, repo, entry, m.ObjectFormat, commit)
		if err != nil {
			return nil, err
		}
		if len(content) > 1<<20 || total+len(content) > 4<<20 {
			return nil, ErrStale
		}
		total += len(content)
		files[entry.Path] = content
	}
	return files, nil
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
	base, err := manifestFiles(ctx, sourceReader, lease.Ref, resolved.SHA)
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
		rows, err := tx.Query(ctx, `SELECT snapshot FROM inventory_changes WHERE org_id=$1 AND repository_id=$2 AND snapshot->>'state'='open' ORDER BY native_id LIMIT 201`, lease.Org, lease.Repo)
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
		if change.State != "open" || strings.HasPrefix(change.HeadBranch, "reforge/") && !strings.HasPrefix(change.HeadBranch, "reforge/repair/") {
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
		if strings.HasPrefix(change.HeadBranch, "reforge/repair/") {
			missing, pending := forge.MissingChecks(targetChecks, observed, BotUpdateJob)
			if pending && !failedChecks(observed) {
				continue
			}
			for _, name := range missing {
				observed = append(observed, forge.Check{ID: "missing:" + name, Name: name, HeadSHA: change.HeadSHA, Status: "completed", Conclusion: "missing"})
			}
		}
		if !failedChecks(observed) {
			continue
		}
		baseline := base
		if change.TargetSHA != resolved.SHA {
			baseline, err = manifestFiles(ctx, sourceReader, lease.Ref, change.TargetSHA)
			if err != nil {
				return nil, readFailure(privateconnector.ForgeSourceManifest, err)
			}
		}
		head, err := manifestFiles(ctx, sourceReader, change.HeadRepository, change.HeadSHA)
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
		category := "ci_failure"
		if len(deps.Changes) > 0 {
			category = "dependency_update"
		}
		out = append(out, Observation{RepositoryID: lease.Repo, Source: "forge_change", SourceID: change.ID, Category: category, Severity: "medium", Title: change.Title, Evidence: e})
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
			_, err := tx.Exec(persistCtx, `UPDATE maintenance_scans SET state=$4,reason=$5,lease_until=NULL,available_at=clock_timestamp()+interval '5 minutes',version=version+1 WHERE org_id=$1 AND repository_id=$2 AND fence=$3 AND state='running'`, lease.Org, lease.Repo, lease.Fence, state, reason)
			return err
		}
		seen := []string{}
		for _, observation := range observations {
			f, err := ObserveTx(persistCtx, tx, lease.Org, observation, lease.User, "discovery-"+lease.Repo)
			if err != nil {
				return err
			}
			seen = append(seen, f.ID)
		}
		rows, err := tx.Query(persistCtx, `UPDATE maintenance_findings SET state='resolved',reason='No longer present in complete canonical discovery',version=version+1 WHERE org_id=$1 AND repository_id=$2 AND source<>'imported_advisory' AND state='open' AND NOT(id=ANY($3::uuid[])) RETURNING `+columns, lease.Org, lease.Repo, seen)
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
