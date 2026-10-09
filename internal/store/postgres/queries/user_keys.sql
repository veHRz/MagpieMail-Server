-- name: InsertUserKey :exec
INSERT INTO user_keys (user_id, master_key_id, wrapped_key)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO NOTHING;

-- name: GetUserKey :one
SELECT * FROM user_keys WHERE user_id = $1;

-- name: LockUserKeys :many
SELECT * FROM user_keys ORDER BY user_id FOR UPDATE;

-- name: RewrapUserKey :exec
UPDATE user_keys
SET master_key_id = $2, wrapped_key = $3, rotated_at = now()
WHERE user_id = $1;

-- name: CountUserKeysByMasterKey :many
SELECT master_key_id, count(*) AS keys
FROM user_keys
GROUP BY master_key_id
ORDER BY master_key_id;
