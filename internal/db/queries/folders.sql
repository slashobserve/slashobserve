-- name: FolderFacts :many
SELECT f.id, f.title, f.general,
  EXISTS (SELECT 1 FROM folder_permissions p WHERE p.folder_id = f.id) AS restricted,
  ARRAY(
    SELECT p.level FROM folder_permissions p
    WHERE p.folder_id = f.id AND (p.user_id = sqlc.narg('user_id')::uuid OR p.group_name = ANY(@groups::text[]))
  )::text[] AS grants,
  coalesce(f.created_by_kind = @kind::text AND f.created_by_id = @principal_id::uuid, false)::boolean AS creator
FROM folders f
ORDER BY f.general, lower(f.title), f.id;

-- name: DashboardFolder :one
SELECT folder_id FROM dashboards WHERE id = @id;

-- name: LastCreatedFolder :one
SELECT folder_id FROM dashboards
WHERE created_by_kind = @kind AND created_by_id = @id
ORDER BY created_at DESC, id DESC
LIMIT 1;

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
ORDER BY f.general, lower(f.title), f.id;

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
WHERE id = @id AND NOT general
RETURNING *;

-- name: DeleteFolderIfEmpty :execrows
DELETE FROM folders
WHERE folders.id = @id AND NOT folders.general AND NOT EXISTS (SELECT 1 FROM dashboards d WHERE d.folder_id = folders.id);

-- name: CountFolderDashboards :one
SELECT count(*)::integer FROM dashboards WHERE folder_id = @id;

-- name: ListFolderPermissions :many
SELECT p.user_id, p.group_name, p.level,
  coalesce(nullif(u.name, ''), u.email, '')::text AS user_name,
  coalesce(u.email, '')::text AS user_email,
  coalesce(u.role, '')::text AS user_role,
  CASE WHEN p.group_name IS NULL THEN 0
    ELSE (SELECT count(*) FROM users m WHERE p.group_name = ANY(m.groups))
  END::integer AS member_count
FROM folder_permissions p
LEFT JOIN users u ON u.id = p.user_id
WHERE p.folder_id = @folder_id
ORDER BY (p.group_name IS NOT NULL), lower(coalesce(nullif(u.name, ''), u.email, p.group_name)), p.created_at;

-- name: DeleteFolderPermissions :exec
DELETE FROM folder_permissions WHERE folder_id = @folder_id;

-- name: InsertFolderPermission :exec
INSERT INTO folder_permissions (folder_id, user_id, group_name, level, created_by_id)
VALUES (@folder_id, sqlc.narg('user_id'), sqlc.narg('group_name'), @level, sqlc.narg('created_by_id'));

-- name: UsersByEmail :many
SELECT id FROM users WHERE lower(email) = lower(@email::text);
