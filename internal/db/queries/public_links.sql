-- name: CreatePublicLink :one
INSERT INTO public_links (dashboard_id, token_hash, display_prefix, variables, selections, expires_at, created_by_kind, created_by_id, created_by_name)
VALUES (@dashboard_id, @token_hash, @display_prefix, @variables, @selections, sqlc.narg('expires_at')::timestamptz, @created_by_kind, @created_by_id, @created_by_name)
RETURNING id, dashboard_id, display_prefix, variables, selections, expires_at, created_by_kind, created_by_id, created_by_name, created_at, last_viewed_at;

-- name: ListPublicLinks :many
SELECT id, dashboard_id, display_prefix, variables, selections, expires_at, created_by_kind, created_by_id, created_by_name, created_at, last_viewed_at
FROM public_links
WHERE dashboard_id = @dashboard_id
ORDER BY created_at DESC, id DESC
LIMIT 100;

-- name: CountPublicLinks :one
SELECT count(*)::int FROM public_links WHERE dashboard_id = @dashboard_id;

-- name: DeletePublicLink :execrows
DELETE FROM public_links WHERE id = @id AND dashboard_id = @dashboard_id;

-- name: PublicLinkByTokenHash :one
SELECT p.id, p.dashboard_id, p.variables, p.selections, p.expires_at, d.latest_version
FROM public_links p
JOIN dashboards d ON d.id = p.dashboard_id
WHERE p.token_hash = @token_hash;

-- name: TouchPublicLink :exec
UPDATE public_links SET last_viewed_at = now()
WHERE id = @id AND (last_viewed_at IS NULL OR last_viewed_at < now() - interval '1 minute');
