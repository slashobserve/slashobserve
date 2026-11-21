-- name: CreateChat :one
INSERT INTO chats (id, owner_kind, owner_id, title, dashboard_id, observation_id)
VALUES (@id, @owner_kind, @owner_id, sqlc.narg('title')::text, sqlc.narg('dashboard_id')::uuid, sqlc.narg('observation_id')::uuid)
RETURNING *;

-- name: GetChat :one
SELECT c.*, coalesce(owner.requested_by_name, '')::text AS owner_name, d.title AS dashboard_title
FROM chats c
LEFT JOIN LATERAL (
  SELECT t.requested_by_name FROM agent_requests t WHERE t.chat_id = c.id ORDER BY t.created_at, t.id LIMIT 1
) owner ON true
LEFT JOIN dashboards d ON d.id = c.dashboard_id AND d.folder_id = ANY(sqlc.arg('folders')::uuid[])
WHERE c.id = @id
  AND (sqlc.narg('owner_id')::uuid IS NULL OR (c.owner_kind = sqlc.narg('owner_kind')::text AND c.owner_id = sqlc.narg('owner_id')::uuid));

-- name: TouchChat :one
UPDATE chats SET last_turn_at = clock_timestamp()
WHERE id = @id AND owner_kind = @owner_kind AND owner_id = @owner_id AND archived_at IS NULL
RETURNING *;

-- name: TouchLatestDashboardChat :one
UPDATE chats SET last_turn_at = clock_timestamp()
WHERE id = (
  SELECT l.id FROM chats l
  WHERE l.owner_kind = @owner_kind AND l.owner_id = @owner_id AND l.dashboard_id = @dashboard_id AND l.archived_at IS NULL
  ORDER BY l.last_turn_at DESC, l.id DESC
  LIMIT 1
  FOR UPDATE
)
RETURNING *;

-- name: ArchiveChat :one
WITH archived AS (
  UPDATE chats SET archived_at = coalesce(archived_at, clock_timestamp())
  WHERE chats.id = @id AND owner_kind = @owner_kind AND owner_id = @owner_id
  RETURNING *
), turns AS (
  UPDATE agent_requests SET archived_at = coalesce(agent_requests.archived_at, clock_timestamp())
  FROM archived WHERE agent_requests.chat_id = archived.id
)
SELECT * FROM archived;

-- name: SetChatSubjectDashboard :exec
UPDATE chats SET dashboard_id = @dashboard_id
WHERE id = @id AND dashboard_id IS NULL AND observation_id IS NULL;

-- name: ListChats :many
SELECT c.*, turns.count AS turns, coalesce(first.prompt, '')::text AS first_prompt, coalesce(first.requested_by_name, '')::text AS owner_name,
  last.id AS last_turn_id, coalesce(last.state, '')::text AS last_turn_state, coalesce(last.prompt, '')::text AS last_turn_prompt,
  coalesce(last.investigation, false)::boolean AS last_turn_investigation, d.title AS dashboard_title
FROM chats c
LEFT JOIN dashboards d ON d.id = c.dashboard_id AND d.folder_id = ANY(sqlc.arg('folders')::uuid[])
CROSS JOIN LATERAL (SELECT count(*) AS count FROM agent_requests t WHERE t.chat_id = c.id) turns
LEFT JOIN LATERAL (
  SELECT t.prompt, t.requested_by_name FROM agent_requests t WHERE t.chat_id = c.id ORDER BY t.created_at, t.id LIMIT 1
) first ON true
LEFT JOIN LATERAL (
  SELECT t.id, t.state, t.prompt, t.investigation FROM agent_requests t WHERE t.chat_id = c.id ORDER BY t.created_at DESC, t.id DESC LIMIT 1
) last ON true
WHERE (sqlc.narg('owner_id')::uuid IS NULL OR (c.owner_kind = sqlc.narg('owner_kind')::text AND c.owner_id = sqlc.narg('owner_id')::uuid))
  AND c.archived_at IS NULL
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR c.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('observation_id')::uuid IS NULL OR c.observation_id = sqlc.narg('observation_id')::uuid)
  AND (sqlc.narg('pinned')::boolean IS NULL OR (c.pinned_at IS NOT NULL) = sqlc.narg('pinned')::boolean)
  AND (sqlc.narg('pattern')::text IS NULL OR c.title ILIKE sqlc.narg('pattern')::text OR EXISTS (
    SELECT 1 FROM agent_requests t WHERE t.chat_id = c.id AND t.prompt ILIKE sqlc.narg('pattern')::text))
  AND (sqlc.narg('cursor_at')::timestamptz IS NULL
    OR ((@pinned_first::boolean AND c.pinned_at IS NOT NULL) = @cursor_pinned::boolean
      AND (c.last_turn_at, c.id) < (sqlc.narg('cursor_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
    OR (@cursor_pinned::boolean AND NOT (@pinned_first::boolean AND c.pinned_at IS NOT NULL)))
ORDER BY (@pinned_first::boolean AND c.pinned_at IS NOT NULL) DESC, c.last_turn_at DESC, c.id DESC
LIMIT @row_limit;

-- name: ListChatRequests :many
SELECT * FROM agent_requests WHERE chat_id = @chat_id ORDER BY created_at, id;

-- name: DeleteChatsEmptiedByDashboardDelete :execrows
DELETE FROM chats c
WHERE c.dashboard_id = @dashboard_id
  AND NOT EXISTS (SELECT 1 FROM agent_requests r WHERE r.chat_id = c.id AND r.dashboard_id IS DISTINCT FROM @dashboard_id);
