-- The horizon and the scan must share this statement's snapshot; never move the horizon into a function or another query.
-- name: ListInbox :many
SELECT sqlc.embed(a),
       coalesce(n.reason, '')::text AS reason,
       (ic.user_id IS NULL OR (a.xid, a.seq) > (ic.read_xid, ic.read_seq))::boolean AS unread
FROM activity a
CROSS JOIN LATERAL (SELECT inbox_reason(a, @reader_id::uuid, @editable::uuid[], @repair_prefix::text) AS reason OFFSET 0) n
LEFT JOIN inbox_cursors ic ON ic.user_id = @reader_id::uuid
WHERE a.xid < pg_snapshot_xmin(pg_current_snapshot())
  AND a.created_at >= now() - interval '30 days'
  AND (a.folder_id = ANY(@folders::uuid[])
    OR (a.folder_id IS NULL AND (@admin::boolean OR (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid))))
  AND (@everything::boolean OR a.type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded'))
  AND (CASE WHEN @everything::boolean
         THEN n.reason IS NOT NULL OR NOT (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid)
         ELSE n.reason = ANY(@reasons::text[])
       END)
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
  WHERE a.xid < pg_snapshot_xmin(pg_current_snapshot())
    AND a.created_at >= now() - interval '30 days'
    AND (a.folder_id = ANY(@folders::uuid[])
      OR (a.folder_id IS NULL AND (@admin::boolean OR (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid))))
    AND a.type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded')
    AND n.reason IS NOT NULL
    AND (ic.user_id IS NULL OR (a.xid, a.seq) > (ic.read_xid, ic.read_seq))
  LIMIT 101
) unread;

-- name: MarkInboxRead :exec
INSERT INTO inbox_cursors (user_id, read_xid, read_seq)
VALUES (@user_id::uuid, @read_xid::xid8, @read_seq::bigint)
ON CONFLICT (user_id) DO UPDATE
SET read_xid = EXCLUDED.read_xid, read_seq = EXCLUDED.read_seq, updated_at = now()
WHERE (inbox_cursors.read_xid, inbox_cursors.read_seq) < (EXCLUDED.read_xid, EXCLUDED.read_seq);
