-- +goose Up
ALTER TABLE agent_requests ADD COLUMN thread_replied_at TIMESTAMPTZ;

UPDATE agent_requests SET thread_replied_at = coalesce(finished_at, now())
WHERE thread_message_id IS NOT NULL AND state IN ('succeeded', 'failed', 'canceled');

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN thread_replied_at;
