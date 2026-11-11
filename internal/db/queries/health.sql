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
SELECT r.id, r.title, r.created_at, r.folder_id, r.folder_title, r.folder_general, r.folder_personal,
  r.checked_version, r.checked_at, r.skipped, r.issues, r.last_viewed_at, r.duplicate_title,
  r.rank::integer AS rank, r.seen_at::timestamptz AS seen_at,
  dd.latest_version AS latest_version,
  rep.id AS repair_id, coalesce(rep.status, '')::text AS repair_status, coalesce(rep.state, '')::text AS repair_state,
  rep.result_version AS repair_version, coalesce(rep.error, '')::text AS repair_error,
  rep.updated_at AS repair_updated_at
FROM (
  SELECT s.*,
    CASE WHEN s.query_failed OR s.variable_failed THEN 0 WHEN s.no_data THEN 1
      WHEN s.duplicate_title THEN 2 ELSE 3 END AS rank,
    coalesce(s.last_viewed_at, 'epoch'::timestamptz) AS seen_at
  FROM dashboard_health_status s
  WHERE s.folder_id = ANY(@folders::uuid[])
    AND (sqlc.narg('folder')::uuid IS NULL OR s.folder_id = sqlc.narg('folder')::uuid)
    AND CASE @kind::text
      WHEN '' THEN s.query_failed OR s.no_data OR s.variable_failed OR s.duplicate_title
        OR coalesce(s.last_viewed_at, s.created_at) < @unopened_before::timestamptz
      WHEN 'query_failed' THEN s.query_failed
      WHEN 'no_data' THEN s.no_data
      WHEN 'variable_failed' THEN s.variable_failed
      WHEN 'duplicate_title' THEN s.duplicate_title
      WHEN 'unopened' THEN coalesce(s.last_viewed_at, s.created_at) < @unopened_before::timestamptz
      ELSE false END
) r
JOIN dashboards dd ON dd.id = r.id
LEFT JOIN LATERAL (
  SELECT a.id, a.status, a.state, a.result_version, a.error,
    coalesce(a.finished_at, a.started_at, a.created_at) AS updated_at
  FROM agent_requests a
  WHERE a.dashboard_id = r.id AND a.source = 'prompt' AND starts_with(a.prompt, @repair_prefix::text)
  ORDER BY a.created_at DESC, a.id DESC
  LIMIT 1
) rep ON true
WHERE sqlc.narg('cursor_rank')::integer IS NULL
  OR r.rank > sqlc.narg('cursor_rank')::integer
  OR (r.rank = sqlc.narg('cursor_rank')::integer AND (
       r.seen_at < sqlc.narg('cursor_seen')::timestamptz
       OR (r.seen_at = sqlc.narg('cursor_seen')::timestamptz
           AND (r.title, r.id) > (sqlc.narg('cursor_title')::text, sqlc.narg('cursor_id')::uuid))))
ORDER BY r.rank, r.seen_at DESC, r.title, r.id
LIMIT @row_limit;

-- name: HealthSummary :one
SELECT
  count(*) FILTER (WHERE s.query_failed OR s.no_data OR s.variable_failed OR s.duplicate_title
    OR coalesce(s.last_viewed_at, s.created_at) < @unopened_before::timestamptz) AS dashboards,
  count(*) FILTER (WHERE s.query_failed OR s.variable_failed) AS failing,
  count(*) FILTER (WHERE s.query_failed) AS query_failed,
  count(*) FILTER (WHERE s.variable_failed) AS variable_failed,
  count(*) FILTER (WHERE s.no_data) AS no_data,
  count(*) FILTER (WHERE s.duplicate_title) AS duplicate_titles,
  count(*) FILTER (WHERE coalesce(s.last_viewed_at, s.created_at) < @unopened_before::timestamptz) AS unopened
FROM dashboard_health_status s
WHERE s.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('folder')::uuid IS NULL OR s.folder_id = sqlc.narg('folder')::uuid);

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
