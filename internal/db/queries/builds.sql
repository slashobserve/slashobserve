-- name: UpsertAgentRequestBuild :one
INSERT INTO agent_request_builds (
  agent_request_id, dashboard_id, base_version, spec, unverified, planned_panels, summary, panels
)
SELECT @agent_request_id::uuid, sqlc.narg('dashboard_id')::uuid, sqlc.narg('base_version')::integer, @spec::jsonb,
  sqlc.narg('unverified')::jsonb, @planned_panels::integer, @summary::text, @panels::integer
WHERE EXISTS (
  SELECT 1 FROM agent_requests r
  WHERE r.id = @agent_request_id::uuid AND r.status = 'running' AND r.attempts = @attempt::integer
)
ON CONFLICT (agent_request_id) DO UPDATE
SET spec = EXCLUDED.spec,
  unverified = EXCLUDED.unverified,
  planned_panels = EXCLUDED.planned_panels,
  summary = EXCLUDED.summary,
  panels = EXCLUDED.panels,
  saves = agent_request_builds.saves + 1,
  updated_at = clock_timestamp()
RETURNING *;

-- name: GetAgentRequestBuild :one
SELECT * FROM agent_request_builds WHERE agent_request_id = @agent_request_id;

-- name: GetAgentRequestBuildView :one
SELECT b.agent_request_id, b.dashboard_id, b.base_version, b.spec, b.unverified, b.planned_panels, b.summary, b.saves,
  b.panels, b.created_at, b.updated_at,
  r.requested_by_kind, r.requested_by_id, r.origin, r.origin_client
FROM agent_request_builds b
JOIN agent_requests r ON r.id = b.agent_request_id
WHERE b.agent_request_id = @agent_request_id;

-- name: ListAgentRequestBuildState :many
SELECT q.id::uuid AS id, b.saves, b.panels, b.planned_panels, coalesce(b.spec ->> 'title', '')::text AS title, b.updated_at,
  c.id AS continued_by
FROM unnest(@ids::uuid[]) AS q(id)
LEFT JOIN agent_request_builds b ON b.agent_request_id = q.id
LEFT JOIN agent_requests c ON c.continues_id = q.id;

-- name: DeleteAgentRequestBuild :execrows
DELETE FROM agent_request_builds b
USING agent_requests r
WHERE b.agent_request_id = @agent_request_id::uuid
  AND r.id = b.agent_request_id AND r.status = 'running' AND r.attempts = @attempt::integer;

-- name: DeleteExpiredAgentRequestBuilds :execrows
DELETE FROM agent_request_builds b
USING agent_requests r
WHERE r.id = b.agent_request_id
  AND r.finished_at < clock_timestamp() - make_interval(days => @days::integer);

-- name: MarkAgentRequestBuildSaved :execrows
UPDATE agent_requests
SET result_version = @result_version::integer,
  dashboard_id = @dashboard_id::uuid,
  build_saved_at = clock_timestamp()
WHERE id = @id AND status = 'failed' AND build_saved_at IS NULL;

-- name: DeleteStoppedAgentRequestBuild :execrows
DELETE FROM agent_request_builds b
USING agent_requests r
WHERE b.agent_request_id = @agent_request_id::uuid
  AND r.id = b.agent_request_id AND r.build_saved_at IS NOT NULL;

-- name: MoveAgentRequestBuild :execrows
UPDATE agent_request_builds b
SET agent_request_id = @to_id::uuid
FROM agent_requests r
WHERE b.agent_request_id = @from_id::uuid
  AND r.id = b.agent_request_id AND r.status = 'failed' AND r.build_saved_at IS NULL;
