-- The horizon and the scan must share this statement's snapshot; never move the horizon into a function or another query.
-- name: ListInbox :many
SELECT sqlc.embed(a),
       coalesce(n.reason, '')::text AS reason,
       coalesce(st.unread, ic.user_id IS NULL OR (a.xid, a.seq) > (ic.read_xid, ic.read_seq))::boolean AS unread,
       coalesce(st.done, false)::boolean AS done
FROM activity a
CROSS JOIN LATERAL (SELECT inbox_reason(a, @reader_id::uuid, @editable::uuid[], @repair_prefix::text) AS reason OFFSET 0) n
LEFT JOIN inbox_cursors ic ON ic.user_id = @reader_id::uuid
LEFT JOIN inbox_item_states st ON st.user_id = @reader_id::uuid AND st.seq = a.seq
WHERE NOT inbox_superseded(a, @reader_id::uuid)
  AND (a.type NOT IN ('thread.mentioned', 'thread.replied') OR a.data->>'message_id' IS NULL
    OR EXISTS (SELECT 1 FROM thread_messages m WHERE m.id = (a.data->>'message_id')::uuid))
  AND a.xid < pg_snapshot_xmin(pg_current_snapshot())
  AND a.created_at >= now() - interval '30 days'
  AND (a.folder_id = ANY(@folders::uuid[])
    OR (a.folder_id IS NULL AND (@admin::boolean OR (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid))))
  AND (@everything::boolean OR a.type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded', 'proposal.merged', 'proposal.discarded', 'access_request.opened'))
  AND (CASE WHEN @everything::boolean
         THEN n.reason IS NOT NULL OR NOT (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid)
         ELSE n.reason = ANY(@reasons::text[])
       END)
  AND coalesce(st.done, false) = @done::boolean
  AND (NOT @unread_only::boolean OR coalesce(st.unread, ic.user_id IS NULL OR (a.xid, a.seq) > (ic.read_xid, ic.read_seq)))
  AND (NOT @has_cursor::boolean OR (a.xid, a.seq) < (@cursor_xid::xid8, @cursor_seq::bigint))
ORDER BY a.xid DESC, a.seq DESC
LIMIT @row_limit;

-- The horizon and the scan must share this statement's snapshot; never move the horizon into a function or another query.
-- name: InboxUnreadCount :one
SELECT count(*)::int
FROM (
  SELECT 1
  FROM activity a
  CROSS JOIN LATERAL (SELECT inbox_reason(a, @reader_id::uuid, @editable::uuid[], @repair_prefix::text) AS reason) n
  LEFT JOIN inbox_cursors ic ON ic.user_id = @reader_id::uuid
  LEFT JOIN inbox_item_states st ON st.user_id = @reader_id::uuid AND st.seq = a.seq
  WHERE NOT inbox_superseded(a, @reader_id::uuid)
    AND (a.type NOT IN ('thread.mentioned', 'thread.replied') OR a.data->>'message_id' IS NULL
      OR EXISTS (SELECT 1 FROM thread_messages m WHERE m.id = (a.data->>'message_id')::uuid))
    AND a.xid < pg_snapshot_xmin(pg_current_snapshot())
    AND a.created_at >= now() - interval '30 days'
    AND (a.folder_id = ANY(@folders::uuid[])
      OR (a.folder_id IS NULL AND (@admin::boolean OR (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid))))
    AND a.type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded', 'proposal.merged', 'proposal.discarded', 'access_request.opened')
    AND n.reason IS NOT NULL
    AND NOT coalesce(st.done, false)
    AND coalesce(st.unread, ic.user_id IS NULL OR (a.xid, a.seq) > (ic.read_xid, ic.read_seq))
  LIMIT 101
) unread;

-- name: MarkInboxRead :exec
WITH moved AS (
  INSERT INTO inbox_cursors (user_id, read_xid, read_seq)
  VALUES (@user_id::uuid, @read_xid::xid8, @read_seq::bigint)
  ON CONFLICT (user_id) DO UPDATE
  SET read_xid = EXCLUDED.read_xid, read_seq = EXCLUDED.read_seq, updated_at = now()
  WHERE (inbox_cursors.read_xid, inbox_cursors.read_seq) < (EXCLUDED.read_xid, EXCLUDED.read_seq)
  RETURNING read_xid, read_seq
)
DELETE FROM inbox_item_states s
USING activity a, moved
WHERE s.user_id = @user_id::uuid AND NOT s.done AND a.seq = s.seq AND (a.xid, a.seq) <= (moved.read_xid, moved.read_seq);

-- name: SetInboxItemUnread :exec
INSERT INTO inbox_item_states (user_id, seq, unread)
VALUES (@user_id::uuid, @seq::bigint, @unread::boolean)
ON CONFLICT (user_id, seq) DO UPDATE
SET unread = EXCLUDED.unread, done = inbox_item_states.done AND NOT EXCLUDED.unread, updated_at = now();

-- name: SetInboxItemDone :exec
INSERT INTO inbox_item_states (user_id, seq, unread, done)
VALUES (@user_id::uuid, @seq::bigint, false, true)
ON CONFLICT (user_id, seq) DO UPDATE SET unread = false, done = true, updated_at = now();

-- name: ReopenInboxItem :exec
UPDATE inbox_item_states SET done = false, updated_at = now()
WHERE user_id = @user_id::uuid AND seq = @seq::bigint;

-- name: InboxObjectTitles :many
SELECT 'dashboard'::text AS kind, d.id AS id, d.title AS title FROM dashboards d
WHERE d.id = ANY(@dashboard_ids::uuid[]) AND (@admin::boolean OR d.folder_id = ANY(@folders::uuid[]))
UNION ALL
SELECT 'observation', o.id, o.title FROM observations o
WHERE o.id = ANY(@observation_ids::uuid[]) AND (@admin::boolean OR o.folder_id = ANY(@folders::uuid[]))
UNION ALL
SELECT 'proposal', p.id, p.title FROM proposals p JOIN dashboards d ON d.id = p.dashboard_id
WHERE p.id = ANY(@proposal_ids::uuid[]) AND (@admin::boolean OR d.folder_id = ANY(@folders::uuid[]));

-- name: InboxThreadTexts :many
SELECT m.id, m.body, q.body AS question
FROM thread_messages m
JOIN threads t ON t.id = m.thread_id
LEFT JOIN agent_requests r ON r.id = m.run_id
LEFT JOIN thread_messages q ON q.id = r.thread_message_id AND q.thread_id = m.thread_id
WHERE m.id = ANY(@ids::uuid[])
  AND (@admin::boolean OR coalesce(
    (SELECT d.folder_id FROM dashboards d WHERE d.id = t.dashboard_id),
    (SELECT o.folder_id FROM observations o WHERE o.id = t.observation_id),
    (SELECT d.folder_id FROM proposals p JOIN dashboards d ON d.id = p.dashboard_id WHERE p.id = t.proposal_id),
    (SELECT coalesce(d.folder_id, (SELECT f.id FROM folders f WHERE f.general)) FROM annotations a LEFT JOIN dashboards d ON d.id = a.dashboard_id WHERE a.id = t.annotation_id)
  ) = ANY(@folders::uuid[]));

-- name: InboxRunTexts :many
SELECT r.id, coalesce(r.error, '')::text AS error, r.prompt, r.dashboard_id, r.subject_observation_id, r.observation_id, r.thread_message_id, r.trigger FROM agent_requests r WHERE r.id = ANY(@ids::uuid[]);

-- name: InboxProposalCloseReasons :many
SELECT p.id, p.close_reason::text AS close_reason FROM proposals p WHERE p.id = ANY(@ids::uuid[]) AND p.close_reason IS NOT NULL;

-- name: InboxAccessRequestNotes :many
SELECT r.id, r.note FROM access_requests r WHERE r.id = ANY(@ids::uuid[]);
