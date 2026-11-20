-- +goose Up
CREATE TABLE chats (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_kind TEXT NOT NULL CHECK (owner_kind IN ('user', 'service_account')),
  owner_id UUID NOT NULL,
  title TEXT CHECK (title IS NULL OR length(title) BETWEEN 1 AND 120),
  dashboard_id UUID REFERENCES dashboards (id) ON DELETE SET NULL,
  observation_id UUID REFERENCES observations (id) ON DELETE SET NULL,
  pinned_at TIMESTAMPTZ,
  archived_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_turn_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chats_subject_check CHECK (dashboard_id IS NULL OR observation_id IS NULL)
);
CREATE INDEX chats_owner_recent_idx ON chats (owner_id, last_turn_at DESC, id DESC) WHERE archived_at IS NULL;
CREATE INDEX chats_owner_dashboard_idx ON chats (owner_id, dashboard_id) WHERE dashboard_id IS NOT NULL;
CREATE INDEX chats_owner_observation_idx ON chats (owner_id, observation_id) WHERE observation_id IS NOT NULL;

ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_chat_id_fkey;

UPDATE agent_requests SET chat_id = id
WHERE chat_id IS NULL AND source = 'observe' AND thread_message_id IS NULL;

UPDATE agent_requests r SET chat_id = g.first_id
FROM (
  SELECT id, first_value(id) OVER (PARTITION BY requested_by_kind, requested_by_id, dashboard_id ORDER BY created_at, id) AS first_id
  FROM agent_requests
  WHERE chat_id IS NULL AND source = 'prompt' AND dashboard_id IS NOT NULL AND thread_message_id IS NULL
) g
WHERE r.id = g.id;

UPDATE agent_requests SET chat_id = id
WHERE chat_id IS NULL AND source NOT IN ('suggest', 'comment') AND thread_message_id IS NULL;

INSERT INTO chats (id, owner_kind, owner_id, title, dashboard_id, observation_id, pinned_at, archived_at, created_at, last_turn_at)
SELECT g.chat_id, root.requested_by_kind, root.requested_by_id, root.title,
  g.dashboard_id, CASE WHEN g.dashboard_id IS NULL AND g.observations = 1 THEN g.observation_id END,
  root.pinned_at, root.archived_at, g.created_at, g.last_turn_at
FROM (
  SELECT r.chat_id,
    (array_agg(r.dashboard_id ORDER BY r.created_at, r.id) FILTER (WHERE r.dashboard_id IS NOT NULL))[1] AS dashboard_id,
    count(DISTINCT o.id) AS observations,
    (array_agg(o.id) FILTER (WHERE o.id IS NOT NULL))[1] AS observation_id,
    min(r.created_at) AS created_at,
    max(r.created_at) AS last_turn_at
  FROM agent_requests r
  LEFT JOIN observations o ON o.run_id = r.id
  WHERE r.chat_id IS NOT NULL
  GROUP BY r.chat_id
) g
JOIN agent_requests root ON root.id = g.chat_id;

ALTER TABLE agent_requests
  ADD CONSTRAINT agent_requests_chat_id_fkey FOREIGN KEY (chat_id) REFERENCES chats (id) ON DELETE SET NULL,
  ADD COLUMN client_id UUID;
CREATE UNIQUE INDEX agent_requests_client_idx ON agent_requests (requested_by_kind, requested_by_id, client_id) WHERE client_id IS NOT NULL;
CREATE INDEX agent_requests_chat_order_idx ON agent_requests (chat_id, created_at, id) WHERE chat_id IS NOT NULL AND status IN ('queued', 'running');

-- +goose Down
DROP INDEX agent_requests_chat_order_idx;
DROP INDEX agent_requests_client_idx;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_chat_id_fkey, DROP COLUMN client_id;

UPDATE agent_requests r SET chat_id = NULL
WHERE r.chat_id = r.id OR r.source <> 'observe'
  OR NOT EXISTS (SELECT 1 FROM agent_requests p WHERE p.id = r.chat_id);

ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_chat_id_fkey FOREIGN KEY (chat_id) REFERENCES agent_requests (id) ON DELETE SET NULL;

DROP TABLE chats;
