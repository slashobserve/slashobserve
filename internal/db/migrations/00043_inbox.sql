-- +goose Up
CREATE TABLE inbox_cursors (
  user_id UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  read_xid XID8 NOT NULL,
  read_seq BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX activity_inbox_idx ON activity (xid, seq)
  WHERE type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed');

-- +goose StatementBegin
CREATE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[]) RETURNS text
LANGUAGE sql STABLE
AS $$
  SELECT CASE
    WHEN a.type = 'proposal.opened' AND a.folder_id = ANY(editable) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.state = 'open' AND NOT (p.author_kind = 'user' AND p.author_id = reader)
    ) THEN 'proposal'
    WHEN a.type = 'observation.block_added' AND a.data->>'state' = 'requested'
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM observations o
      JOIN observation_blocks b ON b.observation_id = o.id
      WHERE o.id = a.target_id AND b.id = (a.data->>'block_id')::uuid AND b.state = 'requested'
        AND o.created_by_kind = 'user' AND o.created_by_id = reader AND o.folder_id = ANY(editable)
    ) THEN 'requested_block'
    WHEN a.type = 'thread.mentioned' AND NOT (a.actor_kind = 'user' AND a.actor_id = reader)
      AND a.data->'mentioned'->>'kind' = 'user' AND a.data->'mentioned'->>'id' = reader::text
    THEN 'mention'
    WHEN a.type = 'thread.replied' AND (
      (NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
        SELECT 1 FROM thread_messages reply
        JOIN thread_messages earlier ON earlier.thread_id = reply.thread_id AND earlier.created_at < reply.created_at
        WHERE reply.id = (a.data->>'message_id')::uuid AND reply.thread_id = a.target_id
          AND earlier.author_kind = 'user' AND earlier.author_id = reader
      ))
      OR (a.run_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM agent_requests r
        WHERE r.id = a.run_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
      ))
    ) THEN 'reply'
    WHEN a.type = 'run.failed' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader AND r.state = 'failed'
        AND NOT EXISTS (SELECT 1 FROM agent_requests c WHERE c.continues_id = r.id)
    ) THEN 'run_failed'
  END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION inbox_reason(activity, uuid, uuid[]);
DROP INDEX activity_inbox_idx;
DROP TABLE inbox_cursors;
