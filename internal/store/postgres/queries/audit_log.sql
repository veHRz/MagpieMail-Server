-- name: AppendAuditEvent :exec
INSERT INTO audit_log (id, occurred_at, actor_user_id, action, target_type, target_id, client_ip, details)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListAuditEvents :many
-- Newest first. A NULL actor lists every actor.
SELECT * FROM audit_log
WHERE (sqlc.narg(actor)::uuid IS NULL OR actor_user_id = sqlc.narg(actor))
  AND occurred_at < sqlc.arg(before)
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg(max_events);
