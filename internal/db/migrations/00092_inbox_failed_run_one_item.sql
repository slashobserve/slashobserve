-- +goose Up
ALTER TABLE thread_messages ADD COLUMN run_failed BOOLEAN NOT NULL DEFAULT false;

UPDATE thread_messages m SET run_failed = true
FROM agent_requests r
WHERE m.run_id = r.id AND r.state = 'failed' AND starts_with(m.body, 'The run failed: ');

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION inbox_superseded(a activity, reader uuid) RETURNS boolean
LANGUAGE sql STABLE
AS $$
  SELECT a.type = 'thread.replied' AND (
    EXISTS (
      SELECT 1 FROM activity m
      WHERE m.xid = a.xid AND m.type = 'thread.mentioned' AND m.target_id = a.target_id
        AND m.data->>'message_id' = a.data->>'message_id'
        AND m.data->'mentioned'->>'kind' = 'user' AND m.data->'mentioned'->>'id' = reader::text
    )
    OR EXISTS (
      SELECT 1 FROM thread_messages m
      JOIN agent_requests r ON r.id = m.run_id
      WHERE m.id = (a.data->>'message_id')::uuid AND m.run_failed
        AND r.state = 'failed' AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
        AND NOT EXISTS (SELECT 1 FROM agent_requests c WHERE c.continues_id = r.id)
    )
  )
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION inbox_superseded(a activity, reader uuid) RETURNS boolean
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

ALTER TABLE thread_messages DROP COLUMN run_failed;
