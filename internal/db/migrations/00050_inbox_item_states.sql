-- +goose Up
CREATE TABLE inbox_item_states (
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  seq BIGINT NOT NULL REFERENCES activity (seq) ON DELETE CASCADE,
  unread BOOLEAN NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, seq)
);

-- +goose StatementBegin
CREATE FUNCTION inbox_superseded(a activity, reader uuid) RETURNS boolean
LANGUAGE sql STABLE
AS $$
  SELECT a.type = 'thread.replied' AND EXISTS (
    SELECT 1 FROM activity m
    WHERE m.xid = a.xid AND m.type = 'thread.mentioned' AND m.target_id = a.target_id
      AND m.data->>'message_id' = a.data->>'message_id'
      AND m.data->'mentioned'->>'kind' = 'user' AND m.data->'mentioned'->>'id' = reader::text
  )
$$;
-- +goose StatementEnd

DROP INDEX activity_inbox_idx;
CREATE INDEX activity_inbox_idx ON activity (xid, seq)
  WHERE type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded', 'proposal.merged', 'proposal.discarded');

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[], repair_prefix text) RETURNS text
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
    WHEN a.type = 'run.succeeded' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
        AND r.source = 'prompt' AND starts_with(r.prompt, repair_prefix) AND r.result_version IS NOT NULL
    ) THEN 'repaired'
    WHEN a.type IN ('proposal.merged', 'proposal.discarded')
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.author_kind = 'user' AND p.author_id = reader
    ) THEN 'proposal_closed'
  END
$$;
-- +goose StatementEnd

-- +goose Down
DROP INDEX activity_inbox_idx;
CREATE INDEX activity_inbox_idx ON activity (xid, seq)
  WHERE type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded');

-- +goose StatementBegin
CREATE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[], repair_prefix text) RETURNS text
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
    WHEN a.type = 'run.succeeded' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
        AND r.source = 'prompt' AND starts_with(r.prompt, repair_prefix) AND r.result_version IS NOT NULL
    ) THEN 'repaired'
  END
$$;
-- +goose StatementEnd

DROP FUNCTION inbox_superseded(activity, uuid);
DROP TABLE inbox_item_states;
