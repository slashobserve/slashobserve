-- +goose Up
CREATE SEQUENCE audit_events_seq;

CREATE TABLE audit_events (
  id UUID NOT NULL,
  seq BIGINT NOT NULL DEFAULT nextval('audit_events_seq'),
  xid XID8 NOT NULL DEFAULT pg_current_xact_id(),
  occurred_at TIMESTAMPTZ NOT NULL,
  category TEXT NOT NULL CHECK (category IN ('auth', 'access', 'content', 'conversation', 'agent', 'query', 'admin', 'system')),
  action TEXT NOT NULL CHECK (action ~ '^[a-z_]+\.[a-z_]+$'),
  outcome TEXT NOT NULL CHECK (outcome IN ('success', 'denied', 'failed', 'unknown')),
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
  activity_seq BIGINT,
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
CREATE UNIQUE INDEX audit_events_activity_key ON audit_events (activity_seq, occurred_at);

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
CREATE FUNCTION audit_events_attach(name TEXT, starts DATE, ends DATE) RETURNS BOOLEAN
LANGUAGE plpgsql
AS $$
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended('audit_events_partitions', 0));
  IF EXISTS (SELECT 1 FROM pg_inherits WHERE inhparent = 'audit_events'::regclass AND inhrelid = to_regclass(name)) THEN
    RETURN false;
  END IF;
  IF to_regclass(name) IS NULL THEN
    EXECUTE format('CREATE TABLE %I (LIKE audit_events INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED)', name);
  END IF;
  EXECUTE format('ALTER TABLE audit_events ATTACH PARTITION %I FOR VALUES FROM (%L) TO (%L)', name,
    starts::timestamp AT TIME ZONE 'UTC', ends::timestamp AT TIME ZONE 'UTC');
  RETURN true;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION audit_events_add_partition(day DATE) RETURNS BOOLEAN
LANGUAGE sql
AS $$
  SELECT audit_events_attach('audit_events_' || to_char(day, 'YYYYMMDD'), day, day + 1);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  daily_from DATE := ((now() - interval '1 hour') AT TIME ZONE 'UTC')::date;
  first DATE := coalesce((SELECT (min(created_at) AT TIME ZONE 'UTC')::date FROM activity), daily_from);
  month DATE;
  day DATE;
BEGIN
  IF first < daily_from THEN
    FOR month IN SELECT generate_series(date_trunc('month', first::timestamp), date_trunc('month', (daily_from - 1)::timestamp), interval '1 month')::date LOOP
      PERFORM audit_events_attach('audit_events_' || to_char(month, 'YYYYMM'), month, least((month + interval '1 month')::date, daily_from));
    END LOOP;
  END IF;
  FOR day IN SELECT generate_series(daily_from, (now() AT TIME ZONE 'UTC')::date + 14, interval '1 day')::date LOOP
    PERFORM audit_events_add_partition(day);
  END LOOP;
END
$$;
-- +goose StatementEnd

CREATE TABLE audit_activity_actions (
  type TEXT PRIMARY KEY,
  action TEXT NOT NULL,
  category TEXT NOT NULL,
  keys TEXT[] NOT NULL,
  extra JSONB NOT NULL
);

INSERT INTO audit_activity_actions (type, action, category, keys, extra) VALUES
  ('dashboard.created', 'dashboard.create', 'content', '{version,parent_version,restored_from,title,summary}', '{}'),
  ('dashboard.version_created', 'dashboard.update', 'content', '{version,parent_version,restored_from,title,summary}', '{}'),
  ('dashboard.restored', 'dashboard.restore', 'content', '{version,parent_version,restored_from,title,summary}', '{}'),
  ('dashboard.moved', 'dashboard.move', 'content', '{title,from_folder_id,to_folder_id}', '{}'),
  ('dashboard.deleted', 'dashboard.delete', 'content', '{title}', '{}'),
  ('annotation.created', 'annotation.create', 'content', '{title,panel_id,tags,time_from,time_to}', '{}'),
  ('annotation.updated', 'annotation.update', 'content', '{title,panel_id,tags,time_from,time_to}', '{}'),
  ('annotation.deleted', 'annotation.delete', 'content', '{title,panel_id,tags,time_from,time_to}', '{}'),
  ('finding.created', 'finding.create', 'content', '{title,agent_request_id}', '{}'),
  ('finding.pinned', 'finding.pin', 'content', '{annotation_id}', '{}'),
  ('finding.moved', 'finding.move', 'content', '{title,from_folder_id,to_folder_id}', '{}'),
  ('saved_search.created', 'saved_search.create', 'content', '{title,kind,changed,from_folder_id,to_folder_id}', '{}'),
  ('saved_search.updated', 'saved_search.update', 'content', '{title,kind,changed,from_folder_id,to_folder_id}', '{}'),
  ('saved_search.moved', 'saved_search.move', 'content', '{title,kind,changed,from_folder_id,to_folder_id}', '{}'),
  ('saved_search.deleted', 'saved_search.delete', 'content', '{title,kind,changed,from_folder_id,to_folder_id}', '{}'),
  ('observation.created', 'observation.create', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.updated', 'observation.update', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.moved', 'observation.move', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.deleted', 'observation.delete', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.resolved', 'observation.resolve', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.reopened', 'observation.reopen', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.published', 'observation.publish', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.rca_withdrawn', 'observation.rca_withdraw', 'content', '{title,from_title,from_folder_id,to_folder_id,revision}', '{}'),
  ('observation.block_added', 'observation.block_add', 'content', '{title,block_id,block_kind,state}', '{}'),
  ('observation.block_updated', 'observation.block_update', 'content', '{title,block_id,block_kind,state}', '{}'),
  ('observation.block_kept', 'observation.block_keep', 'content', '{title,block_id,block_kind,state}', '{}'),
  ('observation.block_discarded', 'observation.block_discard', 'content', '{title,block_id,block_kind,state}', '{}'),
  ('observation.block_removed', 'observation.block_remove', 'content', '{title,block_id,block_kind,state}', '{}'),
  ('proposal.opened', 'proposal.open', 'content', '{title,revision,base_version,version}', '{}'),
  ('proposal.updated', 'proposal.update', 'content', '{title,revision,base_version,version}', '{}'),
  ('proposal.merged', 'proposal.merge', 'content', '{title,revision,base_version,version}', '{}'),
  ('proposal.discarded', 'proposal.discard', 'content', '{title,revision,base_version,version}', '{}'),
  ('thread.opened', 'thread.open', 'conversation', '{object,title,version,panel_id,message_id,resolved_by_version,mentioned}', '{}'),
  ('thread.replied', 'thread.message', 'conversation', '{object,title,version,panel_id,message_id,resolved_by_version,mentioned}', '{}'),
  ('thread.edited', 'thread.message_edit', 'conversation', '{object,title,version,panel_id,message_id,resolved_by_version,mentioned}', '{}'),
  ('thread.deleted', 'thread.message_delete', 'conversation', '{object,title,version,panel_id,message_id,resolved_by_version,mentioned}', '{}'),
  ('thread.resolved', 'thread.resolve', 'conversation', '{object,title,version,panel_id,message_id,resolved_by_version,mentioned}', '{}'),
  ('thread.reopened', 'thread.reopen', 'conversation', '{object,title,version,panel_id,message_id,resolved_by_version,mentioned}', '{}'),
  ('thread.mentioned', 'thread.mention', 'conversation', '{object,title,version,panel_id,message_id,resolved_by_version,mentioned}', '{}'),
  ('run.queued', 'run.queue', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{}'),
  ('run.started', 'run.start', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{}'),
  ('run.succeeded', 'run.finish', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{"result": "succeeded"}'),
  ('run.failed', 'run.finish', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{"result": "failed"}'),
  ('run.canceled', 'run.finish', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{"result": "canceled"}'),
  ('run.canceling', 'run.cancel', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{}'),
  ('run.released', 'run.release', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{}'),
  ('run.kept', 'run.keep', 'agent', '{source,continues_id,attempt,result_version,failure_code,cost_usd,dashboard_title,panels_repaired,result}', '{}'),
  ('public_link.created', 'public_link.create', 'admin', '{object,title,prefix,expires_at}', '{}'),
  ('public_link.revoked', 'public_link.revoke', 'admin', '{object,title,prefix,expires_at}', '{}'),
  ('access_request.opened', 'access_request.open', 'admin', '{object,title,general,requested_by}', '{}'),
  ('access_request.granted', 'access_request.grant', 'admin', '{object,title,general,requested_by}', '{}'),
  ('user.role_changed', 'user.role_change', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('user.signed_out_everywhere', 'user.sign_out_everywhere', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('user.invited', 'user.invite', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('user.invite_updated', 'user.invite_update', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('user.invite_withdrawn', 'user.invite_withdraw', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('user.deactivated', 'user.deactivate', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('user.reactivated', 'user.reactivate', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('user.removed', 'user.remove', 'admin', '{name,email,role,from_role,to_role,manual,source,sessions,grants}', '{}'),
  ('service_account.created', 'service_account.create', 'admin', '{name,role,takes_runs,key_id,key_name,expires_at}', '{}'),
  ('service_account.updated', 'service_account.update', 'admin', '{name,role,takes_runs,key_id,key_name,expires_at}', '{}'),
  ('service_account.deleted', 'service_account.delete', 'admin', '{name,role,takes_runs,key_id,key_name,expires_at}', '{}'),
  ('service_account.key_created', 'service_account.key_create', 'admin', '{name,role,takes_runs,key_id,key_name,expires_at}', '{}'),
  ('service_account.key_revoked', 'service_account.key_revoke', 'admin', '{name,role,takes_runs,key_id,key_name,expires_at}', '{}'),
  ('webhook.created', 'webhook.create', 'admin', '{name,host,event_types,changes,url_changed}', '{}'),
  ('webhook.updated', 'webhook.update', 'admin', '{name,host,event_types,changes,url_changed}', '{}'),
  ('webhook.secret_rotated', 'webhook.secret_rotate', 'admin', '{name,host,event_types,changes,url_changed}', '{}'),
  ('webhook.deleted', 'webhook.delete', 'admin', '{name,host,event_types,changes,url_changed}', '{}'),
  ('mcp_client.connected', 'mcp_client.connect', 'admin', '{client,person_id,person_name,mode,reason}', '{}'),
  ('mcp_client.disconnected', 'mcp_client.disconnect', 'admin', '{client,person_id,person_name,mode,reason}', '{}'),
  ('outbound_domain.added', 'outbound_domain.add', 'admin', '{domain}', '{}'),
  ('outbound_domain.removed', 'outbound_domain.remove', 'admin', '{domain}', '{}'),
  ('grafana_connection.created', 'grafana_connection.create', 'admin', '{name,host,changes,token_changed}', '{}'),
  ('grafana_connection.updated', 'grafana_connection.update', 'admin', '{name,host,changes,token_changed}', '{}'),
  ('grafana_connection.deleted', 'grafana_connection.delete', 'admin', '{name,host,changes,token_changed}', '{}'),
  ('datasource.created', 'datasource.create', 'admin', '{uid,type,name,host,changes,address_changed,settings_changed,credentials_changed}', '{}'),
  ('datasource.updated', 'datasource.update', 'admin', '{uid,type,name,host,changes,address_changed,settings_changed,credentials_changed}', '{}'),
  ('datasource.deleted', 'datasource.delete', 'admin', '{uid,type,name,host,changes,address_changed,settings_changed,credentials_changed}', '{}');

CREATE TABLE audit_backfill (
  singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
  migrated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO audit_backfill DEFAULT VALUES;

-- +goose StatementBegin
CREATE FUNCTION audit_events_catch_up(since TIMESTAMPTZ, by_server TEXT, by_version TEXT, keep_position BOOLEAN, OUT inserted BIGINT)
LANGUAGE plpgsql
AS $$
DECLARE
  dated_until TIMESTAMPTZ := clock_timestamp();
  future BIGINT;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended('audit_events_catch_up', 0));
  INSERT INTO audit_events (id, xid, occurred_at, category, action, outcome, actor_kind, actor_id, actor_name, initiator, run_id,
    origin, origin_client, target_kind, target_id, folder_id, dashboard_id, observation_id, data, activity_seq, server_id, server_version)
  SELECT
    (lpad(to_hex((extract(epoch FROM a.created_at) * 1000)::bigint), 12, '0') || '7' || substr(md5('audit:' || a.seq), 1, 3) ||
      to_hex(8 + get_byte(decode(md5('variant:' || a.seq), 'hex'), 0) % 4) || substr(md5('audit:' || a.seq), 4, 15))::uuid,
    CASE WHEN keep_position THEN a.xid ELSE pg_current_xact_id() END, a.created_at, m.category, m.action, 'success',
    a.actor_kind, a.actor_id, a.actor_name, CASE WHEN a.run_id IS NULL THEN 'person' ELSE 'agent' END, a.run_id,
    coalesce(a.origin, 'unknown'), a.origin_client, a.target_kind, a.target_id::text, a.folder_id, a.dashboard_id, a.observation_id,
    (SELECT coalesce(jsonb_object_agg(d.key, d.value), '{}'::jsonb) FROM jsonb_each(a.data || m.extra) AS d WHERE d.key = ANY (m.keys)),
    a.seq, by_server, by_version
  FROM activity a
  JOIN audit_activity_actions m ON m.type = a.type
  WHERE a.created_at >= since AND a.created_at <= dated_until AND NOT EXISTS (SELECT 1 FROM audit_events e WHERE e.activity_seq = a.seq)
  ORDER BY a.xid, a.seq
  ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS inserted = ROW_COUNT;
  SELECT count(*) INTO future FROM activity a JOIN audit_activity_actions m ON m.type = a.type WHERE a.created_at > dated_until;
  IF future > 0 THEN
    RAISE NOTICE '% activity rows dated after % have no audit event', future, dated_until;
  END IF;
END
$$;
-- +goose StatementEnd

SELECT audit_events_catch_up('-infinity', 'backfill', 'backfill', true);

-- +goose Down
DROP FUNCTION audit_events_catch_up(TIMESTAMPTZ, TEXT, TEXT, BOOLEAN);
DROP TABLE audit_backfill;
DROP TABLE audit_activity_actions;
DROP TABLE audit_events;
DROP FUNCTION audit_events_add_partition(DATE);
DROP FUNCTION audit_events_attach(TEXT, DATE, DATE);
DROP FUNCTION audit_events_refuse_change();
