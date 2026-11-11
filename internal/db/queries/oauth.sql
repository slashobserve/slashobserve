-- name: CreateOAuthClient :one
INSERT INTO oauth_clients (name, redirect_uris)
SELECT sqlc.narg('name')::text, @redirect_uris::text[]
WHERE (
  SELECT count(*) FROM oauth_clients c
  WHERE NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.id)
) < @max_unused::bigint
RETURNING id, name, redirect_uris, registered_at;

-- name: GetOAuthClient :one
SELECT id, name, redirect_uris, registered_at FROM oauth_clients WHERE id = $1;

-- name: CreateOAuthRequest :one
INSERT INTO oauth_requests (client_id, redirect_uri, state, code_challenge, resource, expires_at)
SELECT @client_id::text, @redirect_uri::text, sqlc.narg('state')::text, @code_challenge::text, @resource::text, @expires_at::timestamptz
WHERE (SELECT count(*) FROM oauth_requests) < @max_outstanding::bigint
RETURNING id;

-- name: DeleteExpiredOAuthRequests :exec
DELETE FROM oauth_requests WHERE expires_at <= now();

-- name: GetOAuthRequest :one
SELECT r.id, r.client_id, r.redirect_uri, r.expires_at, c.name AS client_name, c.registered_at AS client_registered_at, c.redirect_uris AS client_redirect_uris
FROM oauth_requests r
JOIN oauth_clients c ON c.id = r.client_id
WHERE r.id = $1 AND r.expires_at > now();

-- name: ConsumeOAuthRequest :one
DELETE FROM oauth_requests
WHERE id = $1 AND expires_at > now()
RETURNING client_id, redirect_uri, state, code_challenge, resource;

-- name: LockOAuthUser :exec
SELECT pg_advisory_xact_lock(hashtextextended('oauth_user:' || CAST(@user_id::uuid AS text), 0));

-- name: CountOAuthSlots :one
SELECT ((SELECT count(*) FROM oauth_grants g
        WHERE g.user_id = @user_id AND g.revoked_at IS NULL AND g.expires_at > now())
     + (SELECT count(DISTINCT c.client_id) FROM oauth_codes c
        WHERE c.user_id = @user_id AND c.used_at IS NULL AND c.expires_at > now()
          AND NOT EXISTS (SELECT 1 FROM oauth_grants g
                          WHERE g.client_id = c.client_id AND g.user_id = c.user_id
                            AND g.revoked_at IS NULL AND g.expires_at > now())))::bigint AS slots;

-- name: HoldsOAuthSlot :one
SELECT (EXISTS (
  SELECT 1 FROM oauth_grants g
  WHERE g.client_id = @client_id AND g.user_id = @user_id AND g.revoked_at IS NULL AND g.expires_at > now()
) OR EXISTS (
  SELECT 1 FROM oauth_codes c
  WHERE c.client_id = @client_id AND c.user_id = @user_id AND c.used_at IS NULL AND c.expires_at > now()
))::boolean AS holds;

-- name: HasLiveOAuthGrant :one
SELECT EXISTS (
  SELECT 1 FROM oauth_grants
  WHERE client_id = @client_id AND user_id = @user_id AND revoked_at IS NULL AND expires_at > now()
);

-- name: CreateOAuthCode :exec
INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, expires_at)
VALUES (@code_hash, @client_id, @user_id, @redirect_uri, @code_challenge, @resource, @expires_at);

-- name: UseOAuthCode :one
UPDATE oauth_codes SET used_at = now(), grant_id = @grant_id
WHERE code_hash = @code_hash AND used_at IS NULL AND expires_at > now()
RETURNING client_id, user_id, redirect_uri, code_challenge, resource;

-- name: GetOAuthCode :one
SELECT client_id, user_id, redirect_uri, code_challenge, resource, expires_at, used_at, grant_id, now()::timestamptz AS db_now
FROM oauth_codes
WHERE code_hash = $1
FOR UPDATE;

-- name: ReplaceLiveOAuthGrant :exec
UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'replaced'
WHERE client_id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: CreateOAuthGrant :one
INSERT INTO oauth_grants (client_id, user_id, resource, expires_at)
VALUES (@client_id, @user_id, @resource, @expires_at)
RETURNING id, created_at, expires_at;

-- name: CreateOAuthToken :exec
INSERT INTO oauth_tokens (token_hash, grant_id, kind, expires_at)
VALUES (@token_hash, @grant_id, @kind, @expires_at);

-- name: SetOAuthGrantExpiry :exec
UPDATE oauth_grants SET expires_at = @expires_at WHERE id = @id;

-- name: GetOAuthRefresh :one
SELECT t.grant_id, t.expires_at, t.rotated_at, g.client_id, g.user_id, g.resource, g.created_at, g.revoked_at, now()::timestamptz AS db_now
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
WHERE t.token_hash = $1 AND t.kind = 'refresh'
FOR UPDATE OF t, g;

-- name: RotateOAuthRefresh :exec
UPDATE oauth_tokens SET rotated_at = now() WHERE token_hash = $1;

-- name: GetOAuthAccess :one
SELECT g.id, g.user_id, g.last_used_at, c.name
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
JOIN oauth_clients c ON c.id = g.client_id
WHERE t.token_hash = @token_hash
  AND t.kind = 'access'
  AND t.expires_at > now()
  AND g.revoked_at IS NULL
  AND g.expires_at > now()
  AND g.resource = @resource;

-- name: TouchOAuthGrant :exec
UPDATE oauth_grants SET last_used_at = now()
WHERE id = @id AND (last_used_at IS NULL OR last_used_at < now() - @min_age::interval);

-- name: GetOAuthTokenGrant :one
SELECT t.grant_id, g.client_id
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
WHERE t.token_hash = $1;

-- name: RevokeOAuthGrant :execrows
UPDATE oauth_grants SET revoked_at = now(), revoked_reason = @reason::text, revoked_by = sqlc.narg('by')::uuid
WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeUserOAuthGrants :execrows
UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'admin', revoked_by = sqlc.narg('by')::uuid
WHERE user_id = @user_id AND revoked_at IS NULL;

-- name: RevokeUserOAuthGrant :execrows
UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'person', revoked_by = @user_id
WHERE id = @id AND user_id = @user_id AND revoked_at IS NULL;

-- name: ListUserOAuthGrants :many
SELECT g.id, g.client_id, c.name AS client_name, c.redirect_uris AS client_redirect_uris,
       g.user_id, u.name AS user_name, u.email AS user_email,
       g.created_at, g.last_used_at, g.expires_at
FROM oauth_grants g
JOIN oauth_clients c ON c.id = g.client_id
JOIN users u ON u.id = g.user_id
WHERE g.user_id = @user_id
  AND g.revoked_at IS NULL
  AND g.expires_at > now()
  AND (sqlc.narg('before_created')::timestamptz IS NULL
       OR (g.created_at, g.id) < (sqlc.narg('before_created')::timestamptz, sqlc.narg('before_id')::uuid))
ORDER BY g.created_at DESC, g.id DESC
LIMIT @lim;

-- name: ListOAuthGrants :many
SELECT g.id, g.client_id, c.name AS client_name, c.redirect_uris AS client_redirect_uris,
       g.user_id, u.name AS user_name, u.email AS user_email,
       g.created_at, g.last_used_at, g.expires_at
FROM oauth_grants g
JOIN oauth_clients c ON c.id = g.client_id
JOIN users u ON u.id = g.user_id
WHERE g.revoked_at IS NULL
  AND g.expires_at > now()
  AND (sqlc.narg('before_created')::timestamptz IS NULL
       OR (g.created_at, g.id) < (sqlc.narg('before_created')::timestamptz, sqlc.narg('before_id')::uuid))
ORDER BY g.created_at DESC, g.id DESC
LIMIT @lim;

-- name: PruneOAuthCodes :execrows
DELETE FROM oauth_codes
WHERE code_hash IN (
  SELECT old.code_hash FROM oauth_codes old
  WHERE old.expires_at < now() - interval '10 minutes'
  LIMIT 10000
);

-- name: PruneOAuthTokens :execrows
DELETE FROM oauth_tokens
WHERE token_hash IN (
  SELECT old.token_hash FROM oauth_tokens old
  WHERE old.expires_at < now()
  LIMIT 10000
);

-- name: PruneOAuthGrants :execrows
DELETE FROM oauth_grants
WHERE id IN (
  SELECT old.id FROM oauth_grants old
  WHERE coalesce(old.revoked_at, old.expires_at) < now() - interval '30 days'
  LIMIT 10000
);

-- name: PruneOAuthClients :execrows
DELETE FROM oauth_clients
WHERE id IN (
  SELECT old.id FROM oauth_clients old
  WHERE old.registered_at < now() - interval '24 hours'
    AND NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = old.id)
    AND NOT EXISTS (SELECT 1 FROM oauth_codes c WHERE c.client_id = old.id)
    AND NOT EXISTS (SELECT 1 FROM oauth_requests r WHERE r.client_id = old.id)
  LIMIT 10000
);
