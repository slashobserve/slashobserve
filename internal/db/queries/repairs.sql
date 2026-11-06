-- name: ListRepairs :many
SELECT d.id, d.title, d.updated_at, d.folder_id,
  f.title AS folder_title, f.general AS folder_general, f.personal AS folder_personal,
  c.unverified_panels::integer AS unverified_panels,
  c.unverified_variables::integer AS unverified_variables,
  c.placeholder_panels::integer AS placeholder_panels,
  iv.created_at AS last_import_at,
  r.id AS repair_id, coalesce(r.status, '')::text AS repair_status, r.finished_at AS repair_finished_at
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
JOIN folders f ON f.id = d.folder_id
CROSS JOIN LATERAL (
  SELECT
    CASE WHEN jsonb_typeof(v.unverified -> 'panels') = 'array' THEN jsonb_array_length(v.unverified -> 'panels') ELSE 0 END AS unverified_panels,
    CASE WHEN jsonb_typeof(v.unverified -> 'variables') = 'array' THEN jsonb_array_length(v.unverified -> 'variables') ELSE 0 END AS unverified_variables,
    (SELECT count(*) FROM jsonb_path_query(v.spec, 'lax $.panels[*] ? (@.type == "placeholder")'))
      + (SELECT count(*) FROM jsonb_path_query(v.spec, 'lax $.panels[*].panels[*] ? (@.type == "placeholder")')) AS placeholder_panels
) c
LEFT JOIN imports i ON i.dashboard_id = d.id
LEFT JOIN dashboard_versions iv ON iv.dashboard_id = i.dashboard_id AND iv.version = i.version
LEFT JOIN LATERAL (
  SELECT a.id, a.status, a.finished_at FROM agent_requests a
  WHERE a.dashboard_id = d.id AND a.source = 'prompt' AND a.prompt = @repair_prompt::text
  ORDER BY a.created_at DESC, a.id DESC
  LIMIT 1
) r ON true
WHERE d.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('folder')::uuid IS NULL OR d.folder_id = sqlc.narg('folder')::uuid)
  AND c.unverified_panels + c.unverified_variables + c.placeholder_panels > 0
  AND (sqlc.narg('cursor_updated_at')::timestamptz IS NULL
    OR (d.updated_at, d.id) < (sqlc.narg('cursor_updated_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY d.updated_at DESC, d.id DESC
LIMIT @row_limit;

-- name: RepairTarget :one
SELECT d.folder_id, d.latest_version, v.unverified
FROM dashboards d
JOIN dashboard_versions v ON v.dashboard_id = d.id AND v.version = d.latest_version
WHERE d.id = @dashboard_id;

-- name: CountActiveRepairsBy :one
SELECT count(*) FROM agent_requests
WHERE requested_by_id = @requester_id AND source = 'prompt' AND prompt = @repair_prompt::text
  AND status IN ('queued', 'running');
