-- name: InsertFinding :one
INSERT INTO findings (id, agent_request_id, dashboard_id, observation_id, title, markdown, evidence, time_from, time_to,
  created_by_kind, created_by_id, created_by_name, origin, origin_client)
VALUES (coalesce(sqlc.narg('id')::uuid, gen_random_uuid()), sqlc.narg('agent_request_id')::uuid, sqlc.narg('dashboard_id')::uuid, @observation_id,
  @title, @markdown, @evidence::jsonb, @time_from, @time_to, @created_by_kind, @created_by_id, @created_by_name,
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text)
ON CONFLICT (agent_request_id) WHERE agent_request_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetFinding :one
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id, r.context_to AS request_to
FROM findings f
JOIN observations o ON o.id = f.observation_id
LEFT JOIN agent_requests r ON r.id = f.agent_request_id
WHERE f.id = @id;

-- name: GetFindingByAgentRequest :one
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id
FROM findings f
JOIN observations o ON o.id = f.observation_id
WHERE f.agent_request_id = @agent_request_id;

-- name: ListFindingsByDashboard :many
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id
FROM findings f
JOIN observations o ON o.id = f.observation_id
WHERE f.dashboard_id = @dashboard_id
  AND o.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (f.created_at, f.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT @row_limit;

-- name: ListFindingsBy :many
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id
FROM findings f
JOIN observations o ON o.id = f.observation_id
WHERE f.created_by_kind = @created_by_kind AND f.created_by_id = @created_by_id
  AND o.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (f.created_at, f.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT @row_limit;

-- name: ListFindingsInFolder :many
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id
FROM findings f
JOIN observations o ON o.id = f.observation_id
WHERE o.folder_id = @folder_id AND f.dashboard_id IS NULL
  AND o.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (f.created_at, f.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT @row_limit;

-- name: SetFindingAnnotation :execrows
UPDATE findings SET annotation_id = @annotation_id WHERE id = @id AND annotation_id IS NULL;
