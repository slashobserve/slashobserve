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
WHERE c.relkind = 'r' AND n.nspname = current_schema() AND c.relname ~ '^audit_events_([0-9]{6}|[0-9]{8})$'
  AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_inherits i WHERE i.inhrelid = c.oid)
ORDER BY c.relname;

-- name: GetAuditBackfill :one
SELECT migrated_at FROM audit_backfill;

-- name: CatchUpAuditEvents :one
SELECT audit_events_catch_up(@since::timestamptz, @by_server::text, @by_version::text, false)::bigint AS inserted;
