-- name: StarDashboard :exec
INSERT INTO dashboard_stars (user_id, dashboard_id) VALUES (@user_id, @dashboard_id)
ON CONFLICT (user_id, dashboard_id) DO NOTHING;

-- name: UnstarDashboard :exec
DELETE FROM dashboard_stars WHERE user_id = @user_id AND dashboard_id = @dashboard_id;

-- name: ListStars :many
SELECT d.id, d.title, d.folder_id, f.title AS folder_title, s.starred_at
FROM dashboard_stars s
JOIN dashboards d ON d.id = s.dashboard_id
JOIN folders f ON f.id = d.folder_id
WHERE s.user_id = @user_id
  AND d.folder_id = ANY(@folders::uuid[])
ORDER BY s.starred_at DESC, d.id
LIMIT 200;
