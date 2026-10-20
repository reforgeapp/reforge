package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
)

var ErrInvalidQualification = errors.New("agent qualification is invalid")

type QualificationService struct {
	db   *store.Store
	auth *auth.Service
}

func NewQualificationService(db *store.Store, identity *auth.Service) *QualificationService {
	return &QualificationService{db: db, auth: identity}
}

func manage(a domain.Actor) bool { return a.Role == domain.Owner || a.Role == domain.Admin }

func validQualification(q Qualification) bool {
	if !auth.ValidID(q.Binding.OrgID) || !auth.ValidID(q.Binding.ConnectionID) || !auth.ValidID(q.EvidenceID) {
		return false
	}
	if q.CheckedAt.IsZero() || q.CheckedAt.After(time.Now()) || !q.ExpiresAt.After(q.CheckedAt) || !q.ExpiresAt.After(time.Now()) || q.ExpiresAt.After(time.Now().Add(400*24*time.Hour)) {
		return false
	}
	return true
}

func (s *QualificationService) Put(ctx context.Context, session auth.Session, org, connectionID string, q Qualification) (Qualification, error) {
	if session.AutomationID() != "" {
		return Qualification{}, auth.ErrForbidden
	}
	if !validQualification(q) || q.Binding.ConnectionID != connectionID || q.Binding.OrgID != org {
		return Qualification{}, ErrInvalidQualification
	}
	e := s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		raw, err := json.Marshal(q)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO agent_qualifications(org_id,connection_id,evidence_id,document,checked_at,expires_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(org_id,connection_id) DO UPDATE SET evidence_id=excluded.evidence_id,document=excluded.document,checked_at=excluded.checked_at,expires_at=excluded.expires_at,created_by=excluded.created_by,updated_at=now()`, org, connectionID, q.EvidenceID, raw, q.CheckedAt, q.ExpiresAt, a.UserID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,'agent.qualification',$4,$5,$6)`, domain.NewID(), org, a.UserID, connectionID, connectionID, raw)
		return err
	})
	return q, e
}

func (s *QualificationService) Get(ctx context.Context, session auth.Session, org, connectionID string) (Qualification, bool, error) {
	var q Qualification
	found := false
	if !auth.ValidID(connectionID) {
		return q, false, auth.ErrInvalid
	}
	e := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		var raw []byte
		e := tx.QueryRow(ctx, `SELECT document FROM agent_qualifications WHERE org_id=$1 AND connection_id=$2`, org, connectionID).Scan(&raw)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		found = true
		return json.Unmarshal(raw, &q)
	})
	return q, found, e
}

func (s *QualificationService) Delete(ctx context.Context, session auth.Session, org, connectionID string) error {
	if session.AutomationID() != "" || !auth.ValidID(connectionID) {
		return auth.ErrInvalid
	}
	return s.auth.WithMutation(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		if _, e := tx.Exec(ctx, `DELETE FROM agent_qualifications WHERE org_id=$1 AND connection_id=$2`, org, connectionID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,action,object_id,request_id,data) VALUES($1,$2,$3,'agent.qualification_cleared',$4,$5,'{}'::jsonb)`, domain.NewID(), org, a.UserID, connectionID, connectionID)
		return e
	})
}

func runtimeKey(provider string) string {
	switch strings.ToLower(provider) {
	case "codex":
		return "codex"
	case "claude", "claude_code":
		return "claude"
	case "agy", "antigravity":
		return "agy"
	case "gemini":
		return "gemini"
	default:
		return "codex"
	}
}

func Capabilities(q Qualification, b Binding, provider string, found bool) map[string]domain.Capability {
	key := runtimeKey(provider)
	result := map[string]domain.Capability{}
	matrix := OfficialSupportMatrix()
	if entry, ok := matrix[key]; ok {
		result["runtime"] = entry
	} else {
		result["runtime"] = disabledFeature("runtime has no qualification contract", "choose a supported official runtime")
	}
	valid := found && q.Binding == b && auth.ValidID(q.EvidenceID) && !q.CheckedAt.IsZero() && q.ExpiresAt.After(time.Now())
	if valid {
		for name, capability := range qualificationFeatures(q, b) {
			result[name] = capability
		}
	}
	return result
}
