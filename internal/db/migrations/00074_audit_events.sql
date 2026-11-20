-- +goose Up
CREATE SEQUENCE audit_events_seq;

CREATE TABLE audit_events (
  id UUID NOT NULL,
  seq BIGINT NOT NULL DEFAULT nextval('audit_events_seq'),
  xid XID8 NOT NULL DEFAULT pg_current_xact_id(),
  occurred_at TIMESTAMPTZ NOT NULL,
  category TEXT NOT NULL CHECK (category IN ('auth', 'access', 'content', 'conversation', 'agent', 'query', 'admin', 'system')),
  action TEXT NOT NULL CHECK (action ~ '^[a-z_]+\.[a-z_]+$'),
  outcome TEXT NOT NULL CHECK (outcome IN ('success', 'denied', 'failed')),
  reason TEXT,
  status INTEGER,
  duration_ms INTEGER,
  actor_kind TEXT NOT NULL CHECK (actor_kind IN ('user', 'service_account', 'anonymous', 'system')),
  actor_id UUID,
  actor_name TEXT,
  actor_role TEXT,
  initiator TEXT NOT NULL CHECK (initiator IN ('person', 'agent', 'system')),
  agent_id UUID,
  agent_name TEXT,
  run_id UUID,
  auth_method TEXT,
  credential_id TEXT,
  origin TEXT NOT NULL,
  origin_client TEXT,
  tool TEXT,
  client_ip INET,
  user_agent TEXT,
  request_id UUID,
  trace_id TEXT,
  http_method TEXT,
  route TEXT,
  target_kind TEXT,
  target_id TEXT,
  target_name TEXT,
  folder_id UUID,
  dashboard_id UUID,
  observation_id UUID,
  thread_id UUID,
  chat_id UUID,
  datasource_uid TEXT,
  data JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
  search TSVECTOR NOT NULL GENERATED ALWAYS AS (to_tsvector('simple'::regconfig,
    action || ' ' || coalesce(actor_name, '') || ' ' || coalesce(target_name, '') || ' ' || coalesce(reason, '') || ' ' ||
    coalesce(tool, '') || ' ' || coalesce(route, '') || ' ' || coalesce(data->>'query', ''))) STORED,
  server_id TEXT NOT NULL,
  server_version TEXT NOT NULL,
  PRIMARY KEY (occurred_at, id)
) PARTITION BY RANGE (occurred_at);

ALTER SEQUENCE audit_events_seq OWNED BY audit_events.seq;

CREATE INDEX audit_events_position_idx ON audit_events (xid, seq);
CREATE INDEX audit_events_action_idx ON audit_events (action, occurred_at DESC);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_id, occurred_at DESC);
CREATE INDEX audit_events_target_idx ON audit_events (target_kind, target_id, occurred_at DESC);
CREATE INDEX audit_events_run_idx ON audit_events (run_id) WHERE run_id IS NOT NULL;
CREATE INDEX audit_events_request_idx ON audit_events (request_id);
CREATE INDEX audit_events_search_idx ON audit_events USING gin (search);

-- +goose StatementBegin
CREATE FUNCTION audit_events_refuse_change() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'audit_events is append-only: % is refused', TG_OP USING ERRCODE = 'insufficient_privilege';
END
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_events_append_only BEFORE UPDATE OR DELETE ON audit_events
  FOR EACH ROW EXECUTE FUNCTION audit_events_refuse_change();

-- +goose StatementBegin
CREATE FUNCTION audit_events_add_partition(day DATE) RETURNS BOOLEAN
LANGUAGE plpgsql
AS $$
DECLARE
  name TEXT := 'audit_events_' || to_char(day, 'YYYYMMDD');
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended('audit_events_partitions', 0));
  IF EXISTS (SELECT 1 FROM pg_inherits WHERE inhparent = 'audit_events'::regclass AND inhrelid = to_regclass(name)) THEN
    RETURN false;
  END IF;
  IF to_regclass(name) IS NULL THEN
    EXECUTE format('CREATE TABLE %I (LIKE audit_events INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED)', name);
  END IF;
  EXECUTE format('ALTER TABLE audit_events ATTACH PARTITION %I FOR VALUES FROM (%L) TO (%L)', name,
    day::timestamp AT TIME ZONE 'UTC', (day + 1)::timestamp AT TIME ZONE 'UTC');
  RETURN true;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  today DATE := (now() AT TIME ZONE 'UTC')::date;
  first DATE := coalesce((SELECT (min(created_at) AT TIME ZONE 'UTC')::date FROM activity), today);
  day DATE;
BEGIN
  FOR day IN SELECT generate_series(least(first, today), today + 14, interval '1 day')::date LOOP
    PERFORM audit_events_add_partition(day);
  END LOOP;
END
$$;
-- +goose StatementEnd

INSERT INTO audit_events (id, xid, occurred_at, category, action, outcome, actor_kind, actor_id, actor_name, initiator, run_id,
  origin, origin_client, target_kind, target_id, folder_id, dashboard_id, observation_id, data, server_id, server_version)
SELECT
  (lpad(to_hex((extract(epoch FROM a.created_at) * 1000)::bigint), 12, '0') || '7' || substr(md5('audit:' || a.seq), 1, 3) ||
    to_hex(8 + get_byte(decode(md5('variant:' || a.seq), 'hex'), 0) % 4) || substr(md5('audit:' || a.seq), 4, 15))::uuid,
  a.xid, a.created_at, m.category, m.action, 'success', a.actor_kind, a.actor_id, a.actor_name,
  CASE WHEN a.run_id IS NULL THEN 'person' ELSE 'agent' END, a.run_id,
  coalesce(a.origin, 'job'), a.origin_client, a.target_kind, a.target_id::text, a.folder_id, a.dashboard_id, a.observation_id,
  a.data || m.extra, 'backfill', 'backfill'
FROM activity a
JOIN (VALUES
  ('dashboard.created', 'dashboard.create', 'content', '{}'::jsonb),
  ('dashboard.version_created', 'dashboard.update', 'content', '{}'),
  ('dashboard.restored', 'dashboard.restore', 'content', '{}'),
  ('dashboard.moved', 'dashboard.move', 'content', '{}'),
  ('dashboard.deleted', 'dashboard.delete', 'content', '{}'),
  ('annotation.created', 'annotation.create', 'content', '{}'),
  ('annotation.updated', 'annotation.update', 'content', '{}'),
  ('annotation.deleted', 'annotation.delete', 'content', '{}'),
  ('finding.created', 'finding.create', 'content', '{}'),
  ('finding.pinned', 'finding.pin', 'content', '{}'),
  ('finding.moved', 'finding.move', 'content', '{}'),
  ('saved_search.created', 'saved_search.create', 'content', '{}'),
  ('saved_search.updated', 'saved_search.update', 'content', '{}'),
  ('saved_search.moved', 'saved_search.move', 'content', '{}'),
  ('saved_search.deleted', 'saved_search.delete', 'content', '{}'),
  ('observation.created', 'observation.create', 'content', '{}'),
  ('observation.updated', 'observation.update', 'content', '{}'),
  ('observation.moved', 'observation.move', 'content', '{}'),
  ('observation.deleted', 'observation.delete', 'content', '{}'),
  ('observation.resolved', 'observation.resolve', 'content', '{}'),
  ('observation.reopened', 'observation.reopen', 'content', '{}'),
  ('observation.published', 'observation.publish', 'content', '{}'),
  ('observation.rca_withdrawn', 'observation.rca_withdraw', 'content', '{}'),
  ('observation.block_added', 'observation.block_add', 'content', '{}'),
  ('observation.block_updated', 'observation.block_update', 'content', '{}'),
  ('observation.block_kept', 'observation.block_keep', 'content', '{}'),
  ('observation.block_discarded', 'observation.block_discard', 'content', '{}'),
  ('observation.block_removed', 'observation.block_remove', 'content', '{}'),
  ('proposal.opened', 'proposal.open', 'content', '{}'),
  ('proposal.updated', 'proposal.update', 'content', '{}'),
  ('proposal.merged', 'proposal.merge', 'content', '{}'),
  ('proposal.discarded', 'proposal.discard', 'content', '{}'),
  ('thread.opened', 'thread.open', 'conversation', '{}'),
  ('thread.replied', 'thread.message', 'conversation', '{}'),
  ('thread.edited', 'thread.message_edit', 'conversation', '{}'),
  ('thread.deleted', 'thread.message_delete', 'conversation', '{}'),
  ('thread.resolved', 'thread.resolve', 'conversation', '{}'),
  ('thread.reopened', 'thread.reopen', 'conversation', '{}'),
  ('thread.mentioned', 'thread.mention', 'conversation', '{}'),
  ('run.queued', 'run.queue', 'agent', '{}'),
  ('run.started', 'run.start', 'agent', '{}'),
  ('run.succeeded', 'run.finish', 'agent', '{"result": "succeeded"}'),
  ('run.failed', 'run.finish', 'agent', '{"result": "failed"}'),
  ('run.canceled', 'run.finish', 'agent', '{"result": "canceled"}'),
  ('run.canceling', 'run.cancel', 'agent', '{}'),
  ('run.released', 'run.release', 'agent', '{}'),
  ('run.kept', 'run.keep', 'agent', '{}'),
  ('public_link.created', 'public_link.create', 'admin', '{}'),
  ('public_link.revoked', 'public_link.revoke', 'admin', '{}'),
  ('access_request.opened', 'access_request.open', 'admin', '{}'),
  ('access_request.granted', 'access_request.grant', 'admin', '{}'),
  ('user.role_changed', 'user.role_change', 'admin', '{}'),
  ('user.signed_out_everywhere', 'user.sign_out_everywhere', 'admin', '{}'),
  ('user.invited', 'user.invite', 'admin', '{}'),
  ('user.invite_updated', 'user.invite_update', 'admin', '{}'),
  ('user.invite_withdrawn', 'user.invite_withdraw', 'admin', '{}'),
  ('user.deactivated', 'user.deactivate', 'admin', '{}'),
  ('user.reactivated', 'user.reactivate', 'admin', '{}'),
  ('user.removed', 'user.remove', 'admin', '{}'),
  ('service_account.created', 'service_account.create', 'admin', '{}'),
  ('service_account.updated', 'service_account.update', 'admin', '{}'),
  ('service_account.deleted', 'service_account.delete', 'admin', '{}'),
  ('service_account.key_created', 'service_account.key_create', 'admin', '{}'),
  ('service_account.key_revoked', 'service_account.key_revoke', 'admin', '{}'),
  ('webhook.created', 'webhook.create', 'admin', '{}'),
  ('webhook.updated', 'webhook.update', 'admin', '{}'),
  ('webhook.secret_rotated', 'webhook.secret_rotate', 'admin', '{}'),
  ('webhook.deleted', 'webhook.delete', 'admin', '{}'),
  ('mcp_client.connected', 'mcp_client.connect', 'admin', '{}'),
  ('mcp_client.disconnected', 'mcp_client.disconnect', 'admin', '{}'),
  ('outbound_domain.added', 'outbound_domain.add', 'admin', '{}'),
  ('outbound_domain.removed', 'outbound_domain.remove', 'admin', '{}'),
  ('grafana_connection.created', 'grafana_connection.create', 'admin', '{}'),
  ('grafana_connection.updated', 'grafana_connection.update', 'admin', '{}'),
  ('grafana_connection.deleted', 'grafana_connection.delete', 'admin', '{}'),
  ('datasource.created', 'datasource.create', 'admin', '{}'),
  ('datasource.updated', 'datasource.update', 'admin', '{}'),
  ('datasource.deleted', 'datasource.delete', 'admin', '{}')
) AS m (type, action, category, extra) ON m.type = a.type
ORDER BY a.xid, a.seq;

-- +goose Down
DROP TABLE audit_events;
DROP FUNCTION audit_events_add_partition(DATE);
DROP FUNCTION audit_events_refuse_change();
