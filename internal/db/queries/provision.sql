-- name: UpsertProvisionedDashboard :exec
INSERT INTO provisioned_dashboards (path, dashboard_id, content_sha256, folder_title)
VALUES (@path, @dashboard_id, @content_sha256, @folder_title)
ON CONFLICT (path) DO UPDATE SET
  dashboard_id = EXCLUDED.dashboard_id,
  content_sha256 = EXCLUDED.content_sha256,
  folder_title = EXCLUDED.folder_title,
  last_error = '',
  updated_at = now();

-- name: ListProvisionedDashboards :many
SELECT path, dashboard_id, content_sha256, folder_title, last_error, updated_at
FROM provisioned_dashboards
ORDER BY path COLLATE "C";

-- name: SetProvisionedDashboardError :exec
UPDATE provisioned_dashboards SET last_error = @last_error WHERE path = @path;

-- name: UpsertProvisionError :exec
INSERT INTO provision_errors (path, message, seen_at)
VALUES (@path, @message, now())
ON CONFLICT (path) DO UPDATE SET message = EXCLUDED.message, seen_at = EXCLUDED.seen_at;

-- name: ListProvisionErrors :many
SELECT path, message, seen_at
FROM provision_errors
ORDER BY path COLLATE "C";

-- name: DeleteProvisionError :exec
DELETE FROM provision_errors WHERE path = @path;
