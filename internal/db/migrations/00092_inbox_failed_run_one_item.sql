-- +goose Up
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
    OR (a.run_id IS NOT NULL AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.run_id AND r.state = 'failed' AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
    ))
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
