-- name: InsertSavedSearch :one
INSERT INTO saved_searches (kind, title, datasource_uid, query, filters, time_from, time_to, folder_id,
  created_by_kind, created_by_id, created_by_name, origin, origin_client)
VALUES (@kind, @title, @datasource_uid, sqlc.narg('query')::text, @filters::jsonb, @time_from, @time_to, @folder_id,
  @created_by_kind, @created_by_id, @created_by_name, sqlc.narg('origin')::text, sqlc.narg('origin_client')::text)
RETURNING *;

-- name: GetSavedSearch :one
SELECT * FROM saved_searches WHERE id = @id;

-- name: LockSavedSearchForDelete :one
SELECT * FROM saved_searches WHERE id = @id FOR UPDATE;

-- name: ListSavedSearches :many
SELECT * FROM saved_searches
WHERE folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('folder_id')::uuid IS NULL OR folder_id = sqlc.narg('folder_id')::uuid)
  AND (sqlc.narg('cursor_updated_at')::timestamptz IS NULL
    OR (updated_at, id) < (sqlc.narg('cursor_updated_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY updated_at DESC, id DESC
LIMIT @row_limit;

-- name: RenameSavedSearch :one
UPDATE saved_searches SET title = @title, updated_at = clock_timestamp() WHERE id = @id RETURNING *;

-- name: MoveSavedSearch :one
UPDATE saved_searches SET folder_id = @folder_id, updated_at = clock_timestamp() WHERE id = @id RETURNING *;

-- name: DeleteSavedSearch :execrows
DELETE FROM saved_searches WHERE id = @id;
