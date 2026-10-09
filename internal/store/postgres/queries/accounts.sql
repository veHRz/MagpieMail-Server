-- name: CreateAccount :one
INSERT INTO accounts (id, user_id, provider_id, email, display_name, auth_method, credentials_encrypted)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1 AND user_id = $2;

-- name: SetAccountCredentials :execrows
UPDATE accounts
SET credentials_encrypted = $3, updated_at = now()
WHERE id = $1 AND user_id = $2;
