-- name: InsertAuditEvent :exec
INSERT INTO audit_events (id, occurred_at, category, action, outcome, reason, status, duration_ms,
  actor_kind, actor_id, actor_name, actor_role, initiator, agent_id, agent_name, run_id, auth_method, credential_id,
  origin, origin_client, tool, client_ip, user_agent, request_id, trace_id, http_method, route,
  target_kind, target_id, target_name, folder_id, dashboard_id, observation_id, thread_id, chat_id, datasource_uid,
  data, activity_seq, server_id, server_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40);

-- name: CopyAuditEvents :copyfrom
INSERT INTO audit_events (id, occurred_at, category, action, outcome, reason, status, duration_ms,
  actor_kind, actor_id, actor_name, actor_role, initiator, agent_id, agent_name, run_id, auth_method, credential_id,
  origin, origin_client, tool, client_ip, user_agent, request_id, trace_id, http_method, route,
  target_kind, target_id, target_name, folder_id, dashboard_id, observation_id, thread_id, chat_id, datasource_uid,
  data, activity_seq, server_id, server_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40);

-- name: CountCommittedAuditEvents :one
SELECT count(*) FROM audit_events
WHERE occurred_at = ANY(@occurred_at::timestamptz[]) AND id = ANY(@ids::uuid[]);

-- name: SetAuditLockTimeout :exec
SELECT set_config('lock_timeout', @timeout::text, true);

-- name: AddAuditPartition :one
SELECT audit_events_add_partition(@day::date);

-- name: ListAuditPartitions :many
SELECT c.relname::text AS name, i.inhdetachpending AS detach_pending
FROM pg_catalog.pg_inherits i JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
WHERE i.inhparent = 'audit_events'::regclass
ORDER BY c.relname;

-- name: ListDetachedAuditPartitions :many
SELECT c.relname::text AS name
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname = current_schema() AND c.relname ~ '^audit_events_([0-9]{4}|[0-9]{6}|[0-9]{8})$'
  AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_inherits i WHERE i.inhrelid = c.oid)
ORDER BY c.relname;

-- name: GetAuditExportCursor :one
SELECT xid, seq, occurred_at FROM audit_export_cursor;

-- name: AdvanceAuditExportCursor :exec
INSERT INTO audit_export_cursor (xid, seq, occurred_at) VALUES (@xid::xid8, @seq::bigint, @occurred_at::timestamptz)
ON CONFLICT (only_row) DO UPDATE SET xid = EXCLUDED.xid, seq = EXCLUDED.seq, occurred_at = GREATEST(audit_export_cursor.occurred_at, EXCLUDED.occurred_at), updated_at = now()
WHERE (audit_export_cursor.xid, audit_export_cursor.seq) < (EXCLUDED.xid, EXCLUDED.seq);

-- name: GetAuditExportLag :one
SELECT COALESCE(GREATEST(EXTRACT(EPOCH FROM edge.newest - edge.exported), 0), 0)::float8 AS lag_seconds
FROM (
  SELECT (SELECT max(occurred_at) FROM audit_events) AS newest,
         COALESCE((SELECT occurred_at FROM audit_export_cursor), (SELECT min(occurred_at) FROM audit_events)) AS exported
) AS edge;

-- name: GetOldestAuditEventAt :one
SELECT min(occurred_at)::timestamptz AS oldest FROM audit_events;
