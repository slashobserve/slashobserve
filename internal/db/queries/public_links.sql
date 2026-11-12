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

-- name: CreateRCAPublicLink :one
INSERT INTO public_links (rca_id, token_hash, display_prefix, variables, selections, expires_at, created_by_kind, created_by_id, created_by_name)
VALUES (@rca_id, @token_hash, @display_prefix, 'frozen', '{}', sqlc.narg('expires_at')::timestamptz, @created_by_kind, @created_by_id, @created_by_name)
RETURNING id, rca_id, display_prefix, expires_at, created_by_kind, created_by_id, created_by_name, created_at, last_viewed_at;

-- name: ListRCAPublicLinks :many
SELECT id, rca_id, display_prefix, expires_at, created_by_kind, created_by_id, created_by_name, created_at, last_viewed_at
FROM public_links
WHERE rca_id = @rca_id
ORDER BY created_at DESC, id DESC;

-- name: CountObservationPublicLinks :one
SELECT count(*)::int
FROM public_links p
JOIN observation_rcas r ON r.id = p.rca_id
WHERE r.observation_id = @observation_id;

-- name: DeleteRCAPublicLink :execrows
DELETE FROM public_links p
WHERE p.id = @link_id AND p.rca_id IN (SELECT r.id FROM observation_rcas r WHERE r.observation_id = @observation_id);

-- name: RCAPublicLinkByTokenHash :one
SELECT p.id AS link_id, p.expires_at, r.id AS rca_id, r.observation_id, r.revision, r.title, r.summary, r.observation_state,
  r.time_from, r.time_to, r.content, r.created_by_name, r.created_at, f.personal
FROM public_links p
JOIN observation_rcas r ON r.id = p.rca_id
JOIN observations o ON o.id = r.observation_id
JOIN folders f ON f.id = o.folder_id
WHERE p.token_hash = @token_hash;
