-- name: CreateDashboard :one
INSERT INTO dashboards (title, tags, latest_version) VALUES (@title, @tags, 1)
RETURNING id, title, tags, latest_version, created_at, updated_at;

-- name: InsertDashboardVersion :one
INSERT INTO dashboard_versions (
  dashboard_id, version, parent_version, restored_from, spec, summary,
  author_kind, author_id, author_name, author_email, agent_request_id
) VALUES (
  @dashboard_id, @version, @parent_version, @restored_from, @spec, @summary,
  @author_kind, @author_id, @author_name, @author_email, @agent_request_id
)
RETURNING dashboard_id, version, parent_version, restored_from, summary,
  author_kind, author_id, author_name, author_email, created_at;

-- name: BumpDashboardVersion :one
UPDATE dashboards
SET latest_version = latest_version + 1, title = @title, tags = @tags, updated_at = now()
WHERE id = @id AND latest_version = @expected_version
RETURNING id, title, tags, latest_version, created_at, updated_at;

-- name: GetDashboardLatestVersion :one
SELECT latest_version FROM dashboards WHERE id = $1;

-- name: GetDashboard :one
SELECT sqlc.embed(d), sqlc.embed(v)
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
WHERE d.id = $1;

-- name: GetDashboardVersion :one
SELECT sqlc.embed(d), sqlc.embed(v),
  lv.author_kind AS updated_by_kind, lv.author_id AS updated_by_id,
  lv.author_name AS updated_by_name, lv.author_email AS updated_by_email
FROM dashboards d
JOIN dashboard_versions lv ON lv.dashboard_id = d.id AND lv.version = d.latest_version
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = @version
WHERE d.id = @id;

-- name: GetDashboardVersionSpec :one
SELECT spec FROM dashboard_versions WHERE dashboard_id = @id AND version = @version;

-- name: ListDashboards :many
SELECT sqlc.embed(d),
  v.author_kind AS updated_by_kind, v.author_id AS updated_by_id,
  v.author_name AS updated_by_name, v.author_email AS updated_by_email
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
WHERE sqlc.narg('cursor_updated_at')::timestamptz IS NULL
  OR (d.updated_at, d.id) < (sqlc.narg('cursor_updated_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
ORDER BY d.updated_at DESC, d.id DESC
LIMIT @row_limit;

-- name: ListDashboardVersions :many
SELECT dashboard_id, version, parent_version, restored_from, summary,
  author_kind, author_id, author_name, author_email, created_at
FROM dashboard_versions
WHERE dashboard_id = @id AND (sqlc.narg('before')::int IS NULL OR version < sqlc.narg('before')::int)
ORDER BY version DESC
LIMIT @row_limit;

-- name: NotifyEvent :exec
SELECT pg_notify(@channel::text, @payload::text);
