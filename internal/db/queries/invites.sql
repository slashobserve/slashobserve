-- name: LockInviteRole :one
SELECT role FROM user_invites WHERE lower(email) = lower(@email::text) FOR UPDATE;

-- name: UpsertInvite :one
INSERT INTO user_invites (email, role, created_by_kind, created_by_id, created_by_name)
VALUES (@email, @role, @created_by_kind, @created_by_id, @created_by_name)
ON CONFLICT ((lower(email))) DO UPDATE SET role = EXCLUDED.role, updated_at = now()
RETURNING id, email, role, created_by_name, created_at, updated_at, (xmax = 0)::boolean AS inserted;

-- name: ListInvites :many
SELECT i.id, i.email, i.role, i.created_by_name, i.created_at, i.updated_at,
  EXISTS (SELECT 1 FROM users u WHERE lower(u.email) = lower(i.email))::boolean AS member
FROM user_invites i
WHERE sqlc.narg('cursor_created_at')::timestamptz IS NULL
  OR (i.created_at, i.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
ORDER BY i.created_at DESC, i.id DESC
LIMIT @row_limit;

-- name: DeleteInvite :one
DELETE FROM user_invites WHERE id = @id RETURNING email;

-- name: EmailHasSignedIn :one
SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower(@email::text));

-- name: AcceptInvite :one
WITH invite AS (
  DELETE FROM user_invites WHERE lower(email) = lower(@email::text)
  RETURNING role, created_by_kind, created_by_id, created_by_name
)
UPDATE users u SET manual_role = invite.role,
  manual_role_by_id = CASE WHEN invite.created_by_kind = 'user' AND EXISTS (SELECT 1 FROM users b WHERE b.id = invite.created_by_id) THEN invite.created_by_id END,
  manual_role_by_name = nullif(invite.created_by_name, '')
FROM invite
WHERE u.id = @id
RETURNING u.manual_role;
