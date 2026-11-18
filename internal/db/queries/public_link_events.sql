-- name: GetPublicLinkEvent :one
SELECT p.display_prefix, p.expires_at, p.dashboard_id, r.observation_id, COALESCE(d.title, r.title, '')::text AS title
FROM public_links p
LEFT JOIN dashboards d ON d.id = p.dashboard_id
LEFT JOIN observation_rcas r ON r.id = p.rca_id
WHERE p.id = @id;
