-- name: FolderFacts :many
SELECT f.id, f.title, f.general, f.personal, f.admins_only,
  coalesce(f.owner_id = sqlc.narg('user_id')::uuid, false)::boolean AS owner,
  EXISTS (SELECT 1 FROM folder_permissions p WHERE p.folder_id = f.id)::boolean AS listed,
  ARRAY(
    SELECT p.level FROM folder_permissions p
    WHERE p.folder_id = f.id AND (p.user_id = sqlc.narg('user_id')::uuid OR p.group_name = ANY(@groups::text[]))
  )::text[] AS grants,
  coalesce(f.created_by_kind = @kind::text AND f.created_by_id = @principal_id::uuid, false)::boolean AS creator
FROM folders f
WHERE NOT f.personal OR f.owner_id = sqlc.narg('user_id')::uuid
ORDER BY NOT f.personal, f.general, lower(f.title), f.id;

-- name: DashboardFolder :one
SELECT folder_id FROM dashboards WHERE id = @id;

-- name: DashboardPlacement :one
SELECT d.folder_id,
  EXISTS (SELECT 1 FROM starter_content s WHERE s.ref = d.id)::boolean AS provisioned,
  EXISTS (SELECT 1 FROM provisioned_dashboards p WHERE p.dashboard_id = d.id)::boolean AS from_file,
  EXISTS (SELECT 1 FROM applied_dashboards a WHERE a.dashboard_id = d.id)::boolean AS applied
FROM dashboards d WHERE d.id = @id;

-- name: DashboardManagersAmong :many
SELECT d.id,
  EXISTS (SELECT 1 FROM starter_content s WHERE s.ref = d.id)::boolean AS provisioned,
  EXISTS (SELECT 1 FROM provisioned_dashboards p WHERE p.dashboard_id = d.id)::boolean AS from_file,
  EXISTS (SELECT 1 FROM applied_dashboards a WHERE a.dashboard_id = d.id)::boolean AS applied
FROM dashboards d WHERE d.id = ANY(@ids::uuid[]);

-- name: ListFolderDetails :many
SELECT f.id, f.title, f.general, f.created_by_kind, f.created_by_id, f.created_by_name,
  (SELECT count(*) FROM dashboards d WHERE d.folder_id = f.id)::integer AS dashboard_count,
  (SELECT count(*) FROM folder_permissions p WHERE p.folder_id = f.id)::integer AS grantee_count,
  ARRAY(
    SELECT g.name FROM (
      SELECT 0 AS kind, coalesce(nullif(u.name, ''), u.email) AS name
      FROM folder_permissions p JOIN users u ON u.id = p.user_id
      WHERE p.folder_id = f.id
      UNION ALL
      SELECT 1, p.group_name FROM folder_permissions p
      WHERE p.folder_id = f.id AND p.group_name IS NOT NULL
    ) g
    ORDER BY g.kind, lower(g.name), g.name
    LIMIT 5
  )::text[] AS grantees
FROM folders f
WHERE f.id = ANY(@ids::uuid[])
ORDER BY NOT f.personal, f.general, lower(f.title), f.id;

-- name: GetFolder :one
SELECT * FROM folders WHERE id = @id;

-- name: LockFolder :one
SELECT * FROM folders WHERE id = @id FOR UPDATE;

-- name: CreateFolder :one
INSERT INTO folders (title, created_by_kind, created_by_id, created_by_name)
VALUES (@title, @created_by_kind, @created_by_id, @created_by_name)
RETURNING *;

-- name: RenameFolder :one
UPDATE folders SET title = @title, updated_at = now()
WHERE id = @id AND NOT general AND NOT personal
RETURNING *;

-- name: DeleteFolderIfEmpty :execrows
DELETE FROM folders
WHERE folders.id = @id AND NOT folders.general
  AND NOT EXISTS (SELECT 1 FROM dashboards d WHERE d.folder_id = folders.id)
  AND NOT EXISTS (SELECT 1 FROM saved_searches s WHERE s.folder_id = folders.id)
  AND NOT EXISTS (SELECT 1 FROM observations o WHERE o.folder_id = folders.id);

-- name: FolderHasContents :one
SELECT (EXISTS (SELECT 1 FROM dashboards d WHERE d.folder_id = @id)
  OR EXISTS (SELECT 1 FROM saved_searches s WHERE s.folder_id = @id)
  OR EXISTS (SELECT 1 FROM observations o WHERE o.folder_id = @id))::boolean;

-- name: ListFolderPermissions :many
SELECT p.user_id, p.group_name, p.level,
  coalesce(nullif(u.name, ''), u.email, '')::text AS user_name,
  coalesce(u.email, '')::text AS user_email,
  coalesce(u.role, '')::text AS user_role,
  CASE WHEN p.group_name IS NULL THEN 0
    ELSE (SELECT count(*) FROM users m WHERE p.group_name = ANY(m.groups)
      OR EXISTS (SELECT 1 FROM team_members tm JOIN teams t ON t.id = tm.team_id WHERE tm.user_id = m.id AND t.name = p.group_name))
  END::integer AS member_count
FROM folder_permissions p
LEFT JOIN users u ON u.id = p.user_id
WHERE p.folder_id = @folder_id
ORDER BY (p.group_name IS NOT NULL), lower(coalesce(nullif(u.name, ''), u.email, p.group_name)), p.created_at;

-- name: SetFolderAdminsOnly :exec
UPDATE folders SET admins_only = @admins_only, updated_at = now() WHERE id = @id;

-- name: ListFolderGrantsForAudit :many
SELECT p.user_id, p.group_name, p.level, coalesce(u.name, '')::text AS user_name
FROM folder_permissions p
LEFT JOIN users u ON u.id = p.user_id
WHERE p.folder_id = @folder_id
ORDER BY (p.group_name IS NOT NULL), p.group_name, p.user_id;

-- name: DeleteFolderPermissions :exec
DELETE FROM folder_permissions WHERE folder_id = @folder_id;

-- name: InsertFolderPermission :exec
INSERT INTO folder_permissions (folder_id, user_id, group_name, level, created_by_id)
VALUES (@folder_id, sqlc.narg('user_id'), sqlc.narg('group_name'), @level, sqlc.narg('created_by_id'));

-- name: UsersByEmail :many
SELECT id FROM users WHERE lower(email) = lower(@email::text);

-- name: FolderByTitle :one
SELECT id FROM folders WHERE lower(title) = lower(@title) AND NOT personal;
