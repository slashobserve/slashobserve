-- +goose Up
ALTER TABLE findings ADD COLUMN kept_at timestamptz;

UPDATE findings f SET kept_at = f.created_at
WHERE f.observation_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM agent_requests r WHERE r.id = f.agent_request_id AND r.chat_id IS NOT NULL AND r.thread_message_id IS NOT NULL);

ALTER TABLE mcp_sources ADD COLUMN discoveries bigint DEFAULT 0 NOT NULL;

-- +goose Down
ALTER TABLE mcp_sources DROP COLUMN discoveries;
ALTER TABLE findings DROP COLUMN kept_at;
