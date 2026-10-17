-- name: LockAgentRequestsForDashboard :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:dashboard:' || CAST(@dashboard_id::uuid AS text), 0));

-- name: LockAgentRequestClaims :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:claim', 0));

-- name: CountActiveAgentRequests :one
SELECT count(*) FROM agent_requests
WHERE dashboard_id = @dashboard_id AND status IN ('queued', 'running');

-- name: EnqueueAgentRequest :one
INSERT INTO agent_requests (
  dashboard_id, source, comment_id, prompt, panel_id, base_version,
  requested_by_kind, requested_by_id, requested_by_name, requested_by_email
) VALUES (
  @dashboard_id, @source, sqlc.narg('comment_id'), @prompt, sqlc.narg('panel_id'), @base_version,
  @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email')
)
RETURNING *;

-- name: ClaimAgentRequest :one
UPDATE agent_requests
SET status = 'running',
  attempts = attempts + 1,
  lease_until = clock_timestamp() + interval '5 minutes',
  started_at = coalesce(started_at, clock_timestamp())
WHERE id = (
  SELECT r.id FROM agent_requests r
  WHERE (r.status = 'queued' OR (r.status = 'running' AND r.lease_until < clock_timestamp()))
    AND r.attempts < 2
    AND NOT EXISTS (
      SELECT 1 FROM agent_requests o
      WHERE o.dashboard_id = r.dashboard_id AND o.status = 'running'
        AND o.lease_until >= clock_timestamp() AND o.id <> r.id
    )
  ORDER BY r.created_at, r.id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING *;

-- name: FailExhaustedAgentRequests :many
UPDATE agent_requests
SET status = 'failed', error = 'agent run did not finish', lease_until = NULL, finished_at = clock_timestamp()
WHERE status = 'running' AND lease_until < clock_timestamp() AND attempts >= 2
RETURNING *;

-- name: FinishAgentRequest :one
UPDATE agent_requests
SET status = @status::text,
  result_version = sqlc.narg('result_version')::integer,
  error = sqlc.narg('error')::text,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING *;

-- name: GetAgentRequest :one
SELECT * FROM agent_requests WHERE id = @id AND dashboard_id = @dashboard_id;

-- name: ListAgentRequests :many
SELECT * FROM agent_requests
WHERE dashboard_id = @dashboard_id
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: GetAgentRequestResultVersion :one
SELECT version FROM dashboard_versions WHERE dashboard_id = @dashboard_id AND agent_request_id = @agent_request_id;

-- name: LatestAgentRequestsForComments :many
SELECT DISTINCT ON (comment_id) comment_id, id, status
FROM agent_requests
WHERE comment_id = ANY(@comment_ids::uuid[])
ORDER BY comment_id, created_at DESC, id DESC;
