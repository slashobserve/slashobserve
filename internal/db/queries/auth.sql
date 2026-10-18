-- name: UpsertUser :one
INSERT INTO users (oidc_issuer, oidc_subject, email, name)
VALUES ($1, $2, $3, $4)
ON CONFLICT (oidc_issuer, oidc_subject)
DO UPDATE SET email = EXCLUDED.email, name = EXCLUDED.name, last_login_at = now(), last_seen_at = now()
RETURNING id, email, name;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSessionUser :one
SELECT u.id, u.email, u.name, u.last_seen_at
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: TouchUserSeen :exec
UPDATE users SET last_seen_at = now()
WHERE id = $1 AND last_seen_at < now() - interval '1 minute';

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();

-- name: CreateLoginState :exec
INSERT INTO oidc_login_states (state_hash, nonce, code_verifier, return_to, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeLoginState :one
DELETE FROM oidc_login_states
WHERE state_hash = $1 AND expires_at > now()
RETURNING nonce, code_verifier, return_to;

-- name: DeleteExpiredLoginStates :exec
DELETE FROM oidc_login_states WHERE expires_at <= now();
