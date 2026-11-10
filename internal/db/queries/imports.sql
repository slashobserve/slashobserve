-- name: LockImportGrafanaUID :exec
SELECT pg_advisory_xact_lock(hashtextextended('imports_grafana_uid:' || @grafana_uid::text, 0));

-- name: InsertImport :one
INSERT INTO imports (
  dashboard_id, version, source, importer_version, original, mapping, summary, datasources, report,
  title, created_by_kind, created_by_id, created_by_name, created_by_email, grafana_uid
) VALUES (
  @dashboard_id, @version, @source, @importer_version, @original, @mapping, @summary, @datasources, @report,
  @title, @created_by_kind, @created_by_id, @created_by_name, @created_by_email,
  CASE WHEN @grafana_uid::text ~ '^[A-Za-z0-9_-]{1,40}$'
    AND NOT EXISTS (SELECT 1 FROM imports claimed WHERE claimed.grafana_uid = @grafana_uid::text)
  THEN @grafana_uid::text ELSE '' END
)
RETURNING id, created_at, grafana_uid;

-- name: UpdateImport :execrows
UPDATE imports
SET version = @version, importer_version = @importer_version, original = @original, mapping = @mapping,
  summary = @summary, datasources = @datasources, report = @report, title = @title
WHERE dashboard_id = @dashboard_id;

-- name: DashboardByGrafanaUID :one
SELECT dashboard_id,
  CASE WHEN jsonb_typeof(summary -> 'grafana_version') = 'number'
    THEN (summary ->> 'grafana_version')::numeric::bigint ELSE 0 END::bigint AS grafana_version
FROM imports
WHERE grafana_uid = @grafana_uid::text AND grafana_uid <> '';

-- name: ListImports :many
SELECT i.id, i.dashboard_id, i.version, i.importer_version, i.title, (i.report -> 'counts')::text AS counts,
  i.created_by_kind, i.created_by_id, i.created_by_name, i.created_by_email, i.created_at
FROM imports i
JOIN dashboards d ON d.id = i.dashboard_id
WHERE d.folder_id = ANY(@folders::uuid[])
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (i.created_at, i.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
  AND (sqlc.narg('made_by_kind')::text IS NULL
    OR (i.created_by_kind = sqlc.narg('made_by_kind')::text AND i.created_by_id = sqlc.narg('made_by_id')::uuid))
  AND (sqlc.narg('dashboard_id')::uuid IS NULL OR i.dashboard_id = sqlc.narg('dashboard_id')::uuid)
ORDER BY i.created_at DESC, i.id DESC
LIMIT @row_limit;

-- name: GetImportByDashboard :one
SELECT i.id, i.dashboard_id, i.version, i.source, i.importer_version, i.mapping, i.summary, i.datasources, i.report, i.title,
  i.created_by_kind, i.created_by_id, i.created_by_name, i.created_by_email, i.created_at, i.grafana_uid,
  v.origin, v.origin_client, v.agent_request_id
FROM imports i
JOIN dashboard_versions v ON v.dashboard_id = i.dashboard_id AND v.version = i.version
WHERE i.dashboard_id = @dashboard_id;
