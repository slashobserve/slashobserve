-- +goose Up
ALTER TABLE agent_requests ADD COLUMN retries_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL;
CREATE UNIQUE INDEX agent_requests_successor_key ON agent_requests ((coalesce(retries_id, continues_id))) WHERE retries_id IS NOT NULL OR continues_id IS NOT NULL;

-- +goose Down
DROP INDEX agent_requests_successor_key;
ALTER TABLE agent_requests DROP COLUMN retries_id;
