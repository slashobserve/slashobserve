-- name: ListSavedPrompts :many
SELECT p.id, p.owner_id, p.name, p.body, p.shared, p.created_at, p.updated_at,
       COALESCE(NULLIF(u.name, ''), u.email)::text AS owner_name
FROM saved_prompts p
JOIN users u ON u.id = p.owner_id
WHERE p.owner_id = @caller_id OR p.shared
ORDER BY (p.owner_id = @caller_id) DESC, lower(p.name), p.id
LIMIT 500;

-- name: FindSavedPromptByName :many
SELECT p.id, p.owner_id, p.name, p.body, p.shared, p.created_at, p.updated_at,
       COALESCE(NULLIF(u.name, ''), u.email)::text AS owner_name
FROM saved_prompts p
JOIN users u ON u.id = p.owner_id
WHERE lower(p.name) = lower(@name::text) AND (p.owner_id = @caller_id OR p.shared)
ORDER BY (p.owner_id = @caller_id) DESC
LIMIT 1;

-- name: GetVisibleSavedPromptForUpdate :one
SELECT p.id, p.owner_id, p.name, p.body, p.shared
FROM saved_prompts p
WHERE p.id = @id AND (p.owner_id = @caller_id OR p.shared)
FOR UPDATE;

-- name: LockSavedPromptOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended('saved_prompts:' || @owner_id::text, 0));

-- name: CountSavedPromptsByOwner :one
SELECT count(*) FROM saved_prompts WHERE owner_id = @owner_id;

-- name: InsertSavedPrompt :one
INSERT INTO saved_prompts (owner_id, name, body, shared)
VALUES (@owner_id, @name, @body, @shared)
RETURNING id, created_at, updated_at;

-- name: UpdateSavedPrompt :one
UPDATE saved_prompts p
SET name = @name, body = @body, shared = @shared, updated_at = now()
FROM users u
WHERE p.id = @id AND u.id = p.owner_id
RETURNING p.id, p.owner_id, p.name, p.body, p.shared, p.created_at, p.updated_at,
          COALESCE(NULLIF(u.name, ''), u.email)::text AS owner_name;

-- name: DeleteSavedPrompt :execrows
DELETE FROM saved_prompts WHERE id = @id;
