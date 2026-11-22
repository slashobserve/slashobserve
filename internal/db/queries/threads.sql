-- name: CreateThread :one
INSERT INTO threads (dashboard_id, dashboard_version, panel_id, annotation_id, observation_id, proposal_id, anchor, block_id)
VALUES (
  sqlc.narg('dashboard_id')::uuid, sqlc.narg('dashboard_version')::integer, sqlc.narg('panel_id')::text,
  sqlc.narg('annotation_id')::uuid, sqlc.narg('observation_id')::uuid, sqlc.narg('proposal_id')::uuid,
  sqlc.narg('anchor')::jsonb, sqlc.narg('block_id')::uuid
)
RETURNING *;

-- name: CreateMessage :one
WITH clock AS (SELECT coalesce(sqlc.narg('created_at')::timestamptz, clock_timestamp()) AS at)
INSERT INTO thread_messages (
  id, thread_id, body, author_kind, author_id, author_name, author_email, origin, origin_client, created_at, updated_at,
  reply_to_id, observation_block_id, client_id
)
SELECT
  coalesce(sqlc.narg('id')::uuid, gen_random_uuid()), @thread_id::uuid, @body::text, @author_kind::text, @author_id::uuid,
  @author_name::text, sqlc.narg('author_email')::text, sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, clock.at, clock.at,
  sqlc.narg('reply_to_id')::uuid, sqlc.narg('observation_block_id')::uuid, sqlc.narg('client_id')::uuid
FROM clock
ON CONFLICT (author_kind, author_id, client_id) WHERE client_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetMessageByClient :one
SELECT * FROM thread_messages WHERE author_kind = @author_kind AND author_id = @author_id AND client_id = @client_id;

-- name: GetThread :one
SELECT * FROM threads WHERE id = @id;

-- name: LockThreadForWrite :one
SELECT * FROM threads WHERE id = @id FOR NO KEY UPDATE;

-- name: SetThreadAgent :one
UPDATE threads SET agent_kind = sqlc.narg('agent_kind')::text, agent_id = sqlc.narg('agent_id')::uuid, agent_off = @agent_off
WHERE id = @id
RETURNING *;

-- The first message of a thread has the thread's id, so a thread id finds its thread as well.
-- name: GetThreadOfMessage :one
SELECT t.* FROM threads t
JOIN thread_messages m ON m.thread_id = t.id
WHERE m.id = @id;

-- name: LockDashboardKeyShare :one
SELECT id FROM dashboards WHERE id = @id FOR KEY SHARE;

-- name: LockDashboardKeyShareSkipLocked :one
SELECT folder_id FROM dashboards WHERE id = @id FOR KEY SHARE SKIP LOCKED;

-- name: LockObservationKeyShare :one
SELECT id FROM observations WHERE id = @id FOR KEY SHARE;

-- name: LockObservationKeyShareSkipLocked :one
SELECT folder_id FROM observations WHERE id = @id FOR KEY SHARE SKIP LOCKED;

-- name: LockProposalKeyShare :one
SELECT id FROM proposals WHERE id = @id FOR KEY SHARE;

-- name: LockAnnotationKeyShare :one
SELECT id FROM annotations WHERE id = @id FOR KEY SHARE;

-- name: InsertRunReply :one
INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, origin, origin_client, run_id, run_assigned_kind)
SELECT @thread_id::uuid, @body::text, @author_kind::text, @author_id::uuid, @author_name::text,
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, @run_id::uuid, @run_assigned_kind::text
WHERE NOT EXISTS (SELECT 1 FROM thread_messages WHERE run_id = @run_id::uuid AND thread_id = @thread_id::uuid)
  AND EXISTS (SELECT 1 FROM agent_requests WHERE id = @run_id::uuid AND thread_replied_at IS NULL)
RETURNING *;

-- name: ShareRunInThread :execrows
UPDATE agent_requests SET thread_message_id = @thread_message_id, thread_replied_at = NULL WHERE id = @id AND thread_message_id IS NULL;

-- name: LockSharedRunsInThread :many
SELECT r.id, r.chat_id, r.thread_message_id, f.id AS finding_id FROM agent_requests r
JOIN thread_messages m ON m.id = r.thread_message_id
LEFT JOIN findings f ON f.agent_request_id = r.id
WHERE m.thread_id = @thread_id AND r.chat_id IS NOT NULL
  AND r.requested_by_kind = @requested_by_kind AND r.requested_by_id = @requested_by_id
ORDER BY r.id
FOR UPDATE OF r;

-- name: WithdrawRunShares :exec
UPDATE agent_requests SET thread_message_id = NULL, thread_replied_at = NULL WHERE id = ANY(@ids::uuid[]);

-- name: DeleteSharedMessages :many
DELETE FROM thread_messages
WHERE thread_id = @thread_id AND (id = ANY(@message_ids::uuid[]) OR run_id = ANY(@run_ids::uuid[]))
RETURNING id;

-- name: MarkRunReplied :exec
UPDATE agent_requests SET thread_replied_at = clock_timestamp() WHERE id = @id AND thread_replied_at IS NULL;

-- name: GetRunProposalTitle :one
SELECT title FROM proposals WHERE run_id = @run_id ORDER BY created_at DESC, id LIMIT 1;

-- name: ListMessagesByID :many
SELECT * FROM thread_messages WHERE id = ANY(@ids::uuid[]);

-- name: GetMessage :one
SELECT * FROM thread_messages WHERE id = @id;

-- name: LockObservationBlock :one
SELECT id FROM observation_blocks WHERE id = @id AND observation_id = @observation_id FOR KEY SHARE;

-- name: LockMessageInThread :one
SELECT id FROM thread_messages WHERE id = @id AND thread_id = @thread_id FOR KEY SHARE;

-- name: ListReplyQuotes :many
SELECT m.id AS message_id, q.id AS quoted_id, q.author_kind, q.author_id, q.author_name, q.origin, q.origin_client,
  q.run_id, q.run_assigned_kind, left(q.body, 200)::text AS excerpt
FROM thread_messages m
JOIN thread_messages q ON q.id = m.reply_to_id
WHERE m.id = ANY(@message_ids::uuid[]);

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
INSERT INTO thread_mentions (message_id, user_id, agent, agent_grant_id, agent_service_account_id, name)
VALUES (@message_id::uuid, sqlc.narg('user_id')::uuid, sqlc.narg('agent')::text, sqlc.narg('agent_grant_id')::uuid, sqlc.narg('agent_service_account_id')::uuid, @name::text);

-- name: ListMentions :many
SELECT * FROM thread_mentions WHERE message_id = ANY(@message_ids::uuid[]) ORDER BY message_id, agent NULLS LAST, name, user_id, agent_grant_id, agent_service_account_id;

-- name: UserMentionNames :many
SELECT id, name, email, role, groups FROM users WHERE id = ANY(@ids::uuid[]);

-- name: MentionCandidates :many
SELECT id, name, email, role, groups, lower(coalesce(nullif(name, ''), email))::text AS sort_key FROM users
WHERE id <> @self::uuid
  AND (name ILIKE @pattern::text ESCAPE '\' OR email ILIKE @pattern::text ESCAPE '\')
  AND (lower(coalesce(nullif(name, ''), email)), id) > (@after_key::text, @after_id::uuid)
ORDER BY lower(coalesce(nullif(name, ''), email)), id
LIMIT @row_limit;

-- name: MentionableGrant :one
SELECT coalesce(c.name, '')::text AS name
FROM oauth_grants g
JOIN oauth_clients c ON c.id = g.client_id
WHERE g.id = @id AND g.user_id = @user_id AND g.revoked_at IS NULL AND g.expires_at > now();

-- name: MentionableServiceAccount :one
SELECT name FROM service_accounts WHERE id = @id AND takes_runs;

-- name: ThreadTranscript :many
SELECT m.*
FROM thread_messages m
WHERE m.thread_id = @thread_id
  AND (m.created_at, m.id) < (SELECT b.created_at, b.id FROM thread_messages b WHERE b.id = @before_id)
ORDER BY m.created_at DESC, m.id DESC
LIMIT @row_limit;

-- name: CountObservationMessages :many
SELECT t.observation_id, count(m.id)::int AS messages
FROM threads t
JOIN thread_messages m ON m.thread_id = t.id
WHERE t.observation_id = ANY(@observation_ids::uuid[])
GROUP BY t.observation_id;

-- name: DeleteMessage :exec
DELETE FROM thread_messages WHERE id = @id;

-- name: DeleteThread :exec
DELETE FROM threads WHERE id = @id;

-- name: ThreadsWithOtherAuthors :many
SELECT DISTINCT thread_id FROM thread_messages
WHERE thread_id = ANY(@thread_ids::uuid[])
  AND NOT (author_kind = @author_kind::text AND author_id = @author_id::uuid);

-- name: RunsOfMessages :many
SELECT DISTINCT ON (thread_message_id) thread_message_id, id, status, assigned_kind
FROM agent_requests
WHERE thread_message_id = ANY(@message_ids::uuid[])
ORDER BY thread_message_id, created_at DESC, id DESC;

-- name: MarkThreadRead :exec
INSERT INTO thread_reads (user_id, thread_id, read_at)
SELECT u.id, @thread_id::uuid, clock_timestamp() FROM users u WHERE u.id = @user_id::uuid
ON CONFLICT (user_id, thread_id) DO UPDATE SET read_at = EXCLUDED.read_at;

-- name: UnreadThreads :many
SELECT t.id
FROM threads t
WHERE t.id = ANY(@thread_ids::uuid[])
  AND EXISTS (
    SELECT 1 FROM thread_messages m
    WHERE m.thread_id = t.id
      AND NOT (m.run_id IS NULL AND m.author_kind = @reader_kind::text AND m.author_id = @reader_id::uuid)
      AND m.created_at > greatest(
        coalesce((SELECT r.read_at FROM thread_reads r WHERE r.user_id = @reader_id::uuid AND r.thread_id = t.id), '-infinity'::timestamptz),
        (SELECT u.threads_seen_since FROM users u WHERE u.id = @reader_id::uuid)
      )
  );

-- name: AgentGrantIsLive :one
SELECT EXISTS (SELECT 1 FROM oauth_grants WHERE id = @id AND revoked_at IS NULL AND expires_at > now())::boolean AS live;

-- name: ListAnsweredRuns :many
SELECT agent_request_id::uuid FROM findings WHERE agent_request_id = ANY(@run_ids::uuid[]);
