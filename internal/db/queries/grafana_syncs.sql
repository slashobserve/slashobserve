-- name: InsertGrafanaSync :one
INSERT INTO grafana_syncs (connection_id, state, request, started_by_kind, started_by_id, lease_id, leased_by, lease_until)
VALUES (@connection_id, @to_state::text, @request, @started_by_kind::text, @started_by_id::uuid, gen_random_uuid(), @instance_id::text,
  clock_timestamp() + make_interval(secs => @lease_seconds::double precision))
RETURNING *;

-- name: PruneGrafanaSyncs :execrows
DELETE FROM grafana_syncs
WHERE grafana_syncs.connection_id = @connection_id
  AND grafana_syncs.finished_at IS NOT NULL
  AND grafana_syncs.id NOT IN (
    SELECT k.id FROM grafana_syncs k
    WHERE k.connection_id = @connection_id
    ORDER BY k.started_at DESC, k.id DESC
    LIMIT @kept::integer
  );

-- name: GetGrafanaSync :one
SELECT * FROM grafana_syncs WHERE id = @id AND connection_id = @connection_id;

-- name: ListGrafanaSyncItems :many
SELECT * FROM grafana_sync_items WHERE sync_id = @sync_id ORDER BY id;

-- name: ListGrafanaSyncItemUIDs :many
SELECT uid FROM grafana_sync_items WHERE sync_id = @sync_id;

-- name: LastGrafanaSyncs :many
SELECT DISTINCT ON (connection_id) * FROM grafana_syncs
WHERE connection_id = ANY(@connection_ids::uuid[]) AND finished_at IS NOT NULL
ORDER BY connection_id, finished_at DESC, id DESC;

-- name: RenewGrafanaSyncLease :execrows
UPDATE grafana_syncs
SET lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision)
WHERE id = @id AND lease_id = @lease_id AND state = ANY(@from_states::text[]);

-- name: SetGrafanaSyncTotal :execrows
UPDATE grafana_syncs SET total = @total::integer
WHERE id = @id AND lease_id = @lease_id AND state = ANY(@from_states::text[]);

-- name: LockHeldGrafanaSync :one
SELECT id FROM grafana_syncs
WHERE id = @id AND lease_id = @lease_id AND state = ANY(@from_states::text[])
FOR UPDATE;

-- name: InsertGrafanaSyncItem :execrows
INSERT INTO grafana_sync_items (sync_id, uid, title, outcome, dashboard_id, unverified_panels, placeholder_panels, error)
VALUES (@sync_id, @uid, @title, @outcome, sqlc.narg('dashboard_id'), @unverified_panels, @placeholder_panels, sqlc.narg('error'))
ON CONFLICT (sync_id, uid) DO NOTHING;

-- name: CountGrafanaSyncItem :execrows
UPDATE grafana_syncs
SET done = done + 1,
  failed = failed + CASE WHEN @outcome::text = 'failed' THEN 1 ELSE 0 END,
  skipped = skipped + CASE WHEN @outcome::text = 'skipped' THEN 1 ELSE 0 END
WHERE id = @id;

-- name: MoveHeldGrafanaSync :execrows
UPDATE grafana_syncs
SET state = @to_state::text,
  error = sqlc.narg('error'),
  finished_at = CASE WHEN @to_state::text = 'running' THEN NULL ELSE clock_timestamp() END,
  lease_id = NULL,
  leased_by = NULL,
  lease_until = NULL
WHERE id = @id AND lease_id = @lease_id AND state = ANY(@from_states::text[]);

-- name: ClaimGrafanaSync :one
UPDATE grafana_syncs s
SET lease_id = gen_random_uuid(),
  leased_by = @instance_id::text,
  lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision),
  lease_losses = s.lease_losses + CASE WHEN s.lease_id IS NULL THEN 0 ELSE 1 END
WHERE s.id = (
  SELECT c.id FROM grafana_syncs c
  WHERE c.state = ANY(@from_states::text[])
    AND (c.lease_id IS NULL OR (c.lease_until < clock_timestamp() AND c.lease_losses = 0))
  ORDER BY c.started_at, c.id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING *;

-- name: ExhaustGrafanaSyncs :many
UPDATE grafana_syncs
SET state = @to_state::text,
  error = @error::text,
  finished_at = clock_timestamp(),
  lease_id = NULL,
  leased_by = NULL,
  lease_until = NULL,
  lease_losses = lease_losses + 1
WHERE state = ANY(@from_states::text[])
  AND lease_id IS NOT NULL
  AND lease_until < clock_timestamp()
  AND lease_losses >= 1
RETURNING id, connection_id;

-- name: CancelGrafanaSync :one
UPDATE grafana_syncs
SET state = @to_state::text,
  finished_at = clock_timestamp(),
  lease_id = NULL,
  leased_by = NULL,
  lease_until = NULL
WHERE id = @id AND connection_id = @connection_id AND state = ANY(@from_states::text[])
RETURNING *;
