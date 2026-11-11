-- name: CreateServiceAccount :one
INSERT INTO service_accounts (name, description, created_by, role) VALUES (@name, @description, @created_by, @role)
RETURNING id, name, description, created_by, created_at, role, provisioned, takes_runs;

-- name: ListServiceAccounts :many
SELECT id, name, description, created_by, created_at, role, provisioned, takes_runs FROM service_accounts ORDER BY name;

-- name: GetServiceAccount :one
SELECT id, name, description, created_by, created_at, role, provisioned, takes_runs FROM service_accounts WHERE id = $1;

-- name: UpdateServiceAccount :one
UPDATE service_accounts
SET role = COALESCE(sqlc.narg('role')::text, role),
    takes_runs = COALESCE(sqlc.narg('takes_runs')::boolean, takes_runs)
WHERE id = @id
RETURNING id, name, description, created_by, created_at, role, provisioned, takes_runs;

-- name: ListServiceAccountPresence :many
SELECT s.id, w.seen_at AS worker_seen_at,
       EXISTS (
         SELECT 1 FROM agent_requests r
         WHERE r.status = 'running' AND r.claimed_by = 'service_account:' || s.id::text AND r.lease_until > clock_timestamp()
       ) AS worker_working,
       clock_timestamp()::timestamptz AS checked_at
FROM service_accounts s
LEFT JOIN agent_workers w ON w.id = 'service_account:' || s.id::text
WHERE s.id = ANY(@ids::uuid[]);

-- name: CreateServiceAccountKey :one
INSERT INTO service_account_keys (service_account_id, name, display_prefix, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, service_account_id, name, display_prefix, created_by, created_at, expires_at, last_used_at, revoked_at;

-- name: ListServiceAccountKeys :many
SELECT id, service_account_id, name, display_prefix, created_by, created_at, expires_at, last_used_at, revoked_at
FROM service_account_keys WHERE service_account_id = $1 ORDER BY created_at DESC;

-- name: RevokeServiceAccountKey :execrows
UPDATE service_account_keys SET revoked_at = COALESCE(revoked_at, now())
WHERE id = $1 AND service_account_id = $2;

-- name: GetKeyPrincipal :one
SELECT k.id AS key_id, sa.id AS service_account_id, sa.name, sa.role, k.last_used_at
FROM service_account_keys k JOIN service_accounts sa ON sa.id = k.service_account_id
WHERE k.token_hash = $1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at > now());

-- name: TouchServiceAccountKey :exec
UPDATE service_account_keys SET last_used_at = now()
WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes');

-- name: GetServiceAccountPrincipal :one
SELECT id, name, role FROM service_accounts WHERE id = $1;

-- name: GetServiceAccountByName :one
SELECT id, name, description, created_by, created_at, role, provisioned, takes_runs FROM service_accounts WHERE name = $1;

-- name: UpsertProvisionedServiceAccount :one
INSERT INTO service_accounts (name, description, role, takes_runs, provisioned) VALUES (@name, @description, @role, @takes_runs, true)
ON CONFLICT (name) DO UPDATE SET role = EXCLUDED.role, description = EXCLUDED.description, takes_runs = EXCLUDED.takes_runs
WHERE service_accounts.provisioned
RETURNING id, name, description, created_by, created_at, role, provisioned, takes_runs;

-- name: ActiveKeyByName :one
SELECT id, token_hash FROM service_account_keys
WHERE service_account_id = @service_account_id AND name = @name
  AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC
LIMIT 1;

-- name: ReplaceProvisionedKey :exec
WITH revoked AS (
  UPDATE service_account_keys SET revoked_at = now()
  WHERE service_account_id = @service_account_id AND name = @name AND revoked_at IS NULL
)
INSERT INTO service_account_keys (service_account_id, name, display_prefix, token_hash)
VALUES (@service_account_id, @name, @display_prefix, @token_hash);
