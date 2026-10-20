package dbgen

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const getOrganisation = `-- name: GetOrganisation :one
SELECT id, name, version, paused, created_at FROM organisations WHERE id = $1
`

func (q *Queries) GetOrganisation(ctx context.Context, id pgtype.UUID) (Organisation, error) {
	row := q.db.QueryRow(ctx, getOrganisation, id)
	var i Organisation
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Version,
		&i.Paused,
		&i.CreatedAt,
	)
	return i, err
}

const setOrganisationPause = `-- name: SetOrganisationPause :one
UPDATE organisations SET paused = $2, version = version + 1 WHERE id = $1 AND version = $3 RETURNING id, name, version, paused, created_at
`

type SetOrganisationPauseParams struct {
	ID      pgtype.UUID `json:"id"`
	Paused  bool        `json:"paused"`
	Version int64       `json:"version"`
}

func (q *Queries) SetOrganisationPause(ctx context.Context, arg SetOrganisationPauseParams) (Organisation, error) {
	row := q.db.QueryRow(ctx, setOrganisationPause, arg.ID, arg.Paused, arg.Version)
	var i Organisation
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Version,
		&i.Paused,
		&i.CreatedAt,
	)
	return i, err
}
