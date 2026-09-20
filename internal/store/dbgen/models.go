package dbgen

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type AuditEvent struct {
	ID           pgtype.UUID        `json:"id"`
	OrgID        pgtype.UUID        `json:"org_id"`
	RepositoryID pgtype.UUID        `json:"repository_id"`
	ActorID      string             `json:"actor_id"`
	Action       string             `json:"action"`
	ObjectID     string             `json:"object_id"`
	RequestID    string             `json:"request_id"`
	Data         []byte             `json:"data"`
	OccurredAt   pgtype.Timestamptz `json:"occurred_at"`
}

type Organisation struct {
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	Version   int64              `json:"version"`
	Paused    bool               `json:"paused"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}
