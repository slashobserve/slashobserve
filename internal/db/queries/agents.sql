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
