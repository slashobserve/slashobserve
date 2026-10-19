-- name: CreateComment :one
WITH clock AS (SELECT clock_timestamp() AS at)
INSERT INTO dashboard_comments (
  dashboard_id, version, panel_id, parent_id, body, author_kind, author_id, author_name, author_email, created_at, updated_at
)
SELECT
  @dashboard_id::uuid, @version::integer, sqlc.narg('panel_id')::text, sqlc.narg('parent_id')::uuid, @body::text, @author_kind::text,
  @author_id::uuid, @author_name::text, sqlc.narg('author_email')::text, clock.at, clock.at
FROM clock
RETURNING *;

-- name: GetComment :one
SELECT * FROM dashboard_comments WHERE id = @id AND dashboard_id = @dashboard_id;

-- name: ListComments :many
SELECT sqlc.embed(c), coalesce(root.status, c.status)::text AS thread_status
FROM dashboard_comments c
LEFT JOIN dashboard_comments root ON root.id = c.parent_id
WHERE c.dashboard_id = @dashboard_id
  AND (sqlc.narg('status')::text IS NULL OR coalesce(root.status, c.status) = sqlc.narg('status')::text)
  AND (sqlc.narg('panel_id')::text IS NULL OR c.panel_id = sqlc.narg('panel_id')::text)
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (c.created_at, c.id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY c.created_at, c.id
LIMIT @row_limit;

-- name: SetCommentStatus :one
UPDATE dashboard_comments
SET status = @status::text,
  resolved_at = CASE WHEN @status::text = 'resolved' THEN now() END,
  resolved_by_kind = sqlc.narg('resolved_by_kind')::text,
  resolved_by_id = sqlc.narg('resolved_by_id')::uuid,
  resolved_by_name = sqlc.narg('resolved_by_name')::text,
  resolved_by_version = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_by_version')::integer END,
  updated_at = now()
WHERE id = @id AND dashboard_id = @dashboard_id AND parent_id IS NULL AND status <> @status::text
RETURNING *;
