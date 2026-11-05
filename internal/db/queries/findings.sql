-- name: InsertFinding :one
INSERT INTO findings (agent_request_id, dashboard_id, title, markdown, evidence, time_from, time_to,
  created_by_kind, created_by_id, created_by_name)
VALUES (sqlc.narg('agent_request_id')::uuid, sqlc.narg('dashboard_id')::uuid, @title, @markdown, @evidence::jsonb,
  @time_from, @time_to, @created_by_kind, @created_by_id, @created_by_name)
ON CONFLICT (agent_request_id) WHERE agent_request_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetFinding :one
SELECT f.*, r.context_to AS request_to
FROM findings f
LEFT JOIN agent_requests r ON r.id = f.agent_request_id
WHERE f.id = @id;

-- name: GetFindingByAgentRequest :one
SELECT * FROM findings WHERE agent_request_id = @agent_request_id;

-- name: ListFindingsByDashboard :many
SELECT * FROM findings
WHERE dashboard_id = @dashboard_id
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: ListFindingsBy :many
SELECT f.* FROM findings f
WHERE f.created_by_kind = @created_by_kind AND f.created_by_id = @created_by_id
  AND (f.dashboard_id IS NULL OR f.dashboard_id IN (SELECT d.id FROM dashboards d WHERE d.folder_id = ANY(@folders::uuid[])))
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (f.created_at, f.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT @row_limit;

-- name: SetFindingAnnotation :execrows
UPDATE findings SET annotation_id = @annotation_id WHERE id = @id AND annotation_id IS NULL;
