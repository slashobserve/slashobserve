-- name: ListDatasources :many
SELECT * FROM datasources ORDER BY uid;

-- name: GetDatasource :one
SELECT * FROM datasources WHERE uid = @uid;

-- name: CountDatasources :one
SELECT count(*) FROM datasources;

-- name: InsertDatasource :one
INSERT INTO datasources (id, uid, type, name, config, secrets, secret_names, created_by)
VALUES (@id, @uid, @type, @name, @config, @secrets, @secret_names, @created_by)
RETURNING *;

-- name: UpdateDatasource :one
UPDATE datasources
SET name = @name, config = @config, secrets = @secrets, secret_names = @secret_names, updated_at = clock_timestamp()
WHERE uid = @uid
RETURNING *;

-- name: DeleteDatasource :execrows
DELETE FROM datasources WHERE uid = @uid;
