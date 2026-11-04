package dbgen

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const getOrganisation = `-- name: GetOrganisation :one
SELECT id, name, version, paused, created_at FROM organisations WHERE id = $1
`

type GetOrganisationRow struct {
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	Version   int64              `json:"version"`
	Paused    bool               `json:"paused"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

func (q *Queries) GetOrganisation(ctx context.Context, id pgtype.UUID) (GetOrganisationRow, error) {
	row := q.db.QueryRow(ctx, getOrganisation, id)
	var i GetOrganisationRow
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

type SetOrganisationPauseRow struct {
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	Version   int64              `json:"version"`
	Paused    bool               `json:"paused"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

func (q *Queries) SetOrganisationPause(ctx context.Context, arg SetOrganisationPauseParams) (SetOrganisationPauseRow, error) {
	row := q.db.QueryRow(ctx, setOrganisationPause, arg.ID, arg.Paused, arg.Version)
	var i SetOrganisationPauseRow
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Version,
		&i.Paused,
		&i.CreatedAt,
	)
	return i, err
}
