-- +goose NO TRANSACTION
-- +goose Up
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_awaiting_reply_idx;
CREATE INDEX CONCURRENTLY IF NOT EXISTS agent_requests_awaiting_reply_idx ON agent_requests (finished_at DESC, id)
  WHERE thread_message_id IS NOT NULL AND thread_replied_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_retries_key;
CREATE INDEX CONCURRENTLY IF NOT EXISTS agent_requests_retries_key ON agent_requests (retries_id)
  WHERE retries_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_retries_key;
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_awaiting_reply_idx;
