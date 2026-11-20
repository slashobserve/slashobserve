-- name: CreateChat :one
INSERT INTO chats (id, owner_kind, owner_id, title, dashboard_id, observation_id)
VALUES (@id, @owner_kind, @owner_id, sqlc.narg('title')::text, sqlc.narg('dashboard_id')::uuid, sqlc.narg('observation_id')::uuid)
RETURNING *;

-- name: GetChat :one
SELECT * FROM chats WHERE id = @id AND owner_kind = @owner_kind AND owner_id = @owner_id;

-- name: TouchChat :one
UPDATE chats SET last_turn_at = clock_timestamp()
WHERE id = @id AND owner_kind = @owner_kind AND owner_id = @owner_id AND archived_at IS NULL
RETURNING *;

-- name: ArchiveChat :one
UPDATE chats SET archived_at = coalesce(archived_at, clock_timestamp())
WHERE id = @id AND owner_kind = @owner_kind AND owner_id = @owner_id
RETURNING *;

-- name: SetChatSubjectDashboard :exec
UPDATE chats SET dashboard_id = @dashboard_id
WHERE id = @id AND dashboard_id IS NULL AND observation_id IS NULL;

-- name: ListChats :many
SELECT c.*, turns.count AS turns, last.id AS last_turn_id, last.state AS last_turn_state, last.prompt AS last_turn_prompt
FROM chats c
CROSS JOIN LATERAL (SELECT count(*) AS count FROM agent_requests t WHERE t.chat_id = c.id) turns
LEFT JOIN LATERAL (
  SELECT t.id, t.state, t.prompt FROM agent_requests t WHERE t.chat_id = c.id ORDER BY t.created_at DESC, t.id DESC LIMIT 1
) last ON true
WHERE c.owner_kind = @owner_kind AND c.owner_id = @owner_id AND c.archived_at IS NULL
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR c.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('observation_id')::uuid IS NULL OR c.observation_id = sqlc.narg('observation_id')::uuid)
  AND (sqlc.narg('pinned')::boolean IS NULL OR (c.pinned_at IS NOT NULL) = sqlc.narg('pinned')::boolean)
  AND (sqlc.narg('pattern')::text IS NULL OR c.title ILIKE sqlc.narg('pattern')::text OR EXISTS (
    SELECT 1 FROM agent_requests t WHERE t.chat_id = c.id AND t.prompt ILIKE sqlc.narg('pattern')::text))
  AND (sqlc.narg('cursor_at')::timestamptz IS NULL
    OR (c.last_turn_at, c.id) < (sqlc.narg('cursor_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY c.last_turn_at DESC, c.id DESC
LIMIT @row_limit;

-- name: ListChatRequests :many
SELECT * FROM agent_requests WHERE chat_id = @chat_id ORDER BY created_at, id;

-- name: EnsureChat :one
INSERT INTO chats AS c (id, owner_kind, owner_id) VALUES (@id, @owner_kind, @owner_id)
ON CONFLICT (id) DO UPDATE SET last_turn_at = clock_timestamp()
WHERE c.owner_kind = excluded.owner_kind AND c.owner_id = excluded.owner_id
RETURNING c.id, (c.xmax = 0) AS created;
