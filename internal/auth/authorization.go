package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"reforge/internal/domain"
)

type Team struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	RepositoryIDs []string `json:"repository_ids"`
	Version       int64    `json:"version"`
}

type RepositoryResolver func(context.Context, pgx.Tx, string, string) (string, error)

func loadBindings(ctx context.Context, tx pgx.Tx, m *Membership) error {
	if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(team_id::text ORDER BY team_id),'{}') FROM team_memberships WHERE org_id=$1 AND user_id=$2`, m.OrgID, m.UserID).Scan(&m.TeamIDs); err != nil {
		return err
	}
	return tx.QueryRow(ctx, `SELECT coalesce(array_agg(repository_id::text ORDER BY repository_id),'{}') FROM member_repositories WHERE org_id=$1 AND user_id=$2`, m.OrgID, m.UserID).Scan(&m.RepositoryIDs)
}

func liveActor(ctx context.Context, tx pgx.Tx, session Session, orgID string) (domain.Actor, error) {
	a := domain.Actor{UserID: session.User.ID, OrgID: orgID, SessionID: session.ID}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>now())`, session.ID, session.User.ID).Scan(&active); err != nil {
		return a, err
	}
	if !active {
		return a, ErrUnauthenticated
	}
	m := Membership{OrgID: orgID, UserID: session.User.ID}
	if err := tx.QueryRow(ctx, `SELECT role,all_repositories,version FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, session.User.ID).Scan(&m.Role, &m.AllRepositories, &m.Version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return a, ErrForbidden
		}
		return a, err
	}
	if err := loadBindings(ctx, tx, &m); err != nil {
		return a, err
	}
	a.Role = m.Role
	a.AllRepositories = m.AllRepositories
	a.TeamIDs = m.TeamIDs
	if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(repository_id::text ORDER BY repository_id),'{}') FROM (SELECT repository_id FROM member_repositories WHERE org_id=$1 AND user_id=$2 UNION SELECT tr.repository_id FROM team_repositories tr JOIN team_memberships tm ON tm.org_id=tr.org_id AND tm.team_id=tr.team_id WHERE tm.org_id=$1 AND tm.user_id=$2) r`, orgID, session.User.ID).Scan(&a.RepositoryIDs); err != nil {
		return a, err
	}
	return a, nil
}

func (s *Service) WithActor(ctx context.Context, session Session, orgID string, fn func(pgx.Tx, domain.Actor) error) error {
	if !ValidID(orgID) {
		return ErrForbidden
	}
	return s.db.Tenant(ctx, orgID, session.User.ID, func(tx pgx.Tx) error {
		actor, err := liveActor(ctx, tx, session, orgID)
		if err != nil {
			return err
		}
		return fn(tx, actor)
	})
}

func (s *Service) ResolveActor(ctx context.Context, session Session, orgID string) (domain.Actor, error) {
	var actor domain.Actor
	err := s.WithActor(ctx, session, orgID, func(_ pgx.Tx, a domain.Actor) error { actor = a; return nil })
	return actor, err
}

func (s *Service) RequireRepository(ctx context.Context, session Session, orgID, repositoryID string) (domain.Actor, error) {
	return s.ResolveRepository(ctx, session, orgID, repositoryID, func(ctx context.Context, tx pgx.Tx, orgID, id string) (string, error) {
		var found string
		err := tx.QueryRow(ctx, `SELECT id::text FROM repositories WHERE org_id=$1 AND id=$2`, orgID, id).Scan(&found)
		return found, err
	})
}

func (s *Service) ResolveRepository(ctx context.Context, session Session, orgID, objectID string, resolve RepositoryResolver) (domain.Actor, error) {
	var actor domain.Actor
	if !ValidID(objectID) || resolve == nil {
		return actor, ErrForbidden
	}
	err := s.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		repoID, err := resolve(ctx, tx, orgID, objectID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
		if !CanReadRepository(a, repoID) {
			return ErrForbidden
		}
		actor = a
		return nil
	})
	return actor, err
}

func CanReadRepository(actor domain.Actor, repositoryID string) bool {
	if actor.AllRepositories {
		return true
	}
	for _, id := range actor.RepositoryIDs {
		if id == repositoryID {
			return true
		}
	}
	return false
}

func ValidID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func audit(ctx context.Context, tx pgx.Tx, orgID, userID, action, objectID, requestID string, version int64) error {
	data, _ := json.Marshal(map[string]int64{"version": version})
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), orgID, userID, action, objectID, requestID, data)
	return err
}

func (s *Service) Members(ctx context.Context, session Session, orgID string, limit int, cursor string) (domain.Page[Membership], error) {
	page := domain.Page[Membership]{Items: []Membership{}}
	err := s.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner {
			return ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT user_id::text,role,all_repositories,version FROM memberships WHERE org_id=$1 AND user_id::text>$2 ORDER BY user_id LIMIT $3`, orgID, cursor, limit+1)
		if err != nil {
			return err
		}
		for rows.Next() {
			m := Membership{OrgID: orgID}
			if err = rows.Scan(&m.UserID, &m.Role, &m.AllRepositories, &m.Version); err != nil {
				rows.Close()
				return err
			}
			page.Items = append(page.Items, m)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		page.Complete = len(page.Items) <= limit
		if !page.Complete {
			page.Items = page.Items[:limit]
			page.NextCursor = page.Items[limit-1].UserID
		}
		for i := range page.Items {
			if err = loadBindings(ctx, tx, &page.Items[i]); err != nil {
				return err
			}
		}
		return nil
	})
	return page, err
}

func (s *Service) PutMember(ctx context.Context, session Session, orgID, userID string, input Membership, expected int64, requestID string) (Membership, error) {
	input.OrgID = orgID
	input.UserID = userID
	if !ValidID(userID) || expected < 0 || !validRole(input.Role) || !validIDs(input.TeamIDs) || !validIDs(input.RepositoryIDs) {
		return input, ErrInvalid
	}
	err := s.db.Tenant(ctx, orgID, session.User.ID, func(tx pgx.Tx) error {
		if !ValidID(orgID) {
			return ErrForbidden
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM organisations WHERE id=$1 FOR UPDATE`, orgID); err != nil {
			return err
		}
		if err := lockSession(ctx, tx, session); err != nil {
			return err
		}
		a, err := liveActor(ctx, tx, session, orgID)
		if err != nil {
			return err
		}
		if a.Role != domain.Owner {
			return ErrForbidden
		}
		var currentRole domain.Role
		var version int64
		err = tx.QueryRow(ctx, `SELECT role,version FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&currentRole, &version)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if version != expected {
			return ErrConflict
		}
		if currentRole == domain.Owner && input.Role != domain.Owner {
			if err = protectOwner(ctx, tx, orgID); err != nil {
				return err
			}
		}
		input.Version = version + 1
		_, err = tx.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role,all_repositories,version) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,user_id) DO UPDATE SET role=EXCLUDED.role,all_repositories=EXCLUDED.all_repositories,version=EXCLUDED.version`, orgID, userID, input.Role, input.AllRepositories, input.Version)
		if err != nil {
			return hideConstraint(err)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM team_memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM member_repositories WHERE org_id=$1 AND user_id=$2`, orgID, userID); err != nil {
			return err
		}
		for _, id := range input.TeamIDs {
			if _, err = tx.Exec(ctx, `INSERT INTO team_memberships(org_id,user_id,team_id) VALUES($1,$2,$3)`, orgID, userID, id); err != nil {
				return hideConstraint(err)
			}
		}
		for _, id := range input.RepositoryIDs {
			if _, err = tx.Exec(ctx, `INSERT INTO member_repositories(org_id,user_id,repository_id) VALUES($1,$2,$3)`, orgID, userID, id); err != nil {
				return hideConstraint(err)
			}
		}
		return audit(ctx, tx, orgID, a.UserID, "membership.updated", userID, requestID, input.Version)
	})
	return input, err
}

func (s *Service) DeleteMember(ctx context.Context, session Session, orgID, userID string, expected int64, requestID string) error {
	if !ValidID(orgID) || !ValidID(userID) || expected < 1 {
		return ErrInvalid
	}
	return s.db.Tenant(ctx, orgID, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM organisations WHERE id=$1 FOR UPDATE`, orgID); err != nil {
			return err
		}
		if err := lockSession(ctx, tx, session); err != nil {
			return err
		}
		a, err := liveActor(ctx, tx, session, orgID)
		if err != nil {
			return err
		}
		if a.Role != domain.Owner {
			return ErrForbidden
		}
		var role domain.Role
		var version int64
		if err = tx.QueryRow(ctx, `SELECT role,version FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&role, &version); errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		} else if err != nil {
			return err
		}
		if version != expected {
			return ErrConflict
		}
		if role == domain.Owner {
			if err = protectOwner(ctx, tx, orgID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, orgID, userID); err != nil {
			return err
		}
		return audit(ctx, tx, orgID, a.UserID, "membership.deleted", userID, requestID, version+1)
	})
}

func protectOwner(ctx context.Context, tx pgx.Tx, orgID string) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE org_id=$1 AND role='owner'`, orgID).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrConflict
	}
	return nil
}
func validRole(r domain.Role) bool {
	return r == domain.Owner || r == domain.Admin || r == domain.Maintainer || r == domain.Reviewer || r == domain.Viewer
}
func validIDs(ids []string) bool {
	if len(ids) > 200 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !ValidID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func hideConstraint(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && (p.Code == "23503" || p.Code == "23505") {
		return ErrInvalid
	}
	return err
}

func (s *Service) Teams(ctx context.Context, session Session, orgID string, limit int, cursor string) (domain.Page[Team], error) {
	page := domain.Page[Team]{Items: []Team{}}
	err := s.WithActor(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `SELECT id::text,name,version FROM teams WHERE org_id=$1 AND id::text>$2 AND ($3 OR id::text=ANY($4::text[])) ORDER BY id LIMIT $5`, orgID, cursor, a.AllRepositories, a.TeamIDs, limit+1)
		if err != nil {
			return err
		}
		for rows.Next() {
			var team Team
			if err = rows.Scan(&team.ID, &team.Name, &team.Version); err != nil {
				rows.Close()
				return err
			}
			page.Items = append(page.Items, team)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		page.Complete = len(page.Items) <= limit
		if !page.Complete {
			page.Items = page.Items[:limit]
			page.NextCursor = page.Items[limit-1].ID
		}
		for i := range page.Items {
			team := &page.Items[i]
			if err = tx.QueryRow(ctx, `SELECT coalesce(array_agg(repository_id::text ORDER BY repository_id),'{}') FROM team_repositories WHERE org_id=$1 AND team_id=$2`, orgID, team.ID).Scan(&team.RepositoryIDs); err != nil {
				return err
			}
		}
		return nil
	})
	return page, err
}

func (s *Service) PutTeam(ctx context.Context, session Session, orgID, teamID string, input Team, expected int64, requestID string) (Team, error) {
	input.ID = teamID
	input.Name = strings.TrimSpace(input.Name)
	if !ValidID(teamID) || len(input.Name) < 1 || len(input.Name) > 160 || expected < 0 || !validIDs(input.RepositoryIDs) {
		return input, ErrInvalid
	}
	err := s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if a.Role != domain.Owner && a.Role != domain.Admin {
			return ErrForbidden
		}
		if !a.AllRepositories {
			found := false
			for _, id := range a.TeamIDs {
				if id == teamID {
					found = true
				}
			}
			if !found {
				return ErrForbidden
			}
		}
		for _, id := range input.RepositoryIDs {
			if !CanReadRepository(a, id) {
				return ErrForbidden
			}
		}
		var version int64
		err := tx.QueryRow(ctx, `SELECT version FROM teams WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, teamID).Scan(&version)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if version != expected {
			return ErrConflict
		}
		input.Version = version + 1
		if version == 0 {
			_, err = tx.Exec(ctx, `INSERT INTO teams(org_id,id,name,version) VALUES($1,$2,$3,$4)`, orgID, teamID, input.Name, input.Version)
		} else {
			_, err = tx.Exec(ctx, `UPDATE teams SET name=$3,version=$4 WHERE org_id=$1 AND id=$2`, orgID, teamID, input.Name, input.Version)
		}
		if err != nil {
			return hideConstraint(err)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM team_repositories WHERE org_id=$1 AND team_id=$2`, orgID, teamID); err != nil {
			return err
		}
		for _, id := range input.RepositoryIDs {
			if _, err = tx.Exec(ctx, `INSERT INTO team_repositories(org_id,team_id,repository_id) VALUES($1,$2,$3)`, orgID, teamID, id); err != nil {
				return hideConstraint(err)
			}
		}
		return audit(ctx, tx, orgID, a.UserID, "team.updated", teamID, requestID, input.Version)
	})
	return input, err
}

func (s *Service) DeleteTeam(ctx context.Context, session Session, orgID, teamID string, expected int64, requestID string) error {
	if !ValidID(teamID) || expected < 1 {
		return ErrInvalid
	}
	return s.WithMutation(ctx, session, orgID, func(tx pgx.Tx, a domain.Actor) error {
		if (a.Role != domain.Owner && a.Role != domain.Admin) || !a.AllRepositories {
			return ErrForbidden
		}
		result, err := tx.Exec(ctx, `DELETE FROM teams WHERE org_id=$1 AND id=$2 AND version=$3`, orgID, teamID, expected)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrConflict
		}
		return audit(ctx, tx, orgID, a.UserID, "team.deleted", teamID, requestID, expected+1)
	})
}

func lockSession(ctx context.Context, tx pgx.Tx, session Session) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM sessions WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>now() FOR SHARE`, session.ID, session.User.ID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthenticated
	}
	return err
}
func (s *Service) WithMutation(ctx context.Context, session Session, orgID string, fn func(pgx.Tx, domain.Actor) error) error {
	if !ValidID(orgID) {
		return ErrForbidden
	}
	return s.db.Tenant(ctx, orgID, session.User.ID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM organisations WHERE id=$1 FOR UPDATE`, orgID); err != nil {
			return err
		}
		if err := lockSession(ctx, tx, session); err != nil {
			return err
		}
		actor, err := liveActor(ctx, tx, session, orgID)
		if err != nil {
			return err
		}
		return fn(tx, actor)
	})
}

func (s *Service) ActorTx(ctx context.Context, tx pgx.Tx, session Session, org string) (domain.Actor, error) {
	if !ValidID(org) || !ValidID(session.User.ID) {
		return domain.Actor{}, ErrForbidden
	}
	var current string
	if err := tx.QueryRow(ctx, `SELECT current_setting('reforge.org_id',true)`).Scan(&current); err != nil {
		return domain.Actor{}, err
	}
	if current != org {
		return domain.Actor{}, ErrForbidden
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('reforge.user_id',$1,true)`, session.User.ID); err != nil {
		return domain.Actor{}, err
	}
	if err := lockSession(ctx, tx, session); err != nil {
		return domain.Actor{}, err
	}
	return liveActor(ctx, tx, session, org)
}
