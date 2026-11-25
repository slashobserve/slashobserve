-- name: InsertFinding :one
INSERT INTO findings (id, agent_request_id, dashboard_id, ask_dashboard_id, observation_id, title, markdown, evidence, time_from, time_to,
  created_by_kind, created_by_id, created_by_name, origin, origin_client, suggested_change, observe_worthy, as_of,
  root_cause_component, root_cause_kind, root_cause_summary, onset, impact, confidence, ruled_out, next_steps, no_incident)
VALUES (coalesce(sqlc.narg('id')::uuid, gen_random_uuid()), sqlc.narg('agent_request_id')::uuid, sqlc.narg('dashboard_id')::uuid, sqlc.narg('ask_dashboard_id')::uuid, @observation_id,
  @title, @markdown, @evidence::jsonb, @time_from, @time_to, @created_by_kind, @created_by_id, @created_by_name,
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('suggested_change')::text, @observe_worthy, sqlc.narg('as_of')::timestamptz,
  sqlc.narg('root_cause_component')::text, sqlc.narg('root_cause_kind')::text, sqlc.narg('root_cause_summary')::text, sqlc.narg('onset')::timestamptz,
  sqlc.narg('impact')::text, sqlc.narg('confidence')::text, sqlc.narg('ruled_out')::text[], sqlc.narg('next_steps')::text[], sqlc.narg('no_incident')::boolean)
ON CONFLICT (agent_request_id) WHERE agent_request_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetFinding :one
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id, r.context_to AS request_to,
  r.requested_by_kind AS requested_by_kind, r.requested_by_id AS requested_by_id, ask.title AS ask_dashboard_title,
  r.thread_message_id AS request_thread_message_id, COALESCE(run_answer_kept_with_admins(r.id, '{}'), false)::boolean AS kept_with_admins
FROM findings f
LEFT JOIN observations o ON o.id = f.observation_id
LEFT JOIN agent_requests r ON r.id = f.agent_request_id
LEFT JOIN dashboards ask ON ask.id = f.ask_dashboard_id
WHERE f.id = @id;

-- name: GetFindingByAgentRequest :one
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id,
  r.requested_by_kind AS requested_by_kind, r.requested_by_id AS requested_by_id, ask.title AS ask_dashboard_title,
  r.thread_message_id AS request_thread_message_id, run_answer_kept_with_admins(r.id, '{}')::boolean AS kept_with_admins
FROM findings f
LEFT JOIN observations o ON o.id = f.observation_id
JOIN agent_requests r ON r.id = f.agent_request_id
LEFT JOIN dashboards ask ON ask.id = f.ask_dashboard_id
WHERE f.agent_request_id = @agent_request_id;

-- name: ListFindingsByDashboard :many
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id, COALESCE(run_answer_kept_with_admins(r.id, '{}'), false)::boolean AS kept_with_admins
FROM findings f
JOIN observations o ON o.id = f.observation_id
LEFT JOIN agent_requests r ON r.id = f.agent_request_id
WHERE f.dashboard_id = @dashboard_id
  AND o.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (f.created_at, f.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT @row_limit;

-- name: ListFindingsBy :many
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id, COALESCE(run_answer_kept_with_admins(r.id, '{}'), false)::boolean AS kept_with_admins
FROM findings f
JOIN observations o ON o.id = f.observation_id
LEFT JOIN agent_requests r ON r.id = f.agent_request_id
WHERE f.created_by_kind = @created_by_kind AND f.created_by_id = @created_by_id
  AND o.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (f.created_at, f.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT @row_limit;

-- name: ListFindingsInFolder :many
SELECT sqlc.embed(f), o.folder_id AS observation_folder_id, COALESCE(run_answer_kept_with_admins(r.id, '{}'), false)::boolean AS kept_with_admins
FROM findings f
JOIN observations o ON o.id = f.observation_id
LEFT JOIN agent_requests r ON r.id = f.agent_request_id
WHERE o.folder_id = @folder_id AND f.dashboard_id IS NULL
  AND o.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (f.created_at, f.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT @row_limit;

-- name: SetFindingAnnotation :execrows
UPDATE findings SET annotation_id = @annotation_id WHERE id = @id AND annotation_id IS NULL;

-- name: LockFinding :one
SELECT observation_id FROM findings WHERE id = @id FOR UPDATE;

-- name: UnsetFindingObservation :exec
UPDATE findings SET observation_id = NULL WHERE agent_request_id = @agent_request_id AND observation_id = @observation_id;

-- name: SetFindingObservation :execrows
UPDATE findings SET observation_id = @observation_id WHERE id = @id AND observation_id IS NULL;

-- name: ListObservationFindingFields :many
SELECT f.* FROM findings f
WHERE f.observation_id = @observation_id
  AND f.agent_request_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM observation_blocks b
    WHERE b.observation_id = f.observation_id AND b.run_id = f.agent_request_id AND b.kind = 'text'
  )
  AND (root_cause_kind IS NOT NULL OR onset IS NOT NULL OR impact IS NOT NULL OR confidence IS NOT NULL
    OR ruled_out IS NOT NULL OR next_steps IS NOT NULL OR no_incident IS NOT NULL)
ORDER BY f.created_at, f.id;
