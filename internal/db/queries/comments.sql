-- name: CreateComment :one
WITH clock AS (SELECT clock_timestamp() AS at)
INSERT INTO dashboard_comments (
  dashboard_id, version, panel_id, body, author_kind, author_id, author_name, author_email, created_at, updated_at
)
SELECT
  @dashboard_id::uuid, @version::integer, @panel_id::text, @body::text, @author_kind::text,
  @author_id::uuid, @author_name::text, sqlc.narg('author_email')::text, clock.at, clock.at
FROM clock
RETURNING *;

-- name: GetComment :one
SELECT * FROM dashboard_comments WHERE id = @id AND dashboard_id = @dashboard_id;

-- name: ListComments :many
SELECT * FROM dashboard_comments
WHERE dashboard_id = @dashboard_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('panel_id')::text IS NULL OR panel_id = sqlc.narg('panel_id')::text)
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at, id
LIMIT @row_limit;

-- name: SetCommentStatus :one
UPDATE dashboard_comments
SET status = @status::text,
  resolved_at = CASE WHEN @status::text = 'resolved' THEN now() END,
  resolved_by_kind = sqlc.narg('resolved_by_kind')::text,
  resolved_by_id = sqlc.narg('resolved_by_id')::uuid,
  resolved_by_name = sqlc.narg('resolved_by_name')::text,
  updated_at = now()
WHERE id = @id AND dashboard_id = @dashboard_id AND status <> @status::text
RETURNING *;
