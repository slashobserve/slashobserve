-- name: CreateObservation :one
INSERT INTO observations (folder_id, title, time_from, time_to, created_by_kind, created_by_id, created_by_name, origin, origin_client, run_id)
VALUES (@folder_id, @title, sqlc.narg('time_from')::timestamptz, sqlc.narg('time_to')::timestamptz,
  @created_by_kind, @created_by_id, @created_by_name, sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('run_id')::uuid)
RETURNING *;

-- name: ObservationByID :one
SELECT * FROM observations WHERE id = @id;

-- name: LockObservation :one
SELECT * FROM observations WHERE id = @id FOR UPDATE;

-- name: ListObservations :many
SELECT * FROM observations
WHERE folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('folder_id')::uuid IS NULL OR folder_id = sqlc.narg('folder_id')::uuid)
  AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
  AND (sqlc.narg('cursor_updated_at')::timestamptz IS NULL
    OR (updated_at, id) < (sqlc.narg('cursor_updated_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY updated_at DESC, id DESC
LIMIT @row_limit;

-- name: UpdateObservation :one
UPDATE observations
SET title = @title, time_from = sqlc.narg('time_from')::timestamptz, time_to = sqlc.narg('time_to')::timestamptz, updated_at = clock_timestamp()
WHERE id = @id
RETURNING *;

-- name: MoveObservation :one
UPDATE observations SET folder_id = @folder_id, updated_at = clock_timestamp() WHERE id = @id RETURNING *;

-- name: ResolveObservation :one
UPDATE observations
SET state = 'resolved', summary = @summary, resolved_at = clock_timestamp(), updated_at = clock_timestamp()
WHERE id = @id AND state = ANY(@from_states::text[])
RETURNING *;

-- name: ReopenObservation :one
UPDATE observations
SET state = 'open', resolved_at = NULL, updated_at = clock_timestamp()
WHERE id = @id AND state = ANY(@from_states::text[])
RETURNING *;

-- name: TouchObservation :exec
UPDATE observations SET updated_at = clock_timestamp() WHERE id = @id;

-- name: ObservationThread :one
SELECT id FROM threads WHERE observation_id = @observation_id;

-- name: ObservationThreads :many
SELECT observation_id, id FROM threads WHERE observation_id = ANY(@observation_ids::uuid[]);

-- name: BlocksOfObservation :many
SELECT * FROM observation_blocks WHERE observation_id = @observation_id ORDER BY position;

-- name: BlockByID :one
SELECT * FROM observation_blocks WHERE id = @id;

-- name: CountBlocks :one
SELECT count(*)::integer AS total, (count(*) FILTER (WHERE kind IN ('panel', 'snapshot')))::integer AS panels
FROM observation_blocks
WHERE observation_id = @observation_id;

-- name: InsertBlock :one
INSERT INTO observation_blocks (observation_id, kind, state, position, width, body, snapshot_id, run_id,
  created_by_kind, created_by_id, created_by_name, origin, origin_client)
VALUES (@observation_id, @kind, @state, @position, @width, @body::jsonb, sqlc.narg('snapshot_id')::uuid, sqlc.narg('run_id')::uuid,
  @created_by_kind, @created_by_id, @created_by_name, sqlc.narg('origin')::text, sqlc.narg('origin_client')::text)
RETURNING *;

-- name: ShiftBlocks :exec
UPDATE observation_blocks
SET position = position + @delta::integer
WHERE observation_id = @observation_id
  AND position BETWEEN @from_position::integer AND @to_position::integer
  AND id IS DISTINCT FROM sqlc.narg('except_id')::uuid;

-- name: UpdateBlock :one
UPDATE observation_blocks
SET body = @body::jsonb, width = @width, position = @position, version = version + 1, updated_at = clock_timestamp()
WHERE id = @id AND version = @version AND state = ANY(@from_states::text[])
RETURNING *;

-- name: KeepBlock :one
UPDATE observation_blocks
SET state = 'live', version = version + 1, updated_at = clock_timestamp()
WHERE id = @id AND version = @version AND state = ANY(@from_states::text[])
RETURNING *;

-- name: DeleteBlock :execrows
DELETE FROM observation_blocks
WHERE id = @id AND state = ANY(@from_states::text[])
  AND (sqlc.narg('version')::integer IS NULL OR version = sqlc.narg('version')::integer);

-- name: InsertSnapshot :one
INSERT INTO snapshots (observation_id, panel, query, frames, user_scoped, time_from, time_to,
  created_by_kind, created_by_id, created_by_name, origin, origin_client, run_id)
VALUES (@observation_id, @panel::jsonb, @query::jsonb, @frames::jsonb, @user_scoped, @time_from, @time_to,
  @created_by_kind, @created_by_id, @created_by_name, sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('run_id')::uuid)
RETURNING *;

-- name: SnapshotByID :one
SELECT * FROM snapshots WHERE id = @id;
