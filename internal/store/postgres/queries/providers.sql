-- name: CreateProvider :one
INSERT INTO providers (id, slug, name, kind, settings)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
