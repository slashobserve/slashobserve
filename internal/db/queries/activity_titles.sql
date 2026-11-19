-- name: GetThreadEventTitle :one
SELECT COALESCE(d.title, o.title, '')::text AS title
FROM (SELECT sqlc.narg('dashboard_id')::uuid AS dashboard_id, sqlc.narg('observation_id')::uuid AS observation_id) x
LEFT JOIN dashboards d ON d.id = x.dashboard_id
LEFT JOIN observations o ON o.id = x.observation_id;
