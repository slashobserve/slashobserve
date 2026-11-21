-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_events_catch_up(since TIMESTAMPTZ, by_server TEXT, by_version TEXT, keep_position BOOLEAN, OUT inserted BIGINT)
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
    coalesce(a.origin, CASE WHEN a.run_id IS NULL THEN 'unknown' ELSE 'agent' END), a.origin_client, a.target_kind, a.target_id::text, a.folder_id, a.dashboard_id, a.observation_id,
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

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_events_catch_up(since TIMESTAMPTZ, by_server TEXT, by_version TEXT, keep_position BOOLEAN, OUT inserted BIGINT)
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
