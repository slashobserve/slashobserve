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
SELECT a.id, a.dashboard_id, a.panel_id, a.time_from, a.time_to, a.text, a.tags, a.created_by, a.created_at,
  COALESCE(NULLIF(u.name, ''), u.email, sa.name, '')::text AS created_by_name
FROM annotations a
LEFT JOIN users u ON u.id = a.created_by
LEFT JOIN service_accounts sa ON sa.id = a.created_by
WHERE (a.dashboard_id IS NULL OR a.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('window_to')::timestamptz IS NULL OR a.time_from <= sqlc.narg('window_to')::timestamptz)
  AND (sqlc.narg('window_from')::timestamptz IS NULL OR COALESCE(a.time_to, a.time_from) >= sqlc.narg('window_from')::timestamptz)
  AND (cardinality(@tags::text[]) = 0 OR a.tags && @tags::text[])
  AND (sqlc.narg('cursor_time')::timestamptz IS NULL
    OR (a.time_from, a.id) < (sqlc.narg('cursor_time')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY a.time_from DESC, a.id DESC
LIMIT @row_limit;
