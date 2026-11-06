-- name: LockAgentRequestsForDashboard :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:dashboard:' || CAST(@dashboard_id::uuid AS text), 0));

-- name: LockAgentRequestClaims :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:claim', 0));

-- name: CountActiveAgentRequests :one
SELECT count(*) FROM agent_requests
WHERE dashboard_id = @dashboard_id AND source NOT IN ('suggest', 'investigate') AND status IN ('queued', 'running');

-- name: EnqueueAgentRequest :one
INSERT INTO agent_requests (
  dashboard_id, source, comment_id, prompt, panel_id, base_version,
  requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client,
  continues_id, continue_note
) VALUES (
  @dashboard_id, @source, sqlc.narg('comment_id'), @prompt, sqlc.narg('panel_id'), @base_version::integer,
  @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text,
  sqlc.narg('continues_id')::uuid, sqlc.narg('continue_note')::text
)
RETURNING *;

-- name: ClaimAgentRequest :one
UPDATE agent_requests
SET status = 'running',
  attempts = attempts + 1,
  lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision),
  started_at = coalesce(started_at, clock_timestamp())
WHERE id = (
  SELECT r.id FROM agent_requests r
  WHERE (r.status = 'queued' OR (r.status = 'running' AND r.lease_until < clock_timestamp()))
    AND r.attempts < @max_attempts::integer
    AND (r.dashboard_id IS NULL OR r.source IN ('suggest', 'investigate') OR NOT EXISTS (
      SELECT 1 FROM agent_requests o
      WHERE o.dashboard_id = r.dashboard_id AND o.status = 'running' AND o.source NOT IN ('suggest', 'investigate') AND o.id <> r.id
    ))
  ORDER BY r.created_at, r.id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING *;

-- name: RenewAgentRequestLease :one
UPDATE agent_requests
SET lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision)
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING cancel_requested_at;

-- name: FailExhaustedAgentRequests :many
UPDATE agent_requests r
SET status = CASE WHEN (saved.version IS NULL AND f.id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  result_version = saved.version,
  dashboard_id = coalesce(r.dashboard_id, saved.dashboard_id),
  error = CASE WHEN (saved.version IS NOT NULL OR f.id IS NOT NULL) THEN NULL WHEN r.cancel_requested_at IS NOT NULL THEN 'the run was canceled' ELSE 'agent run did not finish' END,
  failure_code = CASE WHEN (saved.version IS NOT NULL OR f.id IS NOT NULL) THEN NULL WHEN r.cancel_requested_at IS NOT NULL THEN 'canceled' ELSE 'lease_expired' END,
  lease_until = NULL,
  finished_at = clock_timestamp()
FROM agent_requests e
LEFT JOIN dashboard_versions saved ON saved.agent_request_id = e.id AND (e.dashboard_id IS NULL OR saved.dashboard_id = e.dashboard_id)
LEFT JOIN findings f ON f.agent_request_id = e.id
WHERE r.id = e.id AND r.status = 'running' AND r.lease_until < clock_timestamp() AND r.attempts >= @max_attempts::integer
RETURNING r.*;

-- name: CancelAgentRequest :one
WITH target AS (
  SELECT r.id, saved.version AS saved_version, saved.dashboard_id AS saved_dashboard_id, f.id AS finding_id,
    (r.status = 'queued' OR r.lease_until < clock_timestamp()) AS finishes
  FROM agent_requests r
  LEFT JOIN dashboard_versions saved ON saved.agent_request_id = r.id AND (r.dashboard_id IS NULL OR saved.dashboard_id = r.dashboard_id)
  LEFT JOIN findings f ON f.agent_request_id = r.id
  WHERE r.id = @id AND r.status IN ('queued', 'running') AND r.source <> 'suggest'
  FOR UPDATE OF r
)
UPDATE agent_requests r
SET cancel_requested_at = coalesce(r.cancel_requested_at, clock_timestamp()),
  canceled_by_kind = coalesce(r.canceled_by_kind, @canceled_by_kind),
  canceled_by_id = coalesce(r.canceled_by_id, @canceled_by_id),
  canceled_by_name = coalesce(r.canceled_by_name, @canceled_by_name),
  status = CASE WHEN NOT t.finishes THEN r.status WHEN (t.saved_version IS NULL AND t.finding_id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  result_version = CASE WHEN t.finishes THEN t.saved_version ELSE r.result_version END,
  dashboard_id = CASE WHEN t.finishes THEN coalesce(r.dashboard_id, t.saved_dashboard_id) ELSE r.dashboard_id END,
  error = CASE WHEN NOT t.finishes THEN r.error WHEN (t.saved_version IS NULL AND t.finding_id IS NULL) THEN @message::text END,
  failure_code = CASE WHEN NOT t.finishes THEN r.failure_code WHEN (t.saved_version IS NULL AND t.finding_id IS NULL) THEN 'canceled' END,
  lease_until = CASE WHEN t.finishes THEN NULL ELSE r.lease_until END,
  finished_at = CASE WHEN t.finishes THEN clock_timestamp() ELSE r.finished_at END
FROM target t
WHERE r.id = t.id
RETURNING r.*;

-- name: CountQueuedAgentRequestsBefore :one
SELECT count(*) FROM agent_requests
WHERE status = 'queued' AND (created_at, id) < (@created_at::timestamptz, @id::uuid);

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
  failure_code = sqlc.narg('failure_code')::text,
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING *;

-- name: ReleaseAgentRequest :one
UPDATE agent_requests
SET status = 'queued', attempts = attempts - 1, lease_until = NULL,
  check_failures = check_failures + CASE WHEN @check_failed::boolean THEN 1 ELSE 0 END
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING *;

-- name: GetAgentRequest :one
SELECT * FROM agent_requests WHERE id = @id AND dashboard_id = @dashboard_id AND source NOT IN ('suggest', 'investigate');

-- name: ListAgentRequests :many
SELECT * FROM agent_requests
WHERE dashboard_id = @dashboard_id AND source NOT IN ('suggest', 'investigate')
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
INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client, continues_id, continue_note)
VALUES ('observe', @prompt, @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('continues_id')::uuid, sqlc.narg('continue_note')::text)
RETURNING *;

-- name: GetAgentRequestByID :one
SELECT * FROM agent_requests WHERE id = @id;

-- name: GetAgentRequestSavedVersion :one
SELECT dashboard_id, version FROM dashboard_versions
WHERE agent_request_id = @agent_request_id
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR dashboard_id = sqlc.narg('dashboard_id')::uuid);

-- name: AppendAgentRequestStep :execrows
UPDATE agent_requests
SET steps = (
  SELECT COALESCE(jsonb_agg(ranked.recorded ORDER BY ranked.n), '[]'::jsonb)
  FROM (
    SELECT numbered.recorded, numbered.n,
      CASE WHEN (numbered.recorded ->> 'attempt')::integer = @attempts::integer
        THEN count(*) FILTER (WHERE (numbered.recorded ->> 'attempt')::integer = @attempts::integer)
          OVER (ORDER BY numbered.n DESC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
      END AS recent
    FROM (
      SELECT recorded, n
      FROM jsonb_array_elements(agent_requests.steps) WITH ORDINALITY AS elements(recorded, n)
    ) numbered
  ) ranked
  WHERE ranked.recent IS NULL OR ranked.recent < @max_steps::integer
) || jsonb_build_array(@step::jsonb)
WHERE id = @id AND status = 'running' AND attempts = @attempts::integer;

-- name: ListAgentRequestsBy :many
SELECT r.id, r.source, r.prompt, r.dashboard_id, r.panel_id, r.status, r.result_version, r.error,
  r.origin, r.origin_client, r.created_at, r.started_at, r.finished_at,
  r.failure_code, r.build_saved_at, r.continues_id, r.continue_note,
  r.title, r.pinned_at,
  d.title AS dashboard_title,
  f.id AS finding_id
FROM agent_requests r
LEFT JOIN dashboards d ON d.id = r.dashboard_id AND d.folder_id = ANY(@folders::uuid[])
LEFT JOIN findings f ON f.agent_request_id = r.id
WHERE r.requested_by_kind = @requested_by_kind AND r.requested_by_id = @requested_by_id AND r.source <> 'suggest'
  AND r.archived_at IS NULL
  AND (sqlc.narg('source')::text IS NULL OR r.source = sqlc.narg('source')::text)
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR r.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('pinned')::boolean IS NULL OR (r.pinned_at IS NOT NULL) = sqlc.narg('pinned')::boolean)
  AND (sqlc.narg('pattern')::text IS NULL OR r.title ILIKE sqlc.narg('pattern')::text OR r.prompt ILIKE sqlc.narg('pattern')::text)
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR ((r.pinned_at IS NOT NULL) = @cursor_pinned::boolean
      AND (r.created_at, r.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
    OR (@cursor_pinned::boolean AND r.pinned_at IS NULL))
ORDER BY (r.pinned_at IS NOT NULL) DESC, r.created_at DESC, r.id DESC
LIMIT @row_limit;

-- name: LockSuggestRequests :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:suggest', 0));

-- name: CountActiveSuggestRequestsBy :one
SELECT count(*) FROM agent_requests
WHERE source = 'suggest' AND status IN ('queued', 'running') AND requested_by_id = @requested_by_id;

-- name: CountActiveSuggestRequests :one
SELECT count(*) FROM agent_requests WHERE source = 'suggest' AND status IN ('queued', 'running');

-- name: EnqueueSuggestRequest :one
INSERT INTO agent_requests (
  dashboard_id, source, prompt, panel_id, base_version, draft,
  requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client
) VALUES (
  @dashboard_id, 'suggest', @prompt, @panel_id, @base_version::integer, sqlc.narg('draft')::jsonb,
  @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text
)
RETURNING *;

-- name: SucceedSuggestRequest :one
UPDATE agent_requests
SET status = 'succeeded',
  suggestion = @suggestion::jsonb,
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING *;

-- name: GetSuggestRequest :one
SELECT * FROM agent_requests WHERE id = @id AND dashboard_id = @dashboard_id AND source = 'suggest';

-- name: EnqueueInvestigateRequest :one
INSERT INTO agent_requests (
  dashboard_id, source, prompt, panel_id, context_from, context_to,
  requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client
) VALUES (
  sqlc.narg('dashboard_id')::uuid, 'investigate', @prompt, sqlc.narg('panel_id')::text,
  sqlc.narg('context_from')::timestamptz, sqlc.narg('context_to')::timestamptz,
  @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text
)
RETURNING *;

-- name: CountActiveInvestigateRequestsBy :one
SELECT count(*) FROM agent_requests
WHERE source = 'investigate' AND status IN ('queued', 'running') AND requested_by_id = @requested_by_id;

-- name: CountActiveInvestigateRequests :one
SELECT count(*) FROM agent_requests WHERE source = 'investigate' AND status IN ('queued', 'running');

-- name: SucceedFindingRequest :one
UPDATE agent_requests
SET status = 'succeeded',
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE agent_requests.id = @id AND agent_requests.status = 'running' AND agent_requests.attempts = @attempts
  AND (agent_requests.source = 'investigate'
    OR (agent_requests.source = 'observe' AND agent_requests.dashboard_id IS NULL)
    OR (agent_requests.source IN ('prompt', 'comment') AND agent_requests.dashboard_id IS NOT NULL))
  AND EXISTS (SELECT 1 FROM findings f WHERE f.agent_request_id = agent_requests.id)
RETURNING *;

-- name: UpdateAgentRequestListFields :one
UPDATE agent_requests
SET title = CASE WHEN @set_title::boolean THEN sqlc.narg('title')::text ELSE title END,
  pinned_at = CASE WHEN sqlc.narg('pinned')::boolean IS NULL THEN pinned_at
    WHEN sqlc.narg('pinned')::boolean THEN coalesce(pinned_at, clock_timestamp())
    ELSE NULL END
WHERE id = @id
RETURNING *;

-- name: ArchiveAgentRequest :execrows
UPDATE agent_requests SET archived_at = coalesce(archived_at, clock_timestamp()) WHERE id = @id;
