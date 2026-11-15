-- +goose Up
ALTER TABLE thread_messages
  ADD COLUMN reply_to_id UUID REFERENCES thread_messages (id) ON DELETE SET NULL,
  ADD COLUMN observation_block_id UUID REFERENCES observation_blocks (id) ON DELETE SET NULL;
CREATE INDEX thread_messages_observation_block_idx ON thread_messages (observation_block_id) WHERE observation_block_id IS NOT NULL;
CREATE INDEX thread_messages_reply_to_idx ON thread_messages (reply_to_id) WHERE reply_to_id IS NOT NULL;

-- +goose Down
DROP INDEX thread_messages_reply_to_idx;
DROP INDEX thread_messages_observation_block_idx;
ALTER TABLE thread_messages DROP COLUMN observation_block_id, DROP COLUMN reply_to_id;
