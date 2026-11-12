-- name: CreateThread :one
INSERT INTO threads (dashboard_id, dashboard_version, panel_id, annotation_id, observation_id, proposal_id)
VALUES (
  sqlc.narg('dashboard_id')::uuid, sqlc.narg('dashboard_version')::integer, sqlc.narg('panel_id')::text,
  sqlc.narg('annotation_id')::uuid, sqlc.narg('observation_id')::uuid, sqlc.narg('proposal_id')::uuid
)
RETURNING *;

-- name: CreateMessage :one
WITH clock AS (SELECT coalesce(sqlc.narg('created_at')::timestamptz, clock_timestamp()) AS at)
INSERT INTO thread_messages (
  id, thread_id, body, author_kind, author_id, author_name, author_email, origin, origin_client, created_at, updated_at
)
SELECT
  coalesce(sqlc.narg('id')::uuid, gen_random_uuid()), @thread_id::uuid, @body::text, @author_kind::text, @author_id::uuid,
  @author_name::text, sqlc.narg('author_email')::text, sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, clock.at, clock.at
FROM clock
RETURNING *;

-- name: GetThread :one
SELECT * FROM threads WHERE id = @id;

-- name: GetThreadForShare :one
SELECT * FROM threads WHERE id = @id FOR SHARE;

-- The first message of a thread has the thread's id, so a thread id finds its thread as well.
-- name: GetThreadOfMessage :one
SELECT t.* FROM threads t
JOIN thread_messages m ON m.thread_id = t.id
WHERE m.id = @id;

-- name: LockDashboardKeyShare :one
SELECT id FROM dashboards WHERE id = @id FOR KEY SHARE;

-- name: LockDashboardKeyShareSkipLocked :one
SELECT id FROM dashboards WHERE id = @id FOR KEY SHARE SKIP LOCKED;

-- name: ObservationOfFinding :one
SELECT observation_id FROM findings WHERE id = @id;

-- name: LockObservationKeyShare :one
SELECT id FROM observations WHERE id = @id FOR KEY SHARE;

-- name: LockProposalKeyShare :one
SELECT id FROM proposals WHERE id = @id FOR KEY SHARE;

-- name: LockAnnotationKeyShare :one
SELECT id FROM annotations WHERE id = @id FOR KEY SHARE;

-- name: GetMessage :one
SELECT * FROM thread_messages WHERE id = @id;

-- name: EditMessage :one
UPDATE thread_messages
SET body = @body::text, edited_at = clock_timestamp(), updated_at = clock_timestamp()
WHERE id = @id
RETURNING *;

-- name: SetThreadStatus :one
UPDATE threads
SET status = @status::text,
  resolved_at = CASE WHEN @status::text = 'resolved' THEN now() END,
  resolved_by_kind = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_by_kind')::text END,
  resolved_by_id = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_by_id')::uuid END,
  resolved_by_name = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_by_name')::text END,
  resolved_origin = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_origin')::text END,
  resolved_origin_client = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_origin_client')::text END,
  resolved_run_id = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_run_id')::uuid END,
  resolved_by_version = CASE WHEN @status::text = 'resolved' THEN sqlc.narg('resolved_by_version')::integer END,
  updated_at = clock_timestamp()
WHERE id = @id AND status <> @status::text
RETURNING *;

-- name: ResolveThreadForRun :one
UPDATE threads
SET status = 'resolved',
  resolved_at = now(),
  resolved_by_kind = @resolved_by_kind::text,
  resolved_by_id = @resolved_by_id::uuid,
  resolved_by_name = @resolved_by_name::text,
  resolved_origin = sqlc.narg('resolved_origin')::text,
  resolved_origin_client = sqlc.narg('resolved_origin_client')::text,
  resolved_run_id = @resolved_run_id::uuid,
  resolved_by_version = @resolved_by_version::integer,
  updated_at = clock_timestamp()
WHERE id = @id AND dashboard_id = @dashboard_id AND status = 'open'
RETURNING *;

-- name: ListThreadMessages :many
SELECT sqlc.embed(m), sqlc.embed(t)
FROM thread_messages m
JOIN threads t ON t.id = m.thread_id
WHERE (t.dashboard_id = sqlc.narg('dashboard_id')::uuid
    OR t.annotation_id = sqlc.narg('annotation_id')::uuid
    OR t.observation_id = sqlc.narg('observation_id')::uuid
    OR t.proposal_id = sqlc.narg('proposal_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status')::text)
  AND (sqlc.narg('panel_id')::text IS NULL OR t.panel_id = sqlc.narg('panel_id')::text)
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (m.created_at, m.id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY m.created_at, m.id
LIMIT @row_limit;

-- name: DeleteMentions :exec
DELETE FROM thread_mentions WHERE message_id = @message_id;

-- name: InsertMention :exec
INSERT INTO thread_mentions (message_id, user_id, agent, name)
VALUES (@message_id::uuid, sqlc.narg('user_id')::uuid, sqlc.narg('agent')::text, @name::text);

-- name: ListMentions :many
SELECT * FROM thread_mentions WHERE message_id = ANY(@message_ids::uuid[]) ORDER BY message_id, agent NULLS LAST, name, user_id;

-- name: UserMentionNames :many
SELECT id, name, email FROM users WHERE id = ANY(@ids::uuid[]);
