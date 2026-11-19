-- name: RecordDashboardViewer :exec
INSERT INTO dashboard_viewers (dashboard_id, user_id, seen_at)
SELECT @dashboard_id::uuid, u.id, clock_timestamp() FROM users u WHERE u.id = @user_id::uuid
ON CONFLICT (dashboard_id, user_id) DO UPDATE SET seen_at = EXCLUDED.seen_at;

-- name: PruneDashboardViewers :exec
DELETE FROM dashboard_viewers WHERE seen_at < clock_timestamp() - interval '1 hour';

-- name: ListDashboardViewers :many
SELECT u.id, u.name, u.email
FROM dashboard_viewers v
JOIN users u ON u.id = v.user_id
WHERE v.dashboard_id = @dashboard_id::uuid
  AND v.seen_at > clock_timestamp() - make_interval(secs => @window_seconds::int)
ORDER BY lower(coalesce(nullif(u.name, ''), u.email)), u.id;
