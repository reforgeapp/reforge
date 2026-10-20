-- name: GetOrganisation :one
SELECT id, name, version, paused, created_at FROM organisations WHERE id = $1;

-- name: SetOrganisationPause :one
UPDATE organisations SET paused = $2, version = version + 1 WHERE id = $1 AND version = $3 RETURNING id, name, version, paused, created_at;
