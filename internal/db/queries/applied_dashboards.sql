-- name: GetAppliedDashboard :one
SELECT dashboard_id, content_sha256 FROM applied_dashboards WHERE path = @path;

-- name: DeleteAppliedDashboardElsewhere :exec
DELETE FROM applied_dashboards WHERE dashboard_id = @dashboard_id AND path <> @path;

-- name: UpsertAppliedDashboard :exec
INSERT INTO applied_dashboards (path, dashboard_id, content_sha256, applied_by_id)
VALUES (@path, @dashboard_id, @content_sha256, @applied_by_id)
ON CONFLICT (path) DO UPDATE SET
  dashboard_id = EXCLUDED.dashboard_id,
  content_sha256 = EXCLUDED.content_sha256,
  applied_by_id = EXCLUDED.applied_by_id,
  updated_at = now();

-- name: ApplyCandidates :many
SELECT id FROM dashboards
WHERE folder_id = @folder_id AND lower(title) = lower(@title::text)
ORDER BY id
LIMIT 2;

-- name: IsProvisionedDashboard :one
SELECT EXISTS (SELECT 1 FROM provisioned_dashboards WHERE dashboard_id = @dashboard_id)::boolean;

-- name: ListAppliedInFolder :many
SELECT a.path, a.dashboard_id
FROM applied_dashboards a
JOIN dashboards d ON d.id = a.dashboard_id
WHERE d.folder_id = @folder_id
ORDER BY a.path COLLATE "C";
