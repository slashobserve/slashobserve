-- +goose NO TRANSACTION
-- +goose Up
UPDATE threads SET agent_off = false WHERE agent_off AND agent_kind IS NULL;

ALTER TABLE threads
  DROP CONSTRAINT IF EXISTS threads_agent_off_check,
  ADD CONSTRAINT threads_agent_off_check CHECK (NOT agent_off OR agent_kind IS NOT NULL) NOT VALID;

ALTER TABLE threads VALIDATE CONSTRAINT threads_agent_off_check;

-- +goose Down
ALTER TABLE threads DROP CONSTRAINT IF EXISTS threads_agent_off_check;
