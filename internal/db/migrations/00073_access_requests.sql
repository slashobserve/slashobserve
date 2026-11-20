-- +goose Up
CREATE TABLE access_requests (
  id UUID PRIMARY KEY,
  dashboard_id UUID REFERENCES dashboards (id) ON DELETE CASCADE,
  observation_id UUID REFERENCES observations (id) ON DELETE CASCADE,
  requester_kind TEXT NOT NULL CHECK (requester_kind IN ('user', 'service_account')),
  requester_id UUID NOT NULL,
  requester_name TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '' CHECK (char_length(note) <= 200 AND position(E'\n' IN note) = 0 AND position(E'\r' IN note) = 0),
  granters UUID[] NOT NULL,
  activity_seq BIGINT REFERENCES activity (seq) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  granted_at TIMESTAMPTZ,
  granted_by_kind TEXT,
  granted_by_id UUID,
  CHECK (num_nonnulls(dashboard_id, observation_id) = 1)
);
CREATE INDEX access_requests_object_idx ON access_requests (coalesce(dashboard_id, observation_id), requester_kind, requester_id);
CREATE INDEX access_requests_requester_idx ON access_requests (requester_kind, requester_id) WHERE granted_at IS NULL;
CREATE INDEX access_requests_activity_idx ON access_requests (activity_seq);

-- +goose StatementBegin
CREATE FUNCTION access_request_open(r access_requests) RETURNS boolean
LANGUAGE sql STABLE
AS $$
  SELECT r.granted_at IS NULL AND r.created_at >= now() - interval '30 days' AND NOT EXISTS (
    SELECT 1 FROM inbox_item_states s
    WHERE s.seq = r.activity_seq AND s.done AND s.user_id = ANY(r.granters)
  )
$$;
-- +goose StatementEnd

ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'observation', 'proposal',
    'user', 'service_account', 'webhook', 'mcp_client', 'outbound_domain', 'grafana_connection', 'public_link', 'datasource',
    'access_request'));

DROP INDEX activity_inbox_idx;
CREATE INDEX activity_inbox_idx ON activity (xid, seq)
  WHERE type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded', 'proposal.merged', 'proposal.discarded', 'access_request.opened');

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[], repair_prefix text) RETURNS text
LANGUAGE sql STABLE
AS $$
  SELECT CASE
    WHEN a.type = 'proposal.opened' AND a.folder_id = ANY(editable) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.state = 'open' AND NOT (p.author_kind = 'user' AND p.author_id = reader)
    ) THEN 'proposal'
    WHEN a.type = 'observation.block_added' AND a.data->>'state' = 'requested'
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM observations o
      JOIN observation_blocks b ON b.observation_id = o.id
      WHERE o.id = a.target_id AND b.id = (a.data->>'block_id')::uuid AND b.state = 'requested'
        AND o.created_by_kind = 'user' AND o.created_by_id = reader AND o.folder_id = ANY(editable)
    ) THEN 'requested_block'
    WHEN a.type = 'thread.mentioned' AND NOT (a.actor_kind = 'user' AND a.actor_id = reader)
      AND a.data->'mentioned'->>'kind' = 'user' AND a.data->'mentioned'->>'id' = reader::text
    THEN 'mention'
    WHEN a.type = 'thread.replied' AND (
      (NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
        SELECT 1 FROM thread_messages reply
        JOIN thread_messages earlier ON earlier.thread_id = reply.thread_id AND earlier.created_at < reply.created_at
        WHERE reply.id = (a.data->>'message_id')::uuid AND reply.thread_id = a.target_id
          AND earlier.author_kind = 'user' AND earlier.author_id = reader
      ))
      OR (a.run_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM agent_requests r
        WHERE r.id = a.run_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
      ))
    ) THEN 'reply'
    WHEN a.type = 'run.failed' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader AND r.state = 'failed'
        AND NOT EXISTS (SELECT 1 FROM agent_requests c WHERE c.continues_id = r.id)
    ) THEN 'run_failed'
    WHEN a.type = 'run.succeeded' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
        AND r.source = 'prompt' AND starts_with(r.prompt, repair_prefix) AND r.result_version IS NOT NULL
    ) THEN 'repaired'
    WHEN a.type IN ('proposal.merged', 'proposal.discarded')
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.author_kind = 'user' AND p.author_id = reader
    ) THEN 'proposal_closed'
    WHEN a.type = 'access_request.opened' AND EXISTS (
      SELECT 1 FROM access_requests r
      WHERE r.id = a.target_id AND reader = ANY(r.granters) AND (access_request_open(r) OR (r.granted_at IS NULL AND EXISTS (
        SELECT 1 FROM inbox_item_states s WHERE s.seq = a.seq AND s.user_id = reader AND s.done
      )))
    ) THEN 'access_request'
  END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[], repair_prefix text) RETURNS text
LANGUAGE sql STABLE
AS $$
  SELECT CASE
    WHEN a.type = 'proposal.opened' AND a.folder_id = ANY(editable) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.state = 'open' AND NOT (p.author_kind = 'user' AND p.author_id = reader)
    ) THEN 'proposal'
    WHEN a.type = 'observation.block_added' AND a.data->>'state' = 'requested'
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM observations o
      JOIN observation_blocks b ON b.observation_id = o.id
      WHERE o.id = a.target_id AND b.id = (a.data->>'block_id')::uuid AND b.state = 'requested'
        AND o.created_by_kind = 'user' AND o.created_by_id = reader AND o.folder_id = ANY(editable)
    ) THEN 'requested_block'
    WHEN a.type = 'thread.mentioned' AND NOT (a.actor_kind = 'user' AND a.actor_id = reader)
      AND a.data->'mentioned'->>'kind' = 'user' AND a.data->'mentioned'->>'id' = reader::text
    THEN 'mention'
    WHEN a.type = 'thread.replied' AND (
      (NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
        SELECT 1 FROM thread_messages reply
        JOIN thread_messages earlier ON earlier.thread_id = reply.thread_id AND earlier.created_at < reply.created_at
        WHERE reply.id = (a.data->>'message_id')::uuid AND reply.thread_id = a.target_id
          AND earlier.author_kind = 'user' AND earlier.author_id = reader
      ))
      OR (a.run_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM agent_requests r
        WHERE r.id = a.run_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
      ))
    ) THEN 'reply'
    WHEN a.type = 'run.failed' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader AND r.state = 'failed'
        AND NOT EXISTS (SELECT 1 FROM agent_requests c WHERE c.continues_id = r.id)
    ) THEN 'run_failed'
    WHEN a.type = 'run.succeeded' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
        AND r.source = 'prompt' AND starts_with(r.prompt, repair_prefix) AND r.result_version IS NOT NULL
    ) THEN 'repaired'
    WHEN a.type IN ('proposal.merged', 'proposal.discarded')
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.author_kind = 'user' AND p.author_id = reader
    ) THEN 'proposal_closed'
  END
$$;
-- +goose StatementEnd

DROP INDEX activity_inbox_idx;
CREATE INDEX activity_inbox_idx ON activity (xid, seq)
  WHERE type IN ('proposal.opened', 'observation.block_added', 'thread.mentioned', 'thread.replied', 'run.failed', 'run.succeeded', 'proposal.merged', 'proposal.discarded');

DELETE FROM activity WHERE target_kind = 'access_request';
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'observation', 'proposal',
    'user', 'service_account', 'webhook', 'mcp_client', 'outbound_domain', 'grafana_connection', 'public_link', 'datasource'));

DROP FUNCTION access_request_open(access_requests);
DROP TABLE access_requests;
