-- name: ListMCPSources :many
SELECT * FROM mcp_sources ORDER BY uid;

-- name: GetMCPSource :one
SELECT * FROM mcp_sources WHERE uid = @uid;

-- name: LockMCPSource :one
SELECT * FROM mcp_sources WHERE uid = @uid FOR UPDATE;

-- name: LockMCPSourceUID :exec
SELECT pg_advisory_xact_lock(hashtextextended('mcp_source:' || @uid::text, 0));

-- name: LockMCPSourceCount :exec
SELECT pg_advisory_xact_lock(hashtextextended('mcp_sources:count', 0));

-- name: GetMCPSourceTool :one
SELECT * FROM mcp_source_tools WHERE source_id = @source_id AND name = @name FOR UPDATE;

-- name: CountMCPSources :one
SELECT count(*) FROM mcp_sources;

-- name: InsertMCPSource :one
INSERT INTO mcp_sources (id, uid, name, description, url, audience, secret_header, secret, provisioned, allow_private, created_by)
VALUES (@id, @uid, @name, @description, @url, @audience, @secret_header, @secret, @provisioned, @allow_private, @created_by)
RETURNING *;

-- name: UpdateMCPSource :one
UPDATE mcp_sources
SET name = @name, description = @description, url = @url, audience = @audience, secret_header = @secret_header, secret = @secret,
    provisioned = @provisioned, allow_private = @allow_private,
    checked_at = CASE WHEN @reset_check::boolean THEN NULL ELSE checked_at END,
    failure = CASE WHEN @reset_check::boolean THEN NULL ELSE failure END,
    updated_at = clock_timestamp()
WHERE uid = @uid
RETURNING *;

-- name: DeleteMCPSource :execrows
DELETE FROM mcp_sources WHERE uid = @uid;

-- name: MarkMCPSourceChecked :exec
UPDATE mcp_sources SET checked_at = clock_timestamp(), failure = sqlc.narg('failure'), discoveries = discoveries + 1
WHERE id = @id;

-- name: NoteMCPSourceReachability :exec
UPDATE mcp_sources SET checked_at = clock_timestamp(), failure = sqlc.narg('failure')
WHERE id = @id AND checked_at IS NOT NULL AND failure IS DISTINCT FROM sqlc.narg('failure');

-- name: ListMCPSourceTools :many
SELECT * FROM mcp_source_tools WHERE source_id = @source_id ORDER BY name COLLATE "C";

-- name: ListMCPSourceToolCounts :many
SELECT source_id, state, count(*) AS n FROM mcp_source_tools GROUP BY source_id, state;

-- name: PutMCPSourceTool :exec
INSERT INTO mcp_source_tools (source_id, name, state, refusal, description, input_schema, read_only, hash, seen_description, seen_input_schema, seen_read_only, seen_hash, approved_by, approved_at)
VALUES (@source_id, @name, @state, @refusal, @description, @input_schema, @read_only, @hash, @seen_description, @seen_input_schema, @seen_read_only, @seen_hash, @approved_by, @approved_at)
ON CONFLICT (source_id, name) DO UPDATE SET
  state = EXCLUDED.state, refusal = EXCLUDED.refusal, description = EXCLUDED.description, input_schema = EXCLUDED.input_schema,
  read_only = EXCLUDED.read_only, hash = EXCLUDED.hash, seen_description = EXCLUDED.seen_description, seen_input_schema = EXCLUDED.seen_input_schema,
  seen_read_only = EXCLUDED.seen_read_only, seen_hash = EXCLUDED.seen_hash, approved_by = EXCLUDED.approved_by, approved_at = EXCLUDED.approved_at,
  updated_at = clock_timestamp();

-- name: DeleteMCPSourceTool :exec
DELETE FROM mcp_source_tools WHERE source_id = @source_id AND name = @name;

-- name: DeleteMCPSourceTools :exec
DELETE FROM mcp_source_tools WHERE source_id = @source_id;
