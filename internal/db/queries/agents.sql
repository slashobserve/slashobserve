-- name: AgentRunStats :one
SELECT count(*)::int AS runs,
       (count(*) FILTER (WHERE r.state = 'failed'))::int AS failed,
       coalesce(sum((r.run_usage->>'cost_usd')::float8), 0)::float8 AS cost_usd,
       (count(*) FILTER (WHERE r.status IN ('succeeded', 'failed') AND (r.run_usage->>'cost_usd') IS NULL))::int AS unpriced,
       (count(*) FILTER (WHERE r.started_at IS NOT NULL))::int AS started,
       coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM r.started_at - r.created_at)) FILTER (WHERE r.started_at IS NOT NULL), 0)::float8 AS median_queue_wait_seconds
FROM agent_requests r
WHERE r.assigned_kind = @assigned_kind
  AND r.assigned_id IS NOT DISTINCT FROM sqlc.narg('assigned_id')::uuid
  AND (sqlc.narg('requested_by')::uuid IS NULL OR (r.requested_by_kind = 'user' AND r.requested_by_id = sqlc.narg('requested_by')::uuid))
  AND r.created_at >= now() - interval '7 days';

-- name: ConnectedGrantsOf :many
SELECT g.id, c.name AS client_name, g.mode, g.created_at,
       w.seen_at AS worker_seen_at,
       EXISTS (
         SELECT 1 FROM agent_requests r
         WHERE r.status = 'running' AND r.claimed_by = 'grant:' || g.id::text AND r.lease_until > clock_timestamp()
       ) AS worker_working,
       (SELECT count(*) FROM activity a
        WHERE a.actor_kind = 'user' AND a.actor_id = g.user_id
          AND a.origin = 'mcp' AND a.origin_client IS NOT DISTINCT FROM c.name
          AND a.created_at >= now() - interval '7 days')::int AS writes_7d,
       clock_timestamp()::timestamptz AS checked_at
FROM oauth_grants g
JOIN oauth_clients c ON c.id = g.client_id
LEFT JOIN agent_workers w ON w.id = 'grant:' || g.id::text
WHERE g.user_id = @user_id
  AND g.revoked_at IS NULL
  AND g.expires_at > now()
ORDER BY g.created_at DESC, g.id DESC
LIMIT 100;

-- name: RunTakingServiceAccounts :many
SELECT s.id, s.name,
       w.seen_at AS worker_seen_at,
       EXISTS (
         SELECT 1 FROM agent_requests r
         WHERE r.status = 'running' AND r.claimed_by = 'service_account:' || s.id::text AND r.lease_until > clock_timestamp()
       ) AS worker_working,
       clock_timestamp()::timestamptz AS checked_at
FROM service_accounts s
LEFT JOIN agent_workers w ON w.id = 'service_account:' || s.id::text
WHERE s.takes_runs
ORDER BY s.name, s.id
LIMIT 100;

-- name: ListAgentRuns :many
SELECT r.id, r.source, r.title, r.prompt, r.state, r.result_kind, r.failure_code,
       r.created_at, r.started_at, r.finished_at,
       ((r.run_usage->>'cost_usd') IS NOT NULL)::boolean AS priced,
       coalesce((r.run_usage->>'cost_usd')::float8, 0)::float8 AS cost_usd,
       r.requested_by_kind, r.requested_by_id, r.requested_by_name,
       d.id AS visible_dashboard_id, d.title AS dashboard_title,
       o.id AS visible_observation_id, o.title AS observation_title
FROM agent_requests r
LEFT JOIN dashboards d ON d.id = r.dashboard_id AND d.folder_id = ANY(@folders::uuid[])
LEFT JOIN observations o ON o.id = r.observation_id AND o.folder_id = ANY(@folders::uuid[])
WHERE r.assigned_kind = @assigned_kind
  AND (sqlc.narg('assigned_id')::uuid IS NULL OR r.assigned_id = sqlc.narg('assigned_id')::uuid)
  AND r.source <> 'suggest'
  AND (sqlc.narg('requested_by')::uuid IS NULL OR (r.requested_by_kind = 'user' AND r.requested_by_id = sqlc.narg('requested_by')::uuid))
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (r.created_at, r.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY r.created_at DESC, r.id DESC
LIMIT @row_limit;

-- name: GrantOwnedBy :one
SELECT EXISTS (SELECT 1 FROM oauth_grants WHERE id = @id AND user_id = @user_id);

-- name: ServiceAccountExists :one
SELECT EXISTS (SELECT 1 FROM service_accounts WHERE id = @id);
