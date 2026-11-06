-- name: DashboardsDueForHealth :many
SELECT d.id, d.latest_version, v.spec
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
LEFT JOIN dashboard_health h ON h.dashboard_id = d.id
WHERE h.checked_at IS NULL OR h.checked_at < @checked_before::timestamptz
ORDER BY h.checked_at ASC NULLS FIRST, d.id
LIMIT @row_limit;

-- name: UpsertDashboardHealth :exec
INSERT INTO dashboard_health (dashboard_id, version, checked_at, skipped, issues)
VALUES (@dashboard_id, @version, @checked_at, @skipped, @issues)
ON CONFLICT (dashboard_id) DO UPDATE SET
  version = EXCLUDED.version,
  checked_at = EXCLUDED.checked_at,
  skipped = EXCLUDED.skipped,
  issues = EXCLUDED.issues;

-- name: ListHealth :many
SELECT d.id, d.title, d.created_at, d.updated_at, d.folder_id,
  f.title AS folder_title, f.general AS folder_general, f.personal AS folder_personal,
  h.version AS checked_version, h.checked_at, coalesce(h.skipped, '')::text AS skipped,
  coalesce(h.issues, '[]'::jsonb)::jsonb AS issues,
  lv.viewed_at::timestamptz AS last_viewed_at,
  dup.duplicate::boolean AS duplicate_title
FROM dashboards d
JOIN folders f ON f.id = d.folder_id
LEFT JOIN dashboard_health h ON h.dashboard_id = d.id
LEFT JOIN LATERAL (
  SELECT max(dv.viewed_at) AS viewed_at FROM dashboard_views dv WHERE dv.dashboard_id = d.id
) lv ON true
CROSS JOIN LATERAL (
  SELECT EXISTS (
    SELECT 1 FROM dashboards o
    WHERE o.folder_id = d.folder_id AND o.id <> d.id AND lower(o.title) = lower(d.title)
  ) AS duplicate
) dup
WHERE d.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('folder')::uuid IS NULL OR d.folder_id = sqlc.narg('folder')::uuid)
  AND (
    (@want_checked::boolean AND (
      CASE WHEN @checked_kind::text = '' THEN jsonb_array_length(coalesce(h.issues, '[]'::jsonb)) > 0
      ELSE coalesce(h.issues, '[]'::jsonb) @> jsonb_build_array(jsonb_build_object('kind', @checked_kind::text)) END))
    OR (@want_duplicate::boolean AND dup.duplicate)
    OR (@want_unopened::boolean AND coalesce(lv.viewed_at, d.created_at) < @unopened_before::timestamptz)
  )
  AND (sqlc.narg('cursor_updated_at')::timestamptz IS NULL
    OR (d.updated_at, d.id) < (sqlc.narg('cursor_updated_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY d.updated_at DESC, d.id DESC
LIMIT @row_limit;

-- name: RecordDashboardView :exec
INSERT INTO dashboard_views (user_id, dashboard_id, viewed_at)
VALUES (@user_id, @dashboard_id, now())
ON CONFLICT (user_id, dashboard_id) DO UPDATE SET viewed_at = EXCLUDED.viewed_at
WHERE dashboard_views.viewed_at < now() - interval '10 minutes';

-- name: HealthRepairTarget :one
SELECT d.folder_id, d.latest_version, coalesce(h.issues, '[]'::jsonb)::jsonb AS issues
FROM dashboards d
LEFT JOIN dashboard_health h ON h.dashboard_id = d.id
WHERE d.id = @dashboard_id;

-- name: CountActiveHealthRepairsBy :one
SELECT count(*) FROM agent_requests
WHERE requested_by_id = @requester_id AND source = 'prompt' AND starts_with(prompt, @prompt_prefix::text)
  AND status IN ('queued', 'running');
