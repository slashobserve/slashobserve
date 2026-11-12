-- +goose Up
ALTER TABLE thread_mentions
  ADD COLUMN agent_grant_id UUID REFERENCES oauth_grants (id) ON DELETE CASCADE,
  ADD COLUMN agent_service_account_id UUID REFERENCES service_accounts (id) ON DELETE CASCADE;
ALTER TABLE thread_mentions DROP CONSTRAINT thread_mentions_one_check;
ALTER TABLE thread_mentions ADD CONSTRAINT thread_mentions_one_check
  CHECK (num_nonnulls(user_id, agent, agent_grant_id, agent_service_account_id) = 1);
CREATE UNIQUE INDEX thread_mentions_grant_key ON thread_mentions (message_id, agent_grant_id) WHERE agent_grant_id IS NOT NULL;
CREATE UNIQUE INDEX thread_mentions_service_account_key ON thread_mentions (message_id, agent_service_account_id) WHERE agent_service_account_id IS NOT NULL;

ALTER TABLE agent_requests
  ADD COLUMN thread_message_id UUID REFERENCES thread_messages (id) ON DELETE SET NULL,
  ADD COLUMN thread_context TEXT CONSTRAINT agent_requests_thread_context_check CHECK (length(thread_context) BETWEEN 1 AND 8000);
CREATE INDEX agent_requests_thread_message_idx ON agent_requests (thread_message_id) WHERE thread_message_id IS NOT NULL;

ALTER TABLE thread_messages ADD COLUMN run_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL;
CREATE INDEX thread_messages_run_idx ON thread_messages (run_id) WHERE run_id IS NOT NULL;

-- +goose Down
DROP INDEX thread_messages_run_idx;
ALTER TABLE thread_messages DROP COLUMN run_id;

DROP INDEX agent_requests_thread_message_idx;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_thread_context_check;
ALTER TABLE agent_requests DROP COLUMN thread_context;
ALTER TABLE agent_requests DROP COLUMN thread_message_id;

DELETE FROM thread_mentions WHERE agent_grant_id IS NOT NULL OR agent_service_account_id IS NOT NULL;
DROP INDEX thread_mentions_service_account_key;
DROP INDEX thread_mentions_grant_key;
ALTER TABLE thread_mentions DROP CONSTRAINT thread_mentions_one_check;
ALTER TABLE thread_mentions ADD CONSTRAINT thread_mentions_one_check CHECK (num_nonnulls(user_id, agent) = 1);
ALTER TABLE thread_mentions DROP COLUMN agent_service_account_id;
ALTER TABLE thread_mentions DROP COLUMN agent_grant_id;
