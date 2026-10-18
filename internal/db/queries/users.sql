-- name: ListUsers :many
SELECT u.id, u.email, u.name, u.created_at, u.last_login_at, u.last_seen_at,
  coalesce(dc.n, 0)::integer AS dashboard_count,
  coalesce(kc.n, 0)::integer AS key_count,
  coalesce(sc.n, 0)::integer AS session_count
FROM users u
LEFT JOIN (SELECT author_id, count(*) AS n FROM dashboard_versions
  WHERE version = 1 AND author_kind = 'user' GROUP BY author_id) dc ON dc.author_id = u.id
LEFT JOIN (SELECT created_by, count(*) AS n FROM service_account_keys
  WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now()) GROUP BY created_by) kc ON kc.created_by = u.id
LEFT JOIN (SELECT user_id, count(*) AS n FROM sessions WHERE expires_at > now() GROUP BY user_id) sc ON sc.user_id = u.id
WHERE sqlc.narg('cursor_created_at')::timestamptz IS NULL
  OR (u.created_at, u.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
ORDER BY u.created_at DESC, u.id DESC
LIMIT @row_limit;

-- name: UserExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE id = @id);

-- name: DeleteUserSessions :execrows
DELETE FROM sessions WHERE user_id = @user_id;
