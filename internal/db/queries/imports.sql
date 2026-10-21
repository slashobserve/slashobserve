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
SELECT id, dashboard_id, version, importer_version, title, (report -> 'counts')::text AS counts,
  created_by_kind, created_by_id, created_by_name, created_by_email, created_at
FROM imports
WHERE (sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: GetImportByDashboard :one
SELECT id, dashboard_id, version, source, importer_version, mapping, summary, datasources, report, title,
  created_by_kind, created_by_id, created_by_name, created_by_email, created_at
FROM imports
WHERE dashboard_id = @dashboard_id;
