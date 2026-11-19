-- name: UpsertUser :one
INSERT INTO users (oidc_issuer, oidc_subject, email, name, groups)
VALUES (@oidc_issuer, @oidc_subject, @email, @name, coalesce(@groups::text[], '{}'))
ON CONFLICT (oidc_issuer, oidc_subject)
DO UPDATE SET email = EXCLUDED.email, name = EXCLUDED.name, groups = EXCLUDED.groups,
  last_login_at = CASE WHEN users.deactivated_at IS NULL THEN now() ELSE users.last_login_at END,
  last_seen_at = CASE WHEN users.deactivated_at IS NULL THEN now() ELSE users.last_seen_at END
RETURNING id, email, name, created_at, manual_role, deactivated_at, (xmax = 0)::boolean AS inserted;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSessionUser :one
SELECT u.id, u.email, u.name, u.last_seen_at, u.role, (u.groups || ARRAY(SELECT t.name FROM team_members tm JOIN teams t ON t.id = tm.team_id WHERE tm.user_id = u.id ORDER BY t.name))::text[] AS groups
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now() AND u.deactivated_at IS NULL;

-- name: TouchUserSeen :exec
UPDATE users SET last_seen_at = now()
WHERE id = @id AND last_seen_at < now() - @min_age::interval;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();

-- name: CreateLoginState :execrows
INSERT INTO oidc_login_states (state_hash, nonce, code_verifier, return_to, expires_at)
SELECT @state_hash, @nonce, @code_verifier, @return_to, @expires_at
WHERE (SELECT count(*) FROM oidc_login_states) < @max_outstanding::bigint;

-- name: ConsumeLoginState :one
DELETE FROM oidc_login_states
WHERE state_hash = $1 AND expires_at > now()
RETURNING nonce, code_verifier, return_to;

-- name: DeleteExpiredLoginStates :exec
DELETE FROM oidc_login_states WHERE expires_at <= now();
