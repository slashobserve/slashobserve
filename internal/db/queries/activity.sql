-- No row lock on the owner: a delete holds that row and then cascades into child rows an appender may already hold.
-- name: InsertActivity :one
INSERT INTO activity (type, target_kind, target_id, dashboard_id, finding_id, saved_search_id, observation_id, folder_id, actor_kind, actor_id, actor_name, origin, origin_client, run_id, data)
VALUES (
  @type::text,
  @target_kind::text,
  @target_id::uuid,
  CASE WHEN @owner_kind::text = 'dashboard' THEN sqlc.narg('owner_id')::uuid END,
  CASE WHEN @owner_kind::text = 'observation' AND @target_kind::text = 'finding' THEN @target_id::uuid END,
  CASE WHEN @owner_kind::text = 'saved_search' THEN sqlc.narg('owner_id')::uuid END,
  CASE WHEN @owner_kind::text = 'observation' THEN sqlc.narg('owner_id')::uuid END,
  CASE @owner_kind::text
    WHEN 'dashboard' THEN (SELECT d.folder_id FROM dashboards d WHERE d.id = sqlc.narg('owner_id')::uuid)
    WHEN 'saved_search' THEN (SELECT s.folder_id FROM saved_searches s WHERE s.id = sqlc.narg('owner_id')::uuid)
    WHEN 'observation' THEN (SELECT o.folder_id FROM observations o WHERE o.id = sqlc.narg('owner_id')::uuid)
    WHEN 'general' THEN general_folder_id()
  END,
  @actor_kind::text,
  @actor_id::uuid,
  @actor_name::text,
  sqlc.narg('origin')::text,
  sqlc.narg('origin_client')::text,
  sqlc.narg('run_id')::uuid,
  @data::jsonb
)
RETURNING seq;

-- Run alone before InsertActivity so the insert's snapshot sees a move that committed during the wait.
-- name: ShareActivityOwner :exec
SELECT pg_advisory_xact_lock_shared(hashtextextended('activity:' || @owner_kind::text || ':' || CAST(@owner_id::uuid AS text), 0));

-- name: LockActivityOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended('activity:' || @owner_kind::text || ':' || CAST(@owner_id::uuid AS text), 0));

-- The horizon and the scan must share this statement's snapshot; never move the horizon into a function or another query.
-- name: ListActivity :many
SELECT a.*
FROM activity a
WHERE a.xid < pg_snapshot_xmin(pg_current_snapshot())
  AND (a.folder_id = ANY(@folders::uuid[])
    OR (a.folder_id IS NULL AND (@admin::boolean OR (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid))))
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR a.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('folder_id')::uuid IS NULL OR a.folder_id = sqlc.narg('folder_id')::uuid)
  AND (cardinality(@types::text[]) = 0 OR a.type = ANY(@types::text[]))
  AND (sqlc.narg('since')::timestamptz IS NULL OR a.created_at >= sqlc.narg('since')::timestamptz)
  AND (NOT @has_cursor::boolean OR (a.xid, a.seq) < (@cursor_xid::xid8, @cursor_seq::bigint))
ORDER BY a.xid DESC, a.seq DESC
LIMIT @row_limit;

-- The horizon and the scan must share this statement's snapshot; never move the horizon into a function or another query.
-- name: ListActivityAfter :many
SELECT a.*
FROM activity a
WHERE a.xid < pg_snapshot_xmin(pg_current_snapshot())
  AND (a.folder_id = ANY(@folders::uuid[])
    OR (a.folder_id IS NULL AND (@admin::boolean OR (a.actor_kind = @reader_kind::text AND a.actor_id = @reader_id::uuid))))
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR a.dashboard_id = sqlc.narg('dashboard_id')::uuid)
  AND (sqlc.narg('folder_id')::uuid IS NULL OR a.folder_id = sqlc.narg('folder_id')::uuid)
  AND (cardinality(@types::text[]) = 0 OR a.type = ANY(@types::text[]))
  AND (sqlc.narg('since')::timestamptz IS NULL OR a.created_at >= sqlc.narg('since')::timestamptz)
  AND (a.xid, a.seq) > (@after_xid::xid8, @after_seq::bigint)
ORDER BY a.xid, a.seq
LIMIT @row_limit;

-- The horizon and the scan must share this statement's snapshot; never move the horizon into a function or another query.
-- name: ActivityHead :one
SELECT a.xid, a.seq
FROM activity a
WHERE a.xid < pg_snapshot_xmin(pg_current_snapshot())
ORDER BY a.xid DESC, a.seq DESC
LIMIT 1;

-- name: ActivityXid :one
SELECT xid FROM activity WHERE seq = @seq;

-- name: ActivityHeld :one
SELECT EXISTS (SELECT 1 FROM activity WHERE xid >= pg_snapshot_xmin(pg_current_snapshot()))::boolean;

-- name: MoveDashboardActivity :execrows
UPDATE activity SET folder_id = @folder_id WHERE dashboard_id = @dashboard_id;

-- name: MoveSavedSearchActivity :execrows
UPDATE activity SET folder_id = @folder_id WHERE saved_search_id = @saved_search_id;

-- name: MoveObservationActivity :execrows
UPDATE activity SET folder_id = @folder_id WHERE observation_id = @observation_id;

-- name: PruneActivity :execrows
DELETE FROM activity
WHERE seq IN (SELECT old.seq FROM activity old WHERE old.created_at < @cutoff ORDER BY old.created_at, old.seq LIMIT @batch);
