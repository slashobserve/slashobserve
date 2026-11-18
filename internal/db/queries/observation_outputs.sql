-- name: GetObservationPin :one
SELECT annotation_id, created_at
FROM observation_pins
WHERE observation_id = @observation_id AND dashboard_id = @dashboard_id;

-- name: InsertObservationPin :execrows
INSERT INTO observation_pins (observation_id, dashboard_id, annotation_id)
VALUES (@observation_id, @dashboard_id, @annotation_id)
ON CONFLICT (observation_id, dashboard_id) DO NOTHING;

-- name: GetObservationDashboard :one
SELECT dashboard_id
FROM observation_dashboards
WHERE observation_id = @observation_id
  AND block_id IS NOT DISTINCT FROM sqlc.narg('block_id')::uuid
  AND created_by_kind = @created_by_kind
  AND created_by_id = @created_by_id;

-- name: DeleteObservationDashboard :exec
DELETE FROM observation_dashboards
WHERE observation_id = @observation_id
  AND block_id IS NOT DISTINCT FROM sqlc.narg('block_id')::uuid
  AND created_by_kind = @created_by_kind
  AND created_by_id = @created_by_id;

-- name: InsertObservationDashboard :execrows
INSERT INTO observation_dashboards (observation_id, block_id, dashboard_id, created_by_kind, created_by_id)
VALUES (@observation_id, sqlc.narg('block_id')::uuid, @dashboard_id, @created_by_kind, @created_by_id)
ON CONFLICT DO NOTHING;

-- name: GetObservationRunDashboard :one
SELECT r.dashboard_id
FROM observations o
JOIN agent_requests r ON r.id = o.run_id
WHERE o.id = @id;
