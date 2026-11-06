-- name: InsertGrafanaConnection :one
INSERT INTO grafana_connections (id, name, url, token_ciphertext, created_by)
VALUES (@id, @name, @url, @token_ciphertext, @created_by)
RETURNING *;

-- name: GetGrafanaConnection :one
SELECT * FROM grafana_connections WHERE id = @id;

-- name: ListGrafanaConnections :many
SELECT * FROM grafana_connections ORDER BY created_at, id;

-- name: UpdateGrafanaConnection :one
UPDATE grafana_connections
SET name = @name, url = @url, token_ciphertext = @token_ciphertext, updated_at = now(),
  last_test_at = CASE WHEN @reset_test::boolean THEN NULL ELSE last_test_at END,
  last_test_ok = CASE WHEN @reset_test::boolean THEN false ELSE last_test_ok END,
  last_test_message = CASE WHEN @reset_test::boolean THEN '' ELSE last_test_message END
WHERE id = @id
RETURNING *;

-- name: DeleteGrafanaConnection :execrows
DELETE FROM grafana_connections WHERE id = @id;

-- name: SetGrafanaConnectionTest :one
UPDATE grafana_connections
SET last_test_at = now(), last_test_ok = @ok, last_test_message = @message
WHERE id = @id
RETURNING *;

-- name: SetGrafanaConnectionSync :execrows
UPDATE grafana_connections SET last_sync = @last_sync WHERE id = @id;
