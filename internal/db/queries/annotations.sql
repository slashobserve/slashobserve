-- name: InsertAnnotation :one
INSERT INTO annotations (dashboard_id, panel_id, time_from, time_to, text, tags, created_by)
VALUES (sqlc.narg('dashboard_id')::uuid, @panel_id, @time_from, sqlc.narg('time_to')::timestamptz, @text, @tags::text[], @created_by)
RETURNING id, dashboard_id, panel_id, time_from, time_to, text, tags, created_by, created_at;

-- name: GetAnnotation :one
SELECT id, dashboard_id, panel_id, time_from, time_to, text, tags, created_by, created_at
FROM annotations
WHERE id = @id;

-- name: DeleteAnnotation :execrows
DELETE FROM annotations WHERE id = @id;

-- name: ListAnnotations :many
SELECT id, dashboard_id, panel_id, time_from, time_to, text, tags, created_by, created_at
FROM annotations
WHERE (dashboard_id IS NULL OR dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('window_to')::timestamptz IS NULL OR time_from <= sqlc.narg('window_to')::timestamptz)
  AND (sqlc.narg('window_from')::timestamptz IS NULL OR COALESCE(time_to, time_from) >= sqlc.narg('window_from')::timestamptz)
  AND (cardinality(@tags::text[]) = 0 OR tags && @tags::text[])
  AND (sqlc.narg('cursor_time')::timestamptz IS NULL
    OR (time_from, id) < (sqlc.narg('cursor_time')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY time_from DESC, id DESC
LIMIT @row_limit;
