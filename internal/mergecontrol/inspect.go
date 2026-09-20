package mergecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/maintenance/repair"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/source"
)

func (s *Service) Inspect(ctx context.Context, session auth.Session, org, repo, change, method, request string) (Gate, error) {
	var out Gate
	n, err := strconv.ParseInt(change, 10, 64)
	if !auth.ValidID(repo) || err != nil || n < 1 || strconv.FormatInt(n, 10) != change || len(method) > 40 {
		return out, auth.ErrInvalid
	}
	var cfg Configuration
	var ref forge.RepoRef
	var id string
	err = s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		var e error
		ref, id, e = repository(ctx, tx, org, repo)
		if e != nil {
			return e
		}
		cfg, e = configTx(ctx, tx, org, repo)
		return e
	})
	if err != nil {
		return out, err
	}
	var connection connections.Connection
	check := func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		current, err := configTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if current.Version != cfg.Version {
			return auth.ErrConflict
		}
		_, currentID, err := repository(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if c.ID != id || currentID != id {
			return auth.ErrConflict
		}
		connection = c
		return nil
	}
	result, err := s.providers.ForProtection(cfg.InspectorConnectionID, cfg.CheckPublishers).Read(ctx, org, id, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeMergeInspect, Change: &privateconnector.ChangeArgs{Repository: ref, ChangeID: change}}, check)
	if err != nil {
		return out, err
	}
	if result.MergeEvidence == nil || result.MergeEvidence.Change.ID != change || result.MergeEvidence.Change.Repository != ref {
		return out, privateconnector.ErrInvalid
	}
	paths, lines, err := s.changedPaths(ctx, org, id, *result.MergeEvidence, check)
	if err != nil {
		return out, err
	}
	companions, err := s.inspectCompanions(ctx, org, repo, id, result.MergeEvidence.Change, check)
	if err != nil {
		return out, err
	}
	err = s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if err := check(ctx, tx, connection); err != nil {
			return err
		}
		current, err := s.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if current.State != "healthy" || current.Version != connection.Version {
			return auth.ErrConflict
		}
		resolved, err := s.policies.ResolveTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		authority, err := s.authorityTx(ctx, tx, org, repo, cfg, current, *result.MergeEvidence)
		if err != nil {
			return err
		}
		authority.Paths, authority.PathsVerified = paths, true
		validCompanions, err := validateCompanionsTx(ctx, tx, org, Gate{RepositoryID: repo, Snapshot: *result.MergeEvidence, Companions: companions})
		if err != nil {
			return err
		}
		authority.CompanionsBlocked = !validCompanions
		files := int64(len(paths))
		authority.Usage.ChangedFiles, authority.Usage.ChangedLines = &files, &lines
		out = Evaluate(*result.MergeEvidence, resolved, method, authority, time.Now().UTC())
		out.Companions = companions
		out.Paths, out.ChangedLines = paths, lines
		out.ID, out.RepositoryID, out.ConnectionID, out.ConnectionVersion, out.ConfigurationVersion = domain.NewID(), repo, id, current.Version, cfg.Version
		raw, _ := json.Marshal(out)
		if _, err = tx.Exec(ctx, `INSERT INTO merge_gates(org_id,id,repository_id,change_id,configuration_version,document) VALUES($1,$2,$3,$4,$5,$6)`, org, out.ID, repo, change, cfg.Version, raw); err != nil {
			return err
		}
		return emit(ctx, tx, org, repo, a.UserID, "merge.evaluated", out.ID, 1, request, map[string]any{"gate_id": out.ID, "outcome": out.Decision.Outcome})
	})
	return out, err
}

func (s *Service) changedPaths(ctx context.Context, org, id string, snapshot Snapshot, check func(context.Context, pgx.Tx, connections.Connection) error) ([]string, int64, error) {
	reader := s.providers.SourceReader(org, id, check)
	target, err := source.Fetch(ctx, reader, snapshot.Change.TargetRepository, snapshot.Change.TargetSHA)
	if err != nil {
		return nil, 0, err
	}
	head, err := source.Fetch(ctx, reader, snapshot.Change.HeadRepository, snapshot.Change.HeadSHA)
	if err != nil {
		return nil, 0, err
	}
	left, err := repair.Files(target)
	if err != nil {
		return nil, 0, err
	}
	right, err := repair.Files(head)
	if err != nil {
		return nil, 0, err
	}
	paths := []string{}
	var lines int64
	for path, body := range left {
		next, present := right[path]
		if !present || !bytes.Equal(body, next) {
			paths = append(paths, path)
			lines += int64(bytes.Count(body, []byte{'\n'}) + bytes.Count(next, []byte{'\n'}) + 2)
		}
	}
	for path, body := range right {
		if _, present := left[path]; !present {
			paths = append(paths, path)
			lines += int64(bytes.Count(body, []byte{'\n'}) + 1)
		}
	}
	sort.Strings(paths)
	return paths, lines, nil
}

func (s *Service) authorityTx(ctx context.Context, tx pgx.Tx, org, repo string, cfg Configuration, c connections.Connection, snapshot Snapshot) (Authority, error) {
	now := time.Now().UTC()
	q := cfg.Qualification
	out := Authority{CooperationVerified: cfg.CooperationReference != "", QualificationReference: q.EvidenceReference, ExactHeadEnforced: q.ExactHead}
	zero, one := int64(0), int64(1)
	var concurrency, open int64
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM merge_operations WHERE org_id=$1 AND state IN ('requested','dispatching','queued','reconciling')),(SELECT count(*) FROM inventory_changes WHERE org_id=$1 AND snapshot->>'state' IN ('open','opened'))`, org).Scan(&concurrency, &open); err != nil {
		return out, err
	}
	concurrency = max(1, concurrency)
	out.Usage = policy.Limits{Budget: &zero, Concurrency: &concurrency, Attempts: &one, OpenChanges: &open}
	out.Qualified = cfg.Enabled && q.ConnectionVersion == c.Version && q.Provider == c.Provider && q.ServerVersion == c.ServerVersion && snapshot.Capabilities.ServerVersion == q.ServerVersion && snapshot.Capabilities.Provider == q.Provider && !q.VerifiedAt.After(now) && q.ExpiresAt.After(now) && q.ExactHead && q.StrictTarget && !snapshot.Rules.RequireQueue
	if cfg.InspectorConnectionID != "" {
		inspector, err := s.connections.MetadataTx(ctx, tx, org, cfg.InspectorConnectionID)
		if err != nil {
			return out, err
		}
		out.Qualified = out.Qualified && inspector.State == "healthy" && inspector.Version == q.InspectorVersion
	}
	var mode string
	err := tx.QueryRow(ctx, `SELECT merge_authority FROM maintenance_configs WHERE org_id=$1 AND repository_id=$2`, org, repo).Scan(&mode)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	out.MergeControlled = cfg.Enabled && mode == "reforge"
	var task string
	var rawContext, rawReport, rawChecks []byte
	err = tx.QueryRow(ctx, `SELECT task_id::text,context,report,candidate_checks FROM repair_runs WHERE org_id=$1 AND repository_id=$2 AND state='published' AND candidate_sha=$3 AND native_change->>'id'=$4 ORDER BY created_at DESC LIMIT 1`, org, repo, snapshot.Change.HeadSHA, snapshot.Change.ID).Scan(&task, &rawContext, &rawReport, &rawChecks)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var execution repair.ExecutionContext
	var report repair.Report
	var checks []repair.CheckResult
	if json.Unmarshal(rawContext, &execution) != nil || json.Unmarshal(rawReport, &report) != nil || json.Unmarshal(rawChecks, &checks) != nil {
		return out, privateconnector.ErrInvalid
	}
	if report.State == "validated" && execution.Plan.TargetSHA == snapshot.Change.TargetSHA && repair.Verified(execution.Plan, report.Baseline, checks) {
		out.ValidationHead, out.ValidationTarget, out.ValidationReference = snapshot.Change.HeadSHA, snapshot.Change.TargetSHA, "repair:"+task
	}
	for _, patch := range report.Patches {
		out.Paths = append(out.Paths, patch.Path)
	}
	return out, nil
}
