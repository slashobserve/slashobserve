-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE threads
  ADD COLUMN IF NOT EXISTS agent_kind TEXT,
  ADD COLUMN IF NOT EXISTS agent_id UUID,
  ADD COLUMN IF NOT EXISTS agent_off BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS anchor JSONB,
  ADD COLUMN IF NOT EXISTS block_id UUID;

ALTER TABLE threads
  DROP CONSTRAINT IF EXISTS threads_agent_kind_check,
  DROP CONSTRAINT IF EXISTS threads_agent_check,
  DROP CONSTRAINT IF EXISTS threads_point_check,
  DROP CONSTRAINT IF EXISTS threads_block_point_check,
  DROP CONSTRAINT IF EXISTS threads_block_id_fkey,
  ADD CONSTRAINT threads_agent_kind_check CHECK (agent_kind IN ('builtin', 'grant', 'service_account')) NOT VALID,
  ADD CONSTRAINT threads_agent_check CHECK (
    (agent_kind IS NULL AND agent_id IS NULL)
    OR (agent_kind = 'builtin' AND agent_id IS NULL)
    OR (agent_kind IN ('grant', 'service_account') AND agent_id IS NOT NULL)) NOT VALID,
  ADD CONSTRAINT threads_point_check CHECK (anchor IS NULL OR (jsonb_typeof(anchor) = 'object' AND pg_column_size(anchor) <= 2048)) NOT VALID,
  ADD CONSTRAINT threads_block_point_check CHECK (block_id IS NULL OR anchor IS NOT NULL) NOT VALID,
  ADD CONSTRAINT threads_block_id_fkey FOREIGN KEY (block_id) REFERENCES observation_blocks (id) ON DELETE SET NULL NOT VALID;

UPDATE threads t SET agent_kind = m.run_assigned_kind, agent_id = r.assigned_id
FROM (SELECT DISTINCT ON (thread_id) thread_id, run_id, run_assigned_kind
      FROM thread_messages WHERE run_id IS NOT NULL ORDER BY thread_id, created_at DESC, id DESC) m
JOIN agent_requests r ON r.id = m.run_id
WHERE t.id = m.thread_id AND t.agent_kind IS NULL
  AND (m.run_assigned_kind = 'builtin'
    OR (m.run_assigned_kind = 'grant' AND EXISTS (
      SELECT 1 FROM oauth_grants g WHERE g.id = r.assigned_id AND g.revoked_at IS NULL AND g.expires_at > now()))
    OR (m.run_assigned_kind = 'service_account' AND EXISTS (
      SELECT 1 FROM service_accounts s WHERE s.id = r.assigned_id AND s.takes_runs)));

ALTER TABLE threads VALIDATE CONSTRAINT threads_agent_kind_check;
ALTER TABLE threads VALIDATE CONSTRAINT threads_agent_check;
ALTER TABLE threads VALIDATE CONSTRAINT threads_point_check;
ALTER TABLE threads VALIDATE CONSTRAINT threads_block_point_check;
ALTER TABLE threads VALIDATE CONSTRAINT threads_block_id_fkey;

DROP INDEX CONCURRENTLY IF EXISTS threads_observation_page_idx;
CREATE UNIQUE INDEX CONCURRENTLY threads_observation_page_idx ON threads (observation_id)
  WHERE observation_id IS NOT NULL AND anchor IS NULL AND block_id IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS threads_observation_idx;
CREATE INDEX CONCURRENTLY threads_observation_idx ON threads (observation_id) WHERE observation_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS threads_block_idx;
CREATE INDEX CONCURRENTLY threads_block_idx ON threads (block_id) WHERE block_id IS NOT NULL;

ALTER TABLE thread_messages ADD COLUMN IF NOT EXISTS client_id UUID;
DROP INDEX CONCURRENTLY IF EXISTS thread_messages_client_idx;
CREATE UNIQUE INDEX CONCURRENTLY thread_messages_client_idx ON thread_messages (author_kind, author_id, client_id) WHERE client_id IS NOT NULL;

ALTER TABLE agent_requests
  ADD COLUMN IF NOT EXISTS anchor JSONB,
  ADD COLUMN IF NOT EXISTS block_id UUID;

ALTER TABLE agent_requests
  DROP CONSTRAINT IF EXISTS agent_requests_anchor_check,
  DROP CONSTRAINT IF EXISTS agent_requests_block_id_fkey,
  ADD CONSTRAINT agent_requests_anchor_check CHECK (anchor IS NULL OR (jsonb_typeof(anchor) = 'object' AND pg_column_size(anchor) <= 2048)) NOT VALID,
  ADD CONSTRAINT agent_requests_block_id_fkey FOREIGN KEY (block_id) REFERENCES observation_blocks (id) ON DELETE SET NULL NOT VALID;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_anchor_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_block_id_fkey;

-- +goose Down
ALTER TABLE agent_requests
  DROP CONSTRAINT IF EXISTS agent_requests_block_id_fkey,
  DROP CONSTRAINT IF EXISTS agent_requests_anchor_check,
  DROP COLUMN IF EXISTS block_id,
  DROP COLUMN IF EXISTS anchor;

DROP INDEX IF EXISTS thread_messages_client_idx;
ALTER TABLE thread_messages DROP COLUMN IF EXISTS client_id;

DROP INDEX IF EXISTS threads_block_idx;
DROP INDEX IF EXISTS threads_observation_idx;
DROP INDEX IF EXISTS threads_observation_page_idx;
DELETE FROM threads WHERE observation_id IS NOT NULL AND (anchor IS NOT NULL OR block_id IS NOT NULL);
CREATE UNIQUE INDEX threads_observation_idx ON threads (observation_id) WHERE observation_id IS NOT NULL;

ALTER TABLE threads
  DROP CONSTRAINT IF EXISTS threads_block_id_fkey,
  DROP CONSTRAINT IF EXISTS threads_block_point_check,
  DROP CONSTRAINT IF EXISTS threads_point_check,
  DROP CONSTRAINT IF EXISTS threads_agent_check,
  DROP CONSTRAINT IF EXISTS threads_agent_kind_check,
  DROP COLUMN IF EXISTS block_id,
  DROP COLUMN IF EXISTS anchor,
  DROP COLUMN IF EXISTS agent_off,
  DROP COLUMN IF EXISTS agent_id,
  DROP COLUMN IF EXISTS agent_kind;
