-- name: GetHomeDashboard :one
SELECT h.dashboard_id, h.set_at, d.title AS dashboard_title
FROM home_dashboards h
LEFT JOIN dashboards d ON d.id = h.dashboard_id
WHERE h.user_id = @user_id;

-- name: SetHomeDashboard :exec
INSERT INTO home_dashboards (user_id, dashboard_id) VALUES (@user_id, @dashboard_id)
ON CONFLICT (user_id) DO UPDATE
SET dashboard_id = EXCLUDED.dashboard_id, set_at = now();

-- name: ClearHomeDashboard :exec
DELETE FROM home_dashboards WHERE user_id = @user_id;
