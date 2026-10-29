-- name: CreateDashboard :one
INSERT INTO dashboards (
  title, tags, latest_version, created_by_kind, created_by_id, created_by_name, created_by_email,
  created_origin, created_origin_client, created_via
) VALUES (
  @title, @tags, 1, @created_by_kind, @created_by_id, @created_by_name, sqlc.narg('created_by_email')::text,
  sqlc.narg('created_origin')::text, sqlc.narg('created_origin_client')::text, @created_via
)
RETURNING *;

-- name: InsertDashboardVersion :one
INSERT INTO dashboard_versions (
  dashboard_id, version, parent_version, restored_from, spec, summary,
  author_kind, author_id, author_name, author_email, agent_request_id, origin, origin_client, unverified
) VALUES (
  @dashboard_id, @version, @parent_version, @restored_from, @spec, @summary,
  @author_kind, @author_id, @author_name, @author_email, @agent_request_id,
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('unverified')::jsonb
)
RETURNING dashboard_id, version, parent_version, restored_from, summary,
  author_kind, author_id, author_name, author_email, origin, origin_client, agent_request_id, created_at;

-- name: BumpDashboardVersion :one
UPDATE dashboards
SET latest_version = latest_version + 1, title = @title, tags = @tags, updated_at = now()
WHERE id = @id AND latest_version = @expected_version
RETURNING *;

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

-- name: GetDashboardVersionUnverified :one
SELECT unverified FROM dashboard_versions WHERE dashboard_id = @id AND version = @version;

-- name: ListDashboards :many
SELECT sqlc.embed(d),
  v.author_kind AS updated_by_kind, v.author_id AS updated_by_id,
  v.author_name AS updated_by_name, v.author_email AS updated_by_email,
  v.datasource_uids,
  coalesce(v.spec ->> 'description', '')::text AS description,
  (SELECT count(*) FROM dashboard_comments c
    WHERE c.dashboard_id = d.id AND c.parent_id IS NULL AND c.status = 'open') AS open_comment_count
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
WHERE (sqlc.narg('cursor_updated_at')::timestamptz IS NULL
    OR (d.updated_at, d.id) < (sqlc.narg('cursor_updated_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
  AND (sqlc.narg('creator_kind')::text IS NULL
    OR (d.created_by_kind = sqlc.narg('creator_kind')::text AND d.created_by_id = sqlc.narg('creator_id')::uuid))
  AND (NOT @only_agent::boolean OR d.created_via = 'agent')
  AND (NOT @only_mcp::boolean OR d.created_origin = 'mcp')
  AND (sqlc.narg('mcp_client')::text IS NULL OR d.created_origin_client = sqlc.narg('mcp_client')::text)
  AND (sqlc.narg('datasource')::text IS NULL OR v.datasource_uids @> ARRAY[sqlc.narg('datasource')::text])
  AND (sqlc.narg('tags')::text[] IS NULL OR d.tags @> sqlc.narg('tags')::text[])
  AND (sqlc.narg('q')::text IS NULL
    OR strpos(lower(d.title), lower(sqlc.narg('q')::text)) > 0
    OR strpos(lower(coalesce(v.spec ->> 'description', '')), lower(sqlc.narg('q')::text)) > 0)
ORDER BY d.updated_at DESC, d.id DESC
LIMIT @row_limit;

-- name: CountDashboards :one
SELECT count(*)
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
WHERE (sqlc.narg('creator_kind')::text IS NULL
    OR (d.created_by_kind = sqlc.narg('creator_kind')::text AND d.created_by_id = sqlc.narg('creator_id')::uuid))
  AND (NOT @only_agent::boolean OR d.created_via = 'agent')
  AND (NOT @only_mcp::boolean OR d.created_origin = 'mcp')
  AND (sqlc.narg('mcp_client')::text IS NULL OR d.created_origin_client = sqlc.narg('mcp_client')::text)
  AND (sqlc.narg('datasource')::text IS NULL OR v.datasource_uids @> ARRAY[sqlc.narg('datasource')::text])
  AND (sqlc.narg('tags')::text[] IS NULL OR d.tags @> sqlc.narg('tags')::text[])
  AND (sqlc.narg('q')::text IS NULL
    OR strpos(lower(d.title), lower(sqlc.narg('q')::text)) > 0
    OR strpos(lower(coalesce(v.spec ->> 'description', '')), lower(sqlc.narg('q')::text)) > 0);

-- name: CountDashboardsByDatasource :many
SELECT uid::text AS uid, count(*)::integer AS dashboards
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
CROSS JOIN LATERAL unnest(v.datasource_uids) AS uid
GROUP BY uid;

-- name: ListDashboardTags :many
SELECT tag::text AS tag, count(*)::integer AS dashboards
FROM dashboards d
CROSS JOIN LATERAL unnest(d.tags) AS tag
GROUP BY tag
ORDER BY tag COLLATE "C"
LIMIT 500;

-- name: ListDashboardVersions :many
SELECT dashboard_id, version, parent_version, restored_from, summary,
  author_kind, author_id, author_name, author_email, origin, origin_client, agent_request_id, created_at
FROM dashboard_versions
WHERE dashboard_id = @id AND (sqlc.narg('before')::int IS NULL OR version < sqlc.narg('before')::int)
ORDER BY version DESC
LIMIT @row_limit;

-- name: NotifyEvent :exec
SELECT pg_notify(@channel::text, @payload::text);
