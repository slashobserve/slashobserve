-- name: CreateComment :one
INSERT INTO dashboard_comments (
  dashboard_id, version, panel_id, body, author_kind, author_id, author_name, author_email
) VALUES (
  @dashboard_id, @version, @panel_id, @body, @author_kind, @author_id, @author_name, @author_email
)
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
