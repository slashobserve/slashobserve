-- name: LockAgentRequestsForDashboard :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:dashboard:' || CAST(@dashboard_id::uuid AS text), 0));

-- name: LockAgentRequestClaims :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:claim', 0));

-- name: CountActiveAgentRequests :one
SELECT count(*) FROM agent_requests
WHERE dashboard_id = @dashboard_id AND intent = 'apply' AND status IN ('queued', 'running');

-- name: EnqueueAgentRequest :one
INSERT INTO agent_requests (
  id, dashboard_id, intent, kind, comment_id, prompt, panel_id, base_version,
  requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client,
  continues_id, continue_note, retries_id,
  assigned_kind, assigned_id, trigger, thread_message_id, thread_context, time_zone, as_of,
  chat_id, anchor, block_id, client_id, applies_finding_id
) VALUES (
  coalesce(sqlc.narg('id')::uuid, gen_random_uuid()), @dashboard_id, @intent, 'build', sqlc.narg('comment_id'), @prompt, sqlc.narg('panel_id'), @base_version::integer,
  @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text,
  sqlc.narg('continues_id')::uuid, sqlc.narg('continue_note')::text, sqlc.narg('retries_id')::uuid,
  coalesce(sqlc.narg('assigned_kind')::text, 'builtin'), sqlc.narg('assigned_id')::uuid, coalesce(sqlc.narg('trigger')::text, 'person'),
  sqlc.narg('thread_message_id')::uuid, sqlc.narg('thread_context')::text, sqlc.narg('time_zone')::text, sqlc.narg('as_of')::timestamptz,
  sqlc.narg('chat_id')::uuid, sqlc.narg('anchor')::jsonb, sqlc.narg('block_id')::uuid, sqlc.narg('client_id')::uuid,
  sqlc.narg('applies_finding_id')::uuid
)
ON CONFLICT (requested_by_kind, requested_by_id, client_id) WHERE client_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetRunApplyingFinding :one
SELECT * FROM agent_requests
WHERE applies_finding_id = @finding_id AND status IN ('queued', 'running', 'succeeded');

-- name: ClaimAgentRequest :one
WITH running AS (
  SELECT b.requested_by_kind, b.requested_by_id, count(*) AS runs FROM agent_requests b
  WHERE b.status = 'running' AND b.lease_until >= clock_timestamp()
  GROUP BY b.requested_by_kind, b.requested_by_id
), picked AS (
  SELECT r.id, (r.started_at IS NULL) AS first_claim FROM agent_requests r
  LEFT JOIN running ON running.requested_by_kind = r.requested_by_kind AND running.requested_by_id = r.requested_by_id
  WHERE r.status IN ('queued', 'running')
    AND r.state = ANY(@from_states::text[])
    AND (r.state = 'queued' OR r.lease_until < clock_timestamp())
    AND r.cancel_requested_at IS NULL
    AND r.attempts < @max_attempts::integer
    AND r.assigned_kind = @assigned_kind::text
    AND r.assigned_id IS NOT DISTINCT FROM sqlc.narg('assigned_id')::uuid
    AND (r.dashboard_id IS NULL OR r.intent <> 'apply' OR NOT EXISTS (
      SELECT 1 FROM agent_requests o
      WHERE o.dashboard_id = r.dashboard_id AND o.status = 'running' AND o.intent = 'apply' AND o.id <> r.id
    ))
    AND (r.chat_id IS NULL OR NOT EXISTS (
      SELECT 1 FROM agent_requests e
      WHERE e.chat_id = r.chat_id AND e.status IN ('queued', 'running') AND (e.created_at, e.id) < (r.created_at, r.id)
        AND NOT (e.state = 'queued' AND e.cancel_requested_at IS NOT NULL)
    ))
  ORDER BY coalesce(running.runs, 0), r.created_at, r.id
  FOR UPDATE OF r SKIP LOCKED
  LIMIT 1
)
UPDATE agent_requests
SET state = 'running',
  status = 'running',
  attempts = attempts + 1,
  lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision),
  started_at = coalesce(started_at, clock_timestamp()),
  claimed_by = @claimed_by::text
FROM picked
WHERE agent_requests.id = picked.id
RETURNING sqlc.embed(agent_requests), picked.first_claim::boolean AS first_claim;

-- name: RenewAgentRequestLease :one
UPDATE agent_requests
SET lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision)
WHERE id = @id AND status = 'running' AND attempts = @attempts
RETURNING cancel_requested_at;

-- name: FailExhaustedAgentRequests :many
UPDATE agent_requests r
SET state = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  status = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  result_kind = CASE WHEN f.id IS NOT NULL THEN 'observation' WHEN saved.version IS NOT NULL THEN 'version' WHEN pr.id IS NOT NULL THEN 'proposal' ELSE r.result_kind END,
  result_version = saved.version,
  intent = CASE WHEN saved.version IS NOT NULL THEN 'apply' ELSE r.intent END,
  dashboard_id = coalesce(r.dashboard_id, saved.dashboard_id),
  error = CASE WHEN (saved.version IS NOT NULL OR f.id IS NOT NULL OR pr.id IS NOT NULL) THEN NULL ELSE 'agent run did not finish' END,
  failure_code = CASE WHEN (saved.version IS NOT NULL OR f.id IS NOT NULL OR pr.id IS NOT NULL) THEN NULL ELSE 'lease_expired' END,
  lease_until = NULL,
  finished_at = clock_timestamp()
FROM agent_requests e
LEFT JOIN dashboard_versions saved ON saved.agent_request_id = e.id AND (e.dashboard_id IS NULL OR saved.dashboard_id = e.dashboard_id)
LEFT JOIN findings f ON f.agent_request_id = e.id
LEFT JOIN LATERAL (SELECT p.id FROM proposals p WHERE p.run_id = e.id LIMIT 1) pr ON true
WHERE r.id = e.id AND r.status = 'running' AND r.state = ANY(@from_states::text[])
  AND r.lease_until < clock_timestamp() AND r.attempts >= @max_attempts::integer
RETURNING r.*;

-- name: SettleCanceledAgentRequests :many
UPDATE agent_requests r
SET state = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN 'canceled' ELSE 'succeeded' END,
  status = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  result_kind = CASE WHEN f.id IS NOT NULL THEN 'observation' WHEN saved.version IS NOT NULL THEN 'version' WHEN pr.id IS NOT NULL THEN 'proposal' ELSE r.result_kind END,
  result_version = saved.version,
  intent = CASE WHEN saved.version IS NOT NULL THEN 'apply' ELSE r.intent END,
  dashboard_id = coalesce(r.dashboard_id, saved.dashboard_id),
  error = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN @message::text END,
  failure_code = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN 'canceled' END,
  lease_until = NULL,
  finished_at = clock_timestamp()
FROM agent_requests e
LEFT JOIN dashboard_versions saved ON saved.agent_request_id = e.id AND (e.dashboard_id IS NULL OR saved.dashboard_id = e.dashboard_id)
LEFT JOIN findings f ON f.agent_request_id = e.id
LEFT JOIN LATERAL (SELECT p.id FROM proposals p WHERE p.run_id = e.id LIMIT 1) pr ON true
WHERE r.id = e.id AND r.status IN ('queued', 'running') AND r.state = ANY(@from_states::text[])
  AND r.cancel_requested_at IS NOT NULL
  AND (r.state = 'queued' OR r.lease_until IS NULL OR r.lease_until < clock_timestamp())
RETURNING r.*;

-- name: SettleAgentRequest :one
WITH target AS (
  SELECT r.id, saved.version AS saved_version, saved.dashboard_id AS saved_dashboard_id, f.id AS finding_id, pr.id AS proposal_id
  FROM agent_requests r
  LEFT JOIN dashboard_versions saved ON saved.agent_request_id = r.id AND (r.dashboard_id IS NULL OR saved.dashboard_id = r.dashboard_id)
  LEFT JOIN findings f ON f.agent_request_id = r.id
  LEFT JOIN LATERAL (SELECT p.id FROM proposals p WHERE p.run_id = r.id LIMIT 1) pr ON true
  WHERE r.id = @id AND r.attempts = @attempts AND r.state = ANY(@from_states::text[]) AND r.cancel_requested_at IS NOT NULL
  FOR NO KEY UPDATE OF r
)
UPDATE agent_requests
SET state = CASE WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN 'canceled' ELSE 'succeeded' END,
  status = CASE WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  result_kind = CASE WHEN t.finding_id IS NOT NULL THEN 'observation' WHEN t.saved_version IS NOT NULL THEN 'version' WHEN t.proposal_id IS NOT NULL THEN 'proposal' ELSE agent_requests.result_kind END,
  result_version = t.saved_version,
  intent = CASE WHEN t.saved_version IS NOT NULL THEN 'apply' ELSE agent_requests.intent END,
  dashboard_id = coalesce(agent_requests.dashboard_id, t.saved_dashboard_id),
  error = CASE WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN @message::text END,
  failure_code = CASE WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN 'canceled' END,
  run_usage = coalesce(sqlc.narg('run_usage')::jsonb, agent_requests.run_usage),
  lease_until = NULL,
  finished_at = clock_timestamp()
FROM target t
WHERE agent_requests.id = t.id
RETURNING agent_requests.*;

-- name: CancelAgentRequest :one
WITH target AS (
  SELECT r.id, r.state AS prior_state, saved.version AS saved_version, saved.dashboard_id AS saved_dashboard_id, f.id AS finding_id, pr.id AS proposal_id,
    (r.state = 'queued' OR r.lease_until IS NULL OR r.lease_until < clock_timestamp()) AS finishes
  FROM agent_requests r
  LEFT JOIN dashboard_versions saved ON saved.agent_request_id = r.id AND (r.dashboard_id IS NULL OR saved.dashboard_id = r.dashboard_id)
  LEFT JOIN findings f ON f.agent_request_id = r.id
  LEFT JOIN LATERAL (SELECT p.id FROM proposals p WHERE p.run_id = r.id LIMIT 1) pr ON true
  WHERE r.id = @id AND r.state = ANY(@from_states::text[]) AND r.intent <> 'suggest'
  FOR UPDATE OF r
)
UPDATE agent_requests
SET cancel_requested_at = coalesce(agent_requests.cancel_requested_at, clock_timestamp()),
  canceled_by_kind = coalesce(agent_requests.canceled_by_kind, @canceled_by_kind),
  canceled_by_id = coalesce(agent_requests.canceled_by_id, @canceled_by_id),
  canceled_by_name = coalesce(agent_requests.canceled_by_name, @canceled_by_name),
  state = CASE WHEN NOT t.finishes THEN 'canceling' WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN 'canceled' ELSE 'succeeded' END,
  status = CASE WHEN NOT t.finishes THEN 'running' WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  result_kind = CASE WHEN NOT t.finishes THEN agent_requests.result_kind WHEN t.finding_id IS NOT NULL THEN 'observation' WHEN t.saved_version IS NOT NULL THEN 'version' WHEN t.proposal_id IS NOT NULL THEN 'proposal' ELSE agent_requests.result_kind END,
  result_version = CASE WHEN t.finishes THEN t.saved_version ELSE agent_requests.result_version END,
  intent = CASE WHEN t.finishes AND t.saved_version IS NOT NULL THEN 'apply' ELSE agent_requests.intent END,
  dashboard_id = CASE WHEN t.finishes THEN coalesce(agent_requests.dashboard_id, t.saved_dashboard_id) ELSE agent_requests.dashboard_id END,
  error = CASE WHEN NOT t.finishes THEN agent_requests.error WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN @message::text END,
  failure_code = CASE WHEN NOT t.finishes THEN agent_requests.failure_code WHEN (t.saved_version IS NULL AND t.finding_id IS NULL AND t.proposal_id IS NULL) THEN 'canceled' END,
  lease_until = CASE WHEN t.finishes THEN NULL ELSE agent_requests.lease_until END,
  finished_at = CASE WHEN t.finishes THEN clock_timestamp() ELSE agent_requests.finished_at END
FROM target t
WHERE agent_requests.id = t.id
RETURNING sqlc.embed(agent_requests), t.prior_state::text AS prior_state;

-- name: CountQueuedAgentRequestsBefore :one
SELECT count(*) FROM agent_requests
WHERE status = 'queued' AND (created_at, id) < (@created_at::timestamptz, @id::uuid);

-- name: SucceedAgentRequest :one
UPDATE agent_requests
SET state = 'succeeded',
  status = 'succeeded',
  result_kind = CASE WHEN EXISTS (SELECT 1 FROM findings f WHERE f.agent_request_id = @id) THEN 'observation' ELSE 'version' END,
  result_version = @result_version::integer,
  intent = 'apply',
  run_usage = sqlc.narg('run_usage')::jsonb,
  dashboard_id = coalesce(dashboard_id, (SELECT v.dashboard_id FROM dashboard_versions v WHERE v.agent_request_id = @id)),
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND state = ANY(@from_states::text[]) AND attempts = @attempts
RETURNING *;

-- name: FailAgentRequest :one
UPDATE agent_requests
SET state = 'failed',
  status = 'failed',
  error = @error::text,
  failure_code = sqlc.narg('failure_code')::text,
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND state = ANY(@from_states::text[]) AND attempts = @attempts
RETURNING *;

-- name: ReleaseAgentRequest :one
UPDATE agent_requests
SET state = 'queued', status = 'queued', attempts = attempts - 1, lease_until = NULL,
  check_failures = check_failures + CASE WHEN @check_failed::boolean THEN 1 ELSE 0 END
WHERE id = @id AND state = ANY(@from_states::text[]) AND attempts = @attempts
RETURNING *;

-- name: GetAgentRequest :one
SELECT * FROM agent_requests WHERE id = @id AND dashboard_id = @dashboard_id AND intent = 'apply';

-- name: ListAgentRequests :many
SELECT * FROM agent_requests
WHERE dashboard_id = @dashboard_id AND intent = 'apply'
  AND (@reader_admin::boolean OR (requested_by_kind = @reader_kind AND requested_by_id = @reader_id::uuid) OR NOT run_answer_kept_with_admins(id, '{}'))
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
WHERE kind = 'build' AND intent = 'ask' AND status IN ('queued', 'running') AND requested_by_id = @requested_by_id
  AND (chat_id IS NULL OR NOT EXISTS (SELECT 1 FROM chats c WHERE c.id = chat_id AND c.observation_id IS NOT NULL));

-- name: CountActiveObserveRequests :one
SELECT count(*) FROM agent_requests WHERE kind = 'build' AND intent = 'ask' AND status IN ('queued', 'running')
  AND (chat_id IS NULL OR NOT EXISTS (SELECT 1 FROM chats c WHERE c.id = chat_id AND c.observation_id IS NOT NULL));

-- name: EnqueueObserveRequest :one
INSERT INTO agent_requests (id, intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client, continues_id, continue_note, retries_id, chat_id, thread_context, time_zone, as_of, client_id)
VALUES (coalesce(sqlc.narg('id')::uuid, gen_random_uuid()), 'ask', 'build', @prompt, @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('continues_id')::uuid, sqlc.narg('continue_note')::text, sqlc.narg('retries_id')::uuid,
  sqlc.narg('chat_id')::uuid, sqlc.narg('thread_context')::text, sqlc.narg('time_zone')::text, sqlc.narg('as_of')::timestamptz, sqlc.narg('client_id')::uuid)
ON CONFLICT (requested_by_kind, requested_by_id, client_id) WHERE client_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetAgentRequestByID :one
SELECT * FROM agent_requests WHERE id = @id;

-- name: GetAgentRequestByClient :one
SELECT * FROM agent_requests
WHERE requested_by_kind = @requested_by_kind AND requested_by_id = @requested_by_id AND client_id = @client_id;

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

-- name: SeedAgentRequestStep :execrows
UPDATE agent_requests
SET steps = steps || jsonb_build_array(@step::jsonb)
WHERE id = @id AND status = 'queued';

-- name: ListAgentRequestsBy :many
SELECT r.id, r.intent, r.kind, r.investigation, r.trigger, r.prompt, r.dashboard_id, r.panel_id, r.status, r.state, r.result_kind, r.attempts, r.result_version, r.error,
  r.origin, r.origin_client, r.created_at, r.started_at, r.finished_at,
  r.failure_code, r.build_saved_at, r.continues_id, r.continue_note,
  r.title, r.pinned_at, r.observation_id, r.chat_id,
  r.base_version, r.comment_id, r.thread_message_id, r.subject_observation_id,
  r.requested_by_kind, r.requested_by_id, r.requested_by_name, r.anchor, r.block_id,
  ch.observation_id AS chat_observation_id,
  d.title AS dashboard_title,
  o.title AS observation_title,
  f.id AS finding_id
FROM agent_requests r
LEFT JOIN chats ch ON ch.id = r.chat_id
LEFT JOIN dashboards d ON d.id = r.dashboard_id AND d.folder_id = ANY(sqlc.arg('folders')::uuid[])
LEFT JOIN observations o ON o.id = r.observation_id AND o.folder_id = ANY(sqlc.arg('folders')::uuid[])
LEFT JOIN findings f ON f.agent_request_id = r.id AND o.id IS NOT NULL
WHERE r.requested_by_kind = @requested_by_kind AND r.requested_by_id = @requested_by_id AND r.intent <> 'suggest'
  AND r.archived_at IS NULL
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR r.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('chat_id')::uuid IS NULL OR r.chat_id = sqlc.narg('chat_id')::uuid OR r.id = sqlc.narg('chat_id')::uuid)
  AND (sqlc.narg('observation_id')::uuid IS NULL OR r.subject_observation_id = sqlc.narg('observation_id')::uuid OR r.observation_id = sqlc.narg('observation_id')::uuid)
  AND (NOT sqlc.arg('anchored')::boolean OR r.anchor IS NOT NULL)
  AND (sqlc.narg('pinned')::boolean IS NULL OR (r.pinned_at IS NOT NULL) = sqlc.narg('pinned')::boolean)
  AND (sqlc.narg('pattern')::text IS NULL OR r.title ILIKE sqlc.narg('pattern')::text OR r.prompt ILIKE sqlc.narg('pattern')::text OR EXISTS (
    SELECT 1 FROM findings sf
    WHERE sf.agent_request_id = r.id AND sf.markdown ILIKE sqlc.narg('pattern')::text
      AND CASE WHEN sf.observation_id IS NULL
        THEN sf.dashboard_id IS NULL OR EXISTS (SELECT 1 FROM dashboards sd WHERE sd.id = sf.dashboard_id AND sd.folder_id = ANY(sqlc.arg('folders')::uuid[]))
        ELSE EXISTS (SELECT 1 FROM observations so WHERE so.id = sf.observation_id AND so.folder_id = ANY(sqlc.arg('folders')::uuid[]))
      END))
  AND (sqlc.narg('states')::text[] IS NULL OR r.state = ANY(sqlc.narg('states')::text[]))
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR ((r.pinned_at IS NOT NULL) = @cursor_pinned::boolean
      AND (r.created_at, r.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
    OR (@cursor_pinned::boolean AND r.pinned_at IS NULL))
ORDER BY (r.pinned_at IS NOT NULL) DESC, r.created_at DESC, r.id DESC
LIMIT @row_limit;

-- name: ListAgentRequestsAll :many
SELECT r.id, r.intent, r.kind, r.investigation, r.trigger, r.prompt, r.dashboard_id, r.panel_id, r.status, r.state, r.result_kind, r.attempts, r.result_version, r.error,
  r.origin, r.origin_client, r.created_at, r.started_at, r.finished_at,
  r.failure_code, r.build_saved_at, r.continues_id, r.continue_note,
  r.title, r.pinned_at, r.observation_id, r.chat_id,
  r.base_version, r.comment_id, r.thread_message_id, r.subject_observation_id,
  r.requested_by_kind, r.requested_by_id, r.requested_by_name, r.anchor, r.block_id,
  ch.observation_id AS chat_observation_id,
  d.title AS dashboard_title,
  o.title AS observation_title,
  f.id AS finding_id
FROM agent_requests r
LEFT JOIN chats ch ON ch.id = r.chat_id
LEFT JOIN dashboards d ON d.id = r.dashboard_id AND d.folder_id = ANY(sqlc.arg('folders')::uuid[])
LEFT JOIN observations o ON o.id = r.observation_id AND o.folder_id = ANY(sqlc.arg('folders')::uuid[])
LEFT JOIN findings f ON f.agent_request_id = r.id AND o.id IS NOT NULL
WHERE r.intent <> 'suggest'
  AND r.archived_at IS NULL
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR r.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('chat_id')::uuid IS NULL OR r.chat_id = sqlc.narg('chat_id')::uuid OR r.id = sqlc.narg('chat_id')::uuid)
  AND (sqlc.narg('observation_id')::uuid IS NULL OR r.subject_observation_id = sqlc.narg('observation_id')::uuid OR r.observation_id = sqlc.narg('observation_id')::uuid)
  AND (sqlc.narg('pinned')::boolean IS NULL OR (r.pinned_at IS NOT NULL) = sqlc.narg('pinned')::boolean)
  AND (sqlc.narg('pattern')::text IS NULL OR r.title ILIKE sqlc.narg('pattern')::text OR r.prompt ILIKE sqlc.narg('pattern')::text OR EXISTS (
    SELECT 1 FROM findings sf
    WHERE sf.agent_request_id = r.id AND sf.markdown ILIKE sqlc.narg('pattern')::text
      AND CASE WHEN sf.observation_id IS NULL
        THEN sf.dashboard_id IS NULL OR EXISTS (SELECT 1 FROM dashboards sd WHERE sd.id = sf.dashboard_id AND sd.folder_id = ANY(sqlc.arg('folders')::uuid[]))
        ELSE EXISTS (SELECT 1 FROM observations so WHERE so.id = sf.observation_id AND so.folder_id = ANY(sqlc.arg('folders')::uuid[]))
      END))
  AND (sqlc.narg('states')::text[] IS NULL OR r.state = ANY(sqlc.narg('states')::text[]))
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (r.created_at, r.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY r.created_at DESC, r.id DESC
LIMIT @row_limit;

-- name: CountAgentRequestsBy :one
SELECT count(*) AS total
FROM agent_requests r
WHERE r.requested_by_kind = @requested_by_kind AND r.requested_by_id = @requested_by_id AND r.intent <> 'suggest'
  AND r.archived_at IS NULL
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR r.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('chat_id')::uuid IS NULL OR r.chat_id = sqlc.narg('chat_id')::uuid OR r.id = sqlc.narg('chat_id')::uuid)
  AND (sqlc.narg('observation_id')::uuid IS NULL OR r.subject_observation_id = sqlc.narg('observation_id')::uuid OR r.observation_id = sqlc.narg('observation_id')::uuid)
  AND (NOT sqlc.arg('anchored')::boolean OR r.anchor IS NOT NULL)
  AND (sqlc.narg('pinned')::boolean IS NULL OR (r.pinned_at IS NOT NULL) = sqlc.narg('pinned')::boolean)
  AND (sqlc.narg('pattern')::text IS NULL OR r.title ILIKE sqlc.narg('pattern')::text OR r.prompt ILIKE sqlc.narg('pattern')::text OR EXISTS (
    SELECT 1 FROM findings sf
    WHERE sf.agent_request_id = r.id AND sf.markdown ILIKE sqlc.narg('pattern')::text
      AND CASE WHEN sf.observation_id IS NULL
        THEN sf.dashboard_id IS NULL OR EXISTS (SELECT 1 FROM dashboards sd WHERE sd.id = sf.dashboard_id AND sd.folder_id = ANY(sqlc.arg('folders')::uuid[]))
        ELSE EXISTS (SELECT 1 FROM observations so WHERE so.id = sf.observation_id AND so.folder_id = ANY(sqlc.arg('folders')::uuid[]))
      END))
  AND (sqlc.narg('states')::text[] IS NULL OR r.state = ANY(sqlc.narg('states')::text[]));

-- name: CountAgentRequestsAll :one
SELECT count(*) AS total, count(DISTINCT (r.requested_by_kind, r.requested_by_id)) AS people
FROM agent_requests r
WHERE r.intent <> 'suggest'
  AND r.archived_at IS NULL
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR r.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('chat_id')::uuid IS NULL OR r.chat_id = sqlc.narg('chat_id')::uuid OR r.id = sqlc.narg('chat_id')::uuid)
  AND (sqlc.narg('observation_id')::uuid IS NULL OR r.subject_observation_id = sqlc.narg('observation_id')::uuid OR r.observation_id = sqlc.narg('observation_id')::uuid)
  AND (sqlc.narg('pinned')::boolean IS NULL OR (r.pinned_at IS NOT NULL) = sqlc.narg('pinned')::boolean)
  AND (sqlc.narg('pattern')::text IS NULL OR r.title ILIKE sqlc.narg('pattern')::text OR r.prompt ILIKE sqlc.narg('pattern')::text OR EXISTS (
    SELECT 1 FROM findings sf
    WHERE sf.agent_request_id = r.id AND sf.markdown ILIKE sqlc.narg('pattern')::text
      AND CASE WHEN sf.observation_id IS NULL
        THEN sf.dashboard_id IS NULL OR EXISTS (SELECT 1 FROM dashboards sd WHERE sd.id = sf.dashboard_id AND sd.folder_id = ANY(sqlc.arg('folders')::uuid[]))
        ELSE EXISTS (SELECT 1 FROM observations so WHERE so.id = sf.observation_id AND so.folder_id = ANY(sqlc.arg('folders')::uuid[]))
      END))
  AND (sqlc.narg('states')::text[] IS NULL OR r.state = ANY(sqlc.narg('states')::text[]));

-- name: LockSuggestRequests :exec
SELECT pg_advisory_xact_lock(hashtextextended('agent_requests:suggest', 0));

-- name: CountActiveSuggestRequestsBy :one
SELECT count(*) FROM agent_requests
WHERE intent = 'suggest' AND status IN ('queued', 'running') AND requested_by_id = @requested_by_id;

-- name: CountActiveSuggestRequests :one
SELECT count(*) FROM agent_requests WHERE intent = 'suggest' AND status IN ('queued', 'running');

-- name: EnqueueSuggestRequest :one
INSERT INTO agent_requests (
  dashboard_id, intent, kind, prompt, panel_id, base_version, draft,
  requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client
) VALUES (
  @dashboard_id, 'suggest', 'build', @prompt, @panel_id, @base_version::integer, sqlc.narg('draft')::jsonb,
  @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text
)
RETURNING *;

-- name: SucceedSuggestRequest :one
UPDATE agent_requests
SET state = 'succeeded',
  status = 'succeeded',
  result_kind = 'suggestion',
  suggestion = @suggestion::jsonb,
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE id = @id AND state = ANY(@from_states::text[]) AND attempts = @attempts
RETURNING *;

-- name: GetSuggestRequest :one
SELECT * FROM agent_requests WHERE id = @id AND dashboard_id = @dashboard_id AND intent = 'suggest';

-- name: EnqueueInvestigateRequest :one
INSERT INTO agent_requests (
  id, dashboard_id, intent, kind, prompt, panel_id, context_from, context_to,
  requested_by_kind, requested_by_id, requested_by_name, requested_by_email, origin, origin_client,
  retries_id, assigned_kind, assigned_id, trigger, thread_message_id, thread_context, time_zone, as_of,
  chat_id, anchor, block_id, client_id, subject_observation_id, investigation
) VALUES (
  coalesce(sqlc.narg('id')::uuid, gen_random_uuid()), sqlc.narg('dashboard_id')::uuid, 'ask', 'investigate', @prompt, sqlc.narg('panel_id')::text,
  sqlc.narg('context_from')::timestamptz, sqlc.narg('context_to')::timestamptz,
  @requested_by_kind, @requested_by_id, @requested_by_name, sqlc.narg('requested_by_email'),
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text,
  sqlc.narg('retries_id')::uuid, coalesce(sqlc.narg('assigned_kind')::text, 'builtin'), sqlc.narg('assigned_id')::uuid, coalesce(sqlc.narg('trigger')::text, 'person'),
  sqlc.narg('thread_message_id')::uuid, sqlc.narg('thread_context')::text, sqlc.narg('time_zone')::text, sqlc.narg('as_of')::timestamptz,
  sqlc.narg('chat_id')::uuid, sqlc.narg('anchor')::jsonb, sqlc.narg('block_id')::uuid, sqlc.narg('client_id')::uuid,
  sqlc.narg('subject_observation_id')::uuid, @investigation::boolean
)
ON CONFLICT (requested_by_kind, requested_by_id, client_id) WHERE client_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: CountActiveInvestigateRequestsBy :one
SELECT count(*) FROM agent_requests
WHERE investigation AND status IN ('queued', 'running') AND requested_by_id = @requested_by_id;

-- name: CountActiveInvestigateRequests :one
SELECT count(*) FROM agent_requests WHERE investigation AND status IN ('queued', 'running');

-- name: SucceedFindingRequest :one
UPDATE agent_requests
SET state = 'succeeded',
  status = 'succeeded',
  result_kind = 'observation',
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE agent_requests.id = @id AND agent_requests.state = ANY(@from_states::text[]) AND agent_requests.attempts = @attempts
  AND agent_requests.intent <> 'suggest'
  AND EXISTS (SELECT 1 FROM findings f WHERE f.agent_request_id = agent_requests.id)
RETURNING *;

-- name: RunHasProposal :one
SELECT EXISTS (SELECT 1 FROM proposals WHERE run_id = @id);

-- name: SucceedProposalRequest :one
UPDATE agent_requests
SET state = 'succeeded',
  status = 'succeeded',
  result_kind = 'proposal',
  run_usage = sqlc.narg('run_usage')::jsonb,
  lease_until = NULL,
  finished_at = clock_timestamp()
WHERE agent_requests.id = @id AND agent_requests.state = ANY(@from_states::text[]) AND agent_requests.attempts = @attempts
  AND EXISTS (SELECT 1 FROM proposals p WHERE p.run_id = agent_requests.id)
RETURNING *;

-- name: UpdateAgentRequestListFields :one
WITH chat AS (
  UPDATE chats
  SET title = CASE WHEN @set_title::boolean THEN sqlc.narg('title')::text ELSE chats.title END,
    pinned_at = CASE WHEN sqlc.narg('pinned')::boolean IS NULL THEN chats.pinned_at
      WHEN sqlc.narg('pinned')::boolean THEN coalesce(chats.pinned_at, clock_timestamp())
      ELSE NULL END
  WHERE chats.id = @id
)
UPDATE agent_requests
SET title = CASE WHEN @set_title::boolean THEN sqlc.narg('title')::text ELSE title END,
  pinned_at = CASE WHEN sqlc.narg('pinned')::boolean IS NULL THEN pinned_at
    WHEN sqlc.narg('pinned')::boolean THEN coalesce(pinned_at, clock_timestamp())
    ELSE NULL END
WHERE agent_requests.id = @id
RETURNING *;

-- name: ArchiveAgentRequest :execrows
WITH chat AS (
  UPDATE chats SET archived_at = coalesce(archived_at, clock_timestamp()) WHERE chats.id = @id
)
UPDATE agent_requests SET archived_at = coalesce(agent_requests.archived_at, clock_timestamp()) WHERE agent_requests.id = @id;

-- name: GetAssigneeName :one
SELECT coalesce(
  (SELECT c.name FROM oauth_grants g JOIN oauth_clients c ON c.id = g.client_id WHERE @kind::text = 'grant' AND g.id = @id::uuid),
  (SELECT s.name FROM service_accounts s WHERE @kind::text = 'service_account' AND s.id = @id::uuid),
  ''
)::text AS name;

-- name: ExpireUnclaimedAgentRequests :many
UPDATE agent_requests r
SET state = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  status = CASE WHEN (saved.version IS NULL AND f.id IS NULL AND pr.id IS NULL) THEN 'failed' ELSE 'succeeded' END,
  result_kind = CASE WHEN f.id IS NOT NULL THEN 'observation' WHEN saved.version IS NOT NULL THEN 'version' WHEN pr.id IS NOT NULL THEN 'proposal' ELSE r.result_kind END,
  result_version = saved.version,
  intent = CASE WHEN saved.version IS NOT NULL THEN 'apply' ELSE r.intent END,
  dashboard_id = coalesce(r.dashboard_id, saved.dashboard_id),
  error = CASE WHEN (saved.version IS NOT NULL OR f.id IS NOT NULL OR pr.id IS NOT NULL) THEN NULL ELSE @message::text END,
  failure_code = CASE WHEN (saved.version IS NOT NULL OR f.id IS NOT NULL OR pr.id IS NOT NULL) THEN NULL ELSE 'agent_unavailable' END,
  lease_until = NULL,
  finished_at = clock_timestamp()
FROM agent_requests e
LEFT JOIN dashboard_versions saved ON saved.agent_request_id = e.id AND (e.dashboard_id IS NULL OR saved.dashboard_id = e.dashboard_id)
LEFT JOIN findings f ON f.agent_request_id = e.id
LEFT JOIN LATERAL (SELECT p.id FROM proposals p WHERE p.run_id = e.id LIMIT 1) pr ON true
WHERE r.id = e.id
  AND r.status IN ('queued', 'running')
  AND r.state = ANY(@from_states::text[])
  AND r.assigned_kind <> 'builtin'
  AND r.cancel_requested_at IS NULL
  AND (r.state = 'queued' OR r.lease_until < clock_timestamp())
  AND greatest(r.created_at, coalesce(r.lease_until, r.created_at)) < clock_timestamp() - make_interval(secs => @unclaimed_seconds::double precision)
RETURNING r.*;

-- name: SetRequestObservation :one
UPDATE agent_requests SET observation_id = @observation_id WHERE id = @id
RETURNING *;

-- name: ListRunsAwaitingReply :many
SELECT r.* FROM agent_requests r
WHERE r.state = ANY(@terminal::text[])
  AND r.thread_message_id IS NOT NULL
  AND r.finished_at > clock_timestamp() - make_interval(secs => @within_seconds::float8)
  AND r.thread_replied_at IS NULL
ORDER BY r.finished_at DESC, r.id
LIMIT @max_rows::int;

-- name: ChatTurnsBefore :many
SELECT r.id, r.prompt, r.result_version, f.markdown AS answer, v.summary AS version_summary
FROM agent_requests r
LEFT JOIN findings f ON f.agent_request_id = r.id
LEFT JOIN dashboard_versions v ON v.agent_request_id = r.id
WHERE r.chat_id = @chat_id AND r.state = 'succeeded'
  AND (r.created_at, r.id) < (SELECT b.created_at, b.id FROM agent_requests b WHERE b.id = @before_id)
ORDER BY r.created_at DESC, r.id DESC
LIMIT @row_limit;

-- name: SetRequestThreadContext :exec
UPDATE agent_requests SET thread_context = sqlc.narg('thread_context')::text
WHERE id = @id AND thread_context IS NULL;
