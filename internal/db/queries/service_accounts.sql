-- name: CreateServiceAccount :one
INSERT INTO service_accounts (name, description, created_by, role) VALUES (@name, @description, @created_by, @role)
RETURNING id, name, description, created_by, created_at, role;

-- name: ListServiceAccounts :many
SELECT id, name, description, created_by, created_at, role FROM service_accounts ORDER BY name;

-- name: GetServiceAccount :one
SELECT id, name, description, created_by, created_at, role FROM service_accounts WHERE id = $1;

-- name: SetServiceAccountRole :one
UPDATE service_accounts SET role = @role WHERE id = @id
RETURNING id, name, description, created_by, created_at, role;

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
