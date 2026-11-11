-- name: LockWebhooks :exec
SELECT pg_advisory_xact_lock(hashtextextended('webhooks', 0));

-- name: CountWebhooks :one
SELECT count(*) FROM webhooks;

-- name: InsertWebhook :one
INSERT INTO webhooks (id, name, url, secret_ciphertext, event_types, owner_kind, owner_id, cursor_xid, cursor_seq)
VALUES (@id, @name, @url, @secret_ciphertext, @event_types, @owner_kind, @owner_id, @cursor_xid, @cursor_seq)
RETURNING *;

-- name: ListWebhooks :many
SELECT w.*, coalesce(u.name, sa.name, '')::text AS owner_name
FROM webhooks w
LEFT JOIN users u ON w.owner_kind = 'user' AND u.id = w.owner_id
LEFT JOIN service_accounts sa ON w.owner_kind = 'service_account' AND sa.id = w.owner_id
ORDER BY w.created_at, w.id;

-- name: GetWebhook :one
SELECT w.*, coalesce(u.name, sa.name, '')::text AS owner_name
FROM webhooks w
LEFT JOIN users u ON w.owner_kind = 'user' AND u.id = w.owner_id
LEFT JOIN service_accounts sa ON w.owner_kind = 'service_account' AND sa.id = w.owner_id
WHERE w.id = @id;

-- name: GetWebhookForUpdate :one
SELECT * FROM webhooks WHERE id = @id FOR UPDATE;

-- name: UpdateWebhook :one
UPDATE webhooks
SET name = @name,
  url = @url,
  event_types = @event_types,
  owner_kind = @owner_kind,
  owner_id = @owner_id,
  consecutive_failures = @consecutive_failures,
  next_attempt_at = @next_attempt_at,
  disabled_at = sqlc.narg('disabled_at'),
  disabled_reason = sqlc.narg('disabled_reason'),
  updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetWebhookSecret :one
UPDATE webhooks
SET secret_ciphertext = @secret_ciphertext,
  owner_kind = @owner_kind,
  owner_id = @owner_id,
  updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteWebhook :execrows
DELETE FROM webhooks WHERE id = @id;

-- name: ClaimWebhook :one
UPDATE webhooks
SET lease_id = gen_random_uuid(),
  leased_by = @instance_id,
  lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision)
WHERE id = (
  SELECT w.id FROM webhooks w
  WHERE w.disabled_at IS NULL
    AND w.next_attempt_at <= clock_timestamp()
    AND (w.lease_until IS NULL OR w.lease_until < clock_timestamp())
    AND NOT (w.id = ANY(@done::uuid[]))
  ORDER BY w.next_attempt_at, w.id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING *;

-- name: RenewWebhookLease :one
UPDATE webhooks
SET lease_until = clock_timestamp() + make_interval(secs => @lease_seconds::double precision)
WHERE id = @id AND lease_id = @lease_id AND disabled_at IS NULL
RETURNING url, event_types, secret_ciphertext, owner_kind, owner_id;

-- name: AdvanceWebhook :execrows
UPDATE webhooks
SET cursor_xid = @cursor_xid,
  cursor_seq = @cursor_seq,
  consecutive_failures = 0,
  next_attempt_at = clock_timestamp()
WHERE id = @id AND lease_id = @lease_id;

-- name: FailWebhook :execrows
UPDATE webhooks
SET consecutive_failures = @consecutive_failures,
  next_attempt_at = @next_attempt_at,
  disabled_at = sqlc.narg('disabled_at'),
  disabled_reason = sqlc.narg('disabled_reason')
WHERE id = @id AND lease_id = @lease_id;

-- name: DisableLeasedWebhook :execrows
UPDATE webhooks
SET disabled_at = now(),
  disabled_reason = @reason,
  lease_id = NULL,
  leased_by = NULL,
  lease_until = NULL
WHERE id = @id AND lease_id = @lease_id;

-- name: ReleaseWebhook :execrows
UPDATE webhooks
SET lease_id = NULL, leased_by = NULL, lease_until = NULL
WHERE id = @id AND lease_id = @lease_id;

-- name: InsertWebhookDelivery :one
INSERT INTO webhook_deliveries (webhook_id, delivery_id, event_type, activity_seq, duration_ms, status_code, outcome, error_code)
VALUES (@webhook_id, @delivery_id, @event_type, sqlc.narg('activity_seq'), @duration_ms, sqlc.narg('status_code'), @outcome, sqlc.narg('error_code'))
RETURNING *;

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_deliveries
WHERE webhook_id = @webhook_id
  AND (sqlc.narg('before_id')::bigint IS NULL OR id < sqlc.narg('before_id')::bigint)
ORDER BY id DESC
LIMIT @row_limit;

-- name: PruneWebhookDeliveries :execrows
DELETE FROM webhook_deliveries
WHERE id IN (SELECT old.id FROM webhook_deliveries old WHERE old.attempted_at < @cutoff ORDER BY old.attempted_at, old.id LIMIT @batch);
