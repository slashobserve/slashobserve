-- +goose Up
ALTER TABLE agent_requests ADD COLUMN thread_reply_abandoned_at timestamptz;
UPDATE agent_requests SET thread_reply_abandoned_at = finished_at + interval '24 hours'
WHERE thread_message_id IS NOT NULL AND thread_replied_at IS NULL
  AND finished_at < clock_timestamp() - interval '24 hours';

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN thread_reply_abandoned_at;
