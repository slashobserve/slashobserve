-- name: ListContextSources :many
SELECT * FROM context_sources ORDER BY uid;

-- name: GetContextSource :one
SELECT * FROM context_sources WHERE uid = @uid;

-- name: LockContextSource :one
SELECT * FROM context_sources WHERE uid = @uid FOR UPDATE;

-- name: LockContextSourceUID :exec
SELECT pg_advisory_xact_lock(hashtextextended('context_source:' || @uid::text, 0));

-- name: LockContextSourceCount :exec
SELECT pg_advisory_xact_lock(hashtextextended('context_sources:count', 0));

-- name: GetContextSourceTool :one
SELECT * FROM context_source_tools WHERE source_id = @source_id AND name = @name FOR UPDATE;

-- name: CountContextSources :one
SELECT count(*) FROM context_sources;

-- name: InsertContextSource :one
INSERT INTO context_sources (id, uid, name, description, url, audience, secret_header, secret, provisioned, created_by)
VALUES (@id, @uid, @name, @description, @url, @audience, @secret_header, @secret, @provisioned, @created_by)
RETURNING *;

-- name: UpdateContextSource :one
UPDATE context_sources
SET name = @name, description = @description, url = @url, audience = @audience, secret_header = @secret_header, secret = @secret,
    provisioned = @provisioned,
    checked_at = CASE WHEN @reset_check::boolean THEN NULL ELSE checked_at END,
    failure = CASE WHEN @reset_check::boolean THEN NULL ELSE failure END,
    updated_at = clock_timestamp()
WHERE uid = @uid
RETURNING *;

-- name: DeleteContextSource :execrows
DELETE FROM context_sources WHERE uid = @uid;

-- name: MarkContextSourceChecked :exec
UPDATE context_sources SET checked_at = clock_timestamp(), failure = sqlc.narg('failure')
WHERE id = @id;

-- name: ListContextSourceTools :many
SELECT * FROM context_source_tools WHERE source_id = @source_id ORDER BY name COLLATE "C";

-- name: ListContextSourceToolCounts :many
SELECT source_id, state, count(*) AS n FROM context_source_tools GROUP BY source_id, state;

-- name: PutContextSourceTool :exec
INSERT INTO context_source_tools (source_id, name, state, refusal, description, input_schema, hash, seen_description, seen_input_schema, seen_hash, approved_by, approved_at)
VALUES (@source_id, @name, @state, @refusal, @description, @input_schema, @hash, @seen_description, @seen_input_schema, @seen_hash, @approved_by, @approved_at)
ON CONFLICT (source_id, name) DO UPDATE SET
  state = EXCLUDED.state, refusal = EXCLUDED.refusal, description = EXCLUDED.description, input_schema = EXCLUDED.input_schema,
  hash = EXCLUDED.hash, seen_description = EXCLUDED.seen_description, seen_input_schema = EXCLUDED.seen_input_schema,
  seen_hash = EXCLUDED.seen_hash, approved_by = EXCLUDED.approved_by, approved_at = EXCLUDED.approved_at,
  updated_at = clock_timestamp();

-- name: DeleteContextSourceTool :exec
DELETE FROM context_source_tools WHERE source_id = @source_id AND name = @name;

-- name: DeleteContextSourceTools :exec
DELETE FROM context_source_tools WHERE source_id = @source_id;
