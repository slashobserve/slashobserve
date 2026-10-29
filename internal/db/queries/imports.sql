-- name: InsertImport :one
INSERT INTO imports (
  dashboard_id, version, source, importer_version, original, mapping, summary, datasources, report,
  title, created_by_kind, created_by_id, created_by_name, created_by_email
) VALUES (
  @dashboard_id, @version, @source, @importer_version, @original, @mapping, @summary, @datasources, @report,
  @title, @created_by_kind, @created_by_id, @created_by_name, @created_by_email
)
RETURNING id, created_at;

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
SELECT id, dashboard_id, version, source, importer_version, mapping, summary, datasources, report, title,
  created_by_kind, created_by_id, created_by_name, created_by_email, created_at
FROM imports
WHERE dashboard_id = @dashboard_id;
