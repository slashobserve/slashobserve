-- +goose Up
ALTER TABLE threads
  ADD COLUMN agent_kind TEXT CHECK (agent_kind IN ('builtin', 'grant', 'service_account')),
  ADD COLUMN agent_id UUID,
  ADD COLUMN agent_off BOOLEAN NOT NULL DEFAULT false,
  ADD CONSTRAINT threads_agent_check CHECK (
    (agent_kind IS NULL AND agent_id IS NULL)
    OR (agent_kind = 'builtin' AND agent_id IS NULL)
    OR (agent_kind IN ('grant', 'service_account') AND agent_id IS NOT NULL));

UPDATE threads t SET agent_kind = m.run_assigned_kind, agent_id = r.assigned_id
FROM (SELECT DISTINCT ON (thread_id) thread_id, run_id, run_assigned_kind
      FROM thread_messages WHERE run_id IS NOT NULL ORDER BY thread_id, created_at DESC, id DESC) m
JOIN agent_requests r ON r.id = m.run_id
WHERE t.id = m.thread_id
  AND (m.run_assigned_kind = 'builtin'
    OR (m.run_assigned_kind = 'grant' AND EXISTS (
      SELECT 1 FROM oauth_grants g WHERE g.id = r.assigned_id AND g.revoked_at IS NULL AND g.expires_at > now()))
    OR (m.run_assigned_kind = 'service_account' AND EXISTS (
      SELECT 1 FROM service_accounts s WHERE s.id = r.assigned_id AND s.takes_runs)));

ALTER TABLE threads
  ADD COLUMN anchor JSONB,
  ADD COLUMN block_id UUID REFERENCES observation_blocks (id) ON DELETE SET NULL,
  ADD CONSTRAINT threads_point_check CHECK (anchor IS NULL OR (jsonb_typeof(anchor) = 'object' AND pg_column_size(anchor) <= 2048)),
  ADD CONSTRAINT threads_block_point_check CHECK (block_id IS NULL OR anchor IS NOT NULL);

DROP INDEX threads_observation_idx;
CREATE UNIQUE INDEX threads_observation_page_idx ON threads (observation_id)
  WHERE observation_id IS NOT NULL AND anchor IS NULL AND block_id IS NULL;
CREATE INDEX threads_observation_idx ON threads (observation_id) WHERE observation_id IS NOT NULL;
CREATE INDEX threads_block_idx ON threads (block_id) WHERE block_id IS NOT NULL;

ALTER TABLE thread_messages ADD COLUMN client_id UUID;
CREATE UNIQUE INDEX thread_messages_client_idx ON thread_messages (author_kind, author_id, client_id) WHERE client_id IS NOT NULL;

ALTER TABLE agent_requests
  ADD COLUMN anchor JSONB CONSTRAINT agent_requests_anchor_check CHECK (anchor IS NULL OR (jsonb_typeof(anchor) = 'object' AND pg_column_size(anchor) <= 2048)),
  ADD COLUMN block_id UUID REFERENCES observation_blocks (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN block_id, DROP COLUMN anchor;

DROP INDEX thread_messages_client_idx;
ALTER TABLE thread_messages DROP COLUMN client_id;

DROP INDEX threads_block_idx;
DROP INDEX threads_observation_idx;
DROP INDEX threads_observation_page_idx;
DELETE FROM threads WHERE observation_id IS NOT NULL AND (anchor IS NOT NULL OR block_id IS NOT NULL);
CREATE UNIQUE INDEX threads_observation_idx ON threads (observation_id) WHERE observation_id IS NOT NULL;

ALTER TABLE threads
  DROP CONSTRAINT threads_block_point_check,
  DROP CONSTRAINT threads_point_check,
  DROP COLUMN block_id,
  DROP COLUMN anchor,
  DROP CONSTRAINT threads_agent_check,
  DROP COLUMN agent_off,
  DROP COLUMN agent_id,
  DROP COLUMN agent_kind;
