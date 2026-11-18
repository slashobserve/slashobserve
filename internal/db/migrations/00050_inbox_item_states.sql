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

-- +goose Down
DROP FUNCTION inbox_superseded(activity, uuid);
DROP TABLE inbox_item_states;
