package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/inventory"
	"reforge/internal/store"
	"reforge/internal/workflow"
)

type Service struct {
	db     *store.Store
	auth   *auth.Service
	reader Reader
}

func New(db *store.Store, identity *auth.Service, reader Reader) *Service {
	return &Service{db, identity, reader}
}
func manage(a domain.Actor, repo string) bool {
	return auth.CanReadRepository(a, repo) && (a.Role == domain.Owner || a.Role == domain.Admin || a.Role == domain.Maintainer)
}
func hidden(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrForbidden
	}
	return err
}
func lockOrg(ctx context.Context, tx pgx.Tx, org string) error {
	var id string
	return hidden(tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, org).Scan(&id))
}
func memberScope(ctx context.Context, tx pgx.Tx, org, user, repo string, write bool) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m WHERE m.org_id=$1 AND m.user_id=$2 AND (NOT $4 OR m.role IN ('owner','admin','maintainer')) AND (m.all_repositories OR EXISTS(SELECT 1 FROM member_repositories mr WHERE mr.org_id=m.org_id AND mr.user_id=m.user_id AND mr.repository_id=$3) OR EXISTS(SELECT 1 FROM team_memberships tm JOIN team_repositories tr ON tr.org_id=tm.org_id AND tr.team_id=tm.team_id WHERE tm.org_id=m.org_id AND tm.user_id=m.user_id AND tr.repository_id=$3)))`, org, user, repo, write).Scan(&allowed)
	if err == nil && !allowed {
		return auth.ErrForbidden
	}
	return err
}

const columns = `id::text,org_id::text,repository_id::text,fingerprint,source,source_id,category,severity,title,evidence,evidence_digest,state,reason,coalesce(assigned_to::text,''),snooze_until,coalesce(superseded_by::text,''),version,first_seen,last_seen`

func scanFinding(row pgx.Row) (Finding, error) {
	var f Finding
	var body []byte
	err := row.Scan(&f.ID, &f.OrgID, &f.RepositoryID, &f.Fingerprint, &f.Source, &f.SourceID, &f.Category, &f.Severity, &f.Title, &body, &f.EvidenceDigest, &f.State, &f.Reason, &f.AssignedTo, &f.SnoozeUntil, &f.SupersededBy, &f.Version, &f.FirstSeen, &f.LastSeen)
	if err == nil {
		err = json.Unmarshal(body, &f.Evidence)
	}
	return f, hidden(err)
}
func load(ctx context.Context, tx pgx.Tx, org, id string) (Finding, error) {
	return scanFinding(tx.QueryRow(ctx, `SELECT `+columns+` FROM maintenance_findings WHERE org_id=$1 AND id=$2`, org, id))
}
func record(ctx context.Context, tx pgx.Tx, f Finding, actor, action, request string) error {
	body, _ := json.Marshal(map[string]any{"version": f.Version, "evidence_digest": f.EvidenceDigest})
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data,repository_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, domain.NewID(), f.OrgID, actor, "finding."+action, f.ID, request, body, f.RepositoryID); err != nil {
		return err
	}
	return workflow.EmitTx(ctx, tx, domain.Event{OrgID: f.OrgID, RepositoryID: f.RepositoryID, Type: "finding." + action, AggregateType: "finding", AggregateID: f.ID, AggregateVersion: f.Version, RequestID: request, DataVersion: 1, Data: body})
}
func (s *Service) Get(ctx context.Context, session auth.Session, org, id string) (Finding, error) {
	var f Finding
	if !auth.ValidID(id) {
		return f, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		f, err = load(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, f.RepositoryID) {
			return auth.ErrForbidden
		}
		return nil
	})
	return f, err
}
func (s *Service) List(ctx context.Context, session auth.Session, org string, limit int, cursor string, filter Filter) (domain.Page[Finding], error) {
	out := domain.Page[Finding]{Items: []Finding{}}
	if limit < 1 || limit > 200 || cursor != "" && !auth.ValidID(cursor) || filter.RepositoryID != "" && !auth.ValidID(filter.RepositoryID) || len(filter.Query) > 256 {
		return out, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM maintenance_findings WHERE org_id=$1 AND ($2 OR repository_id=ANY($3::uuid[])) AND ($4='' OR id>nullif($4,'')::uuid) AND ($6='' OR repository_id=nullif($6,'')::uuid) AND ($7='' OR state=$7) AND ($8='' OR category=$8) AND ($9='' OR severity=$9) AND ($10='' OR strpos(lower(title),lower($10))>0) ORDER BY id LIMIT $5`, org, a.AllRepositories, a.RepositoryIDs, cursor, limit+1, filter.RepositoryID, filter.State, filter.Category, filter.Severity, filter.Query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			f, err := scanFinding(rows)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, f)
		}
		return rows.Err()
	})
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	out.Complete = out.NextCursor == ""
	return out, err
}
func (s *Service) Update(ctx context.Context, session auth.Session, org, id string, in Update, expected int64, request string) (Finding, error) {
	var f Finding
	if !auth.ValidID(id) || expected < 1 || len(in.Reason) > 2000 || in.AssignedTo != "" && !auth.ValidID(in.AssignedTo) {
		return f, auth.ErrInvalid
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var err error
		f, err = load(ctx, tx, org, id)
		if err != nil {
			return err
		}
		if !manage(a, f.RepositoryID) {
			return auth.ErrForbidden
		}
		if f.Version != expected {
			return auth.ErrConflict
		}
		switch in.Action {
		case "assign":
			if in.AssignedTo != "" {
				if err = memberScope(ctx, tx, org, in.AssignedTo, f.RepositoryID, false); err != nil {
					return err
				}
			}
			f.AssignedTo = in.AssignedTo
		case "dismiss", "snooze", "reopen":
			if strings.TrimSpace(in.Reason) == "" || f.State == "superseded" {
				return auth.ErrInvalid
			}
			f.Reason = in.Reason
			f.SnoozeUntil = nil
			if in.Action == "dismiss" {
				f.State = "dismissed"
			}
			if in.Action == "reopen" {
				f.State = "open"
			}
			if in.Action == "snooze" {
				if in.SnoozeUntil == nil || !in.SnoozeUntil.After(time.Now()) || in.SnoozeUntil.After(time.Now().Add(366*24*time.Hour)) {
					return auth.ErrInvalid
				}
				f.State = "snoozed"
				f.SnoozeUntil = in.SnoozeUntil
			}
		default:
			return auth.ErrInvalid
		}
		f.Version++
		_, err = tx.Exec(ctx, `UPDATE maintenance_findings SET state=$3,reason=$4,assigned_to=nullif($5,'')::uuid,snooze_until=$6,version=$7 WHERE org_id=$1 AND id=$2`, org, id, f.State, f.Reason, f.AssignedTo, f.SnoozeUntil, f.Version)
		if err != nil {
			return err
		}
		return record(ctx, tx, f, a.UserID, in.Action, request)
	})
	return f, err
}
func (s *Service) GetConfig(ctx context.Context, session auth.Session, org, repo string) (Config, error) {
	var c Config
	if !auth.ValidID(repo) {
		return c, auth.ErrInvalid
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		var err error
		c, err = configTx(ctx, tx, org, repo)
		return err
	})
	return c, err
}
func configTx(ctx context.Context, tx pgx.Tx, org, repo string) (Config, error) {
	c := Config{RepositoryID: repo, TrustedBots: []BotIdentity{}, MergeAuthority: "observe"}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT trusted_bots,merge_authority,version FROM maintenance_configs WHERE org_id=$1 AND repository_id=$2`, org, repo).Scan(&raw, &c.MergeAuthority, &c.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		var id string
		err = tx.QueryRow(ctx, `SELECT id::text FROM repositories WHERE org_id=$1 AND id=$2`, org, repo).Scan(&id)
		return c, hidden(err)
	}
	if err == nil {
		err = json.Unmarshal(raw, &c.TrustedBots)
	}
	return c, err
}
func (s *Service) PutConfig(ctx context.Context, session auth.Session, org, repo string, in Config, expected int64, request string) (Config, error) {
	var out Config
	if !auth.ValidID(repo) || expected < 0 || len(in.TrustedBots) > 20 || (in.MergeAuthority != "observe" && in.MergeAuthority != "reforge" && in.MergeAuthority != "bot") {
		return out, auth.ErrInvalid
	}
	seen := map[string]bool{}
	for _, bot := range in.TrustedBots {
		if (bot.Kind != "renovate" && bot.Kind != "dependabot") || bot.ActorID == "" || len(bot.ActorID) > 128 || strings.TrimSpace(bot.ActorID) != bot.ActorID || seen[bot.ActorID] {
			return out, auth.ErrInvalid
		}
		seen[bot.ActorID] = true
	}
	err := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.CanReadRepository(a, repo) || (a.Role != domain.Owner && a.Role != domain.Admin) {
			return auth.ErrForbidden
		}
		old, err := configTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if old.Version != expected {
			return auth.ErrConflict
		}
		out = in
		out.RepositoryID = repo
		out.Version = expected + 1
		if out.TrustedBots == nil {
			out.TrustedBots = []BotIdentity{}
		}
		raw, _ := json.Marshal(out.TrustedBots)
		_, err = tx.Exec(ctx, `INSERT INTO maintenance_configs(org_id,repository_id,trusted_bots,merge_authority,version) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,repository_id) DO UPDATE SET trusted_bots=excluded.trusted_bots,merge_authority=excluded.merge_authority,version=excluded.version`, org, repo, raw, out.MergeAuthority, out.Version)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]any{"version": out.Version})
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data,repository_id) VALUES($1,$2,$3,'maintenance.config',$4::text,$5,$6,$4::uuid)`, domain.NewID(), org, a.UserID, repo, request, body)
		return err
	})
	return out, err
}
func PrepareRepairTx(ctx context.Context, tx pgx.Tx, org, id string, expected int64) (Finding, error) {
	var f Finding
	if err := lockOrg(ctx, tx, org); err != nil {
		return f, err
	}
	f, err := load(ctx, tx, org, id)
	if err != nil {
		return f, err
	}
	if f.Version != expected || f.State != "open" || time.Since(f.LastSeen) > 15*time.Minute {
		return f, ErrStale
	}
	if !f.Evidence.Complete || len(f.Evidence.Blockers) > 0 {
		return f, ErrBlocked
	}
	if err = inventory.RequireFreshTx(ctx, tx, org, f.RepositoryID); err != nil {
		return f, err
	}
	config, err := configTx(ctx, tx, org, f.RepositoryID)
	if err != nil {
		return f, err
	}
	if config.Version != f.Evidence.ConfigVersion {
		return f, ErrStale
	}
	var current int64
	if err = tx.QueryRow(ctx, `SELECT version FROM connections WHERE org_id=$1 AND id=$2`, org, f.Evidence.ConnectionID).Scan(&current); err != nil {
		return f, err
	}
	if current != f.Evidence.ConnectionVersion {
		return f, ErrStale
	}
	overlaps, err := tx.Query(ctx, `SELECT evidence FROM maintenance_findings WHERE org_id=$1 AND repository_id=$2 AND id<>$3 AND state='open' AND evidence->>'bot' IN ('renovate','dependabot')`, org, f.RepositoryID, f.ID)
	if err != nil {
		return f, err
	}
	for overlaps.Next() {
		var raw []byte
		var e Evidence
		if err = overlaps.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &e)
		}
		if err != nil {
			overlaps.Close()
			return f, err
		}
		if e.Bot != f.Evidence.Bot && e.TargetBranch == f.Evidence.TargetBranch && Overlap(e.Dependencies, f.Evidence.Dependencies) {
			overlaps.Close()
			return f, ErrDuplicate
		}
	}
	err = overlaps.Err()
	overlaps.Close()
	if err != nil {
		return f, err
	}
	rows, err := tx.Query(ctx, `SELECT mf.id::text,mf.evidence FROM maintenance_repairs mr JOIN maintenance_findings mf ON mf.org_id=mr.org_id AND mf.id=mr.finding_id JOIN workflow_tasks t ON t.org_id=mr.org_id AND t.id=mr.task_id WHERE mr.org_id=$1 AND mr.repository_id=$2 AND mr.active`, org, f.RepositoryID)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var other string
		var body []byte
		var evidence Evidence
		if err = rows.Scan(&other, &body); err != nil {
			return f, err
		}
		if err = json.Unmarshal(body, &evidence); err != nil {
			return f, err
		}
		if other == f.ID || evidence.TargetBranch == f.Evidence.TargetBranch && Overlap(evidence.Dependencies, f.Evidence.Dependencies) {
			return f, ErrDuplicate
		}
	}
	return f, rows.Err()
}
func BindRepairTx(ctx context.Context, tx pgx.Tx, f Finding, taskID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO maintenance_repairs(org_id,finding_id,repository_id,task_id,evidence_digest) VALUES($1,$2,$3,$4,$5)`, f.OrgID, f.ID, f.RepositoryID, taskID, f.EvidenceDigest)
	return err
}
