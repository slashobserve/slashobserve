-- name: ListDatasources :many
SELECT * FROM datasources ORDER BY uid;

-- name: GetDatasource :one
SELECT * FROM datasources WHERE uid = @uid;

-- name: LockDatasource :one
SELECT * FROM datasources WHERE uid = @uid FOR UPDATE;

-- name: LockDatasourceCount :exec
SELECT pg_advisory_xact_lock(hashtextextended('datasources:count', 0));

-- name: CountDatasources :one
SELECT count(*) FROM datasources;

-- name: InsertDatasource :one
INSERT INTO datasources (id, uid, type, name, audience, config, secrets, secret_names, created_by)
VALUES (@id, @uid, @type, @name, @audience, @config, @secrets, @secret_names, @created_by)
RETURNING *;

-- name: UpdateDatasource :one
UPDATE datasources
SET name = @name, audience = @audience, config = @config, secrets = @secrets, secret_names = @secret_names, updated_at = clock_timestamp()
WHERE uid = @uid
RETURNING *;

-- name: DeleteDatasource :execrows
DELETE FROM datasources WHERE uid = @uid;

-- name: ListDatasourceDashboards :many
SELECT d.id, d.title, (d.folder_id = ANY(@visible::uuid[]))::boolean AS visible
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
WHERE v.datasource_uids @> ARRAY[@uid::text]
ORDER BY d.title, d.id
LIMIT 200;

-- name: ListDatasourceSavedSearches :many
SELECT id, title, kind, (folder_id = ANY(@visible::uuid[]))::boolean AS visible
FROM saved_searches
WHERE datasource_uid = @uid
ORDER BY title, id
LIMIT 200;
