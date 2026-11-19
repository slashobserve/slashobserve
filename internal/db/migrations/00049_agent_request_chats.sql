-- +goose Up
ALTER TABLE agent_requests ADD COLUMN chat_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL;
CREATE INDEX agent_requests_chat_idx ON agent_requests (chat_id) WHERE chat_id IS NOT NULL;

-- +goose Down
DROP INDEX agent_requests_chat_idx;
ALTER TABLE agent_requests DROP COLUMN chat_id;
