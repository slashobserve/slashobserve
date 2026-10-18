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
  @dashboard_id, @source, sqlc.narg('comment_id'), @prompt, sqlc.narg('panel_id'), @base_version::integer,
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
    AND r.attempts < @max_attempts::integer
    AND (r.dashboard_id IS NULL OR NOT EXISTS (
      SELECT 1 FROM agent_requests o
      WHERE o.dashboard_id = r.dashboard_id AND o.status = 'running' AND o.id <> r.id
    ))
  ORDER BY r.created_at, r.id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING *;

-- name: FailExhaustedAgentRequests :many
UPDATE agent_requests r
SET status = CASE WHEN saved.version IS NULL THEN 'failed' ELSE 'succeeded' END,
  result_version = saved.version,
  dashboard_id = coalesce(r.dashboard_id, saved.dashboard_id),
  error = CASE WHEN saved.version IS NULL THEN 'agent run did not finish' END,
  lease_until = NULL,
  finished_at = clock_timestamp()
FROM agent_requests e
LEFT JOIN dashboard_versions saved ON saved.agent_request_id = e.id AND (e.dashboard_id IS NULL OR saved.dashboard_id = e.dashboard_id)
WHERE r.id = e.id AND r.status = 'running' AND r.lease_until < clock_timestamp() AND r.attempts >= @max_attempts::integer
RETURNING r.*;

-- name: SucceedAgentRequest :one
UPDATE agent_requests
SET status = 'succeeded',
  result_version = @result_version::integer,
  run_usage = sqlc.narg('run_usage')::jsonb,
  dashboard_id = coalesce(dashboard_id, (SELECT v.dashboard_id FROM dashboard_versions v WHERE v.agent_request_id = @id)),
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING *;

-- name: FailAgentRequest :one
UPDATE agent_requests
SET status = 'failed',
  error = @error::text,
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING *;

-- name: ReleaseAgentRequest :one
UPDATE agent_requests
SET status = 'queued', attempts = attempts - 1, lease_until = NULL,
  steps = CASE WHEN @check_failed::boolean THEN steps
    ELSE jsonb_path_query_array(steps, '$[*] ? (@.attempt != $attempt)', jsonb_build_object('attempt', @attempts::integer)) END,
  check_failures = check_failures + CASE WHEN @check_failed::boolean THEN 1 ELSE 0 END
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING *;

-- name: GetAgentRequest :one
SELECT * FROM agent_requests WHERE id = @id AND dashboard_id = @dashboard_id AND source <> 'observe';

-- name: ListAgentRequests :many
SELECT * FROM agent_requests
WHERE dashboard_id = @dashboard_id AND source <> 'observe'
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: LatestAgentRequestsForComments :many
SELECT DISTINCT ON (comment_id) comment_id, id, status
FROM agent_requests
WHERE comment_id = ANY(@comment_ids::uuid[])
ORDER BY comment_id, created_at DESC, id DESC;

-- name: LockObserveRequests :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:observe', 0));

-- name: CountActiveObserveRequestsBy :one
SELECT count(*) FROM agent_requests
WHERE source = 'observe' AND status IN ('queued', 'running') AND requested_by_id = @requested_by_id;

-- name: CountActiveObserveRequests :one
SELECT count(*) FROM agent_requests WHERE source = 'observe' AND status IN ('queued', 'running');

-- name: EnqueueObserveRequest :one
INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, requested_by_email)
VALUES ('observe', @prompt, @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'))
RETURNING *;

-- name: GetAgentRequestByID :one
SELECT * FROM agent_requests WHERE id = @id;

-- name: GetAgentRequestSavedVersion :one
SELECT dashboard_id, version FROM dashboard_versions
WHERE agent_request_id = @agent_request_id
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR dashboard_id = sqlc.narg('dashboard_id')::uuid);

-- name: AppendAgentRequestStep :execrows
UPDATE agent_requests
SET steps = steps || jsonb_build_array(@step::jsonb)
WHERE id = @id AND status = 'running' AND attempts = @attempts
  AND (SELECT count(*) FROM jsonb_array_elements(steps) AS recorded WHERE (recorded ->> 'attempt')::integer = @attempts) < @max_steps::integer;
