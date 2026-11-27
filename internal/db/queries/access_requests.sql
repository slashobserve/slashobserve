-- name: LockAccessRequestDashboard :one
SELECT d.id, d.folder_id, d.title FROM dashboards d WHERE d.id = @id FOR KEY SHARE;

-- name: LockAccessRequestObservation :one
SELECT o.id, o.folder_id, o.title FROM observations o WHERE o.id = @id FOR KEY SHARE;

-- name: LockAccessRequestSlot :exec
SELECT pg_advisory_xact_lock(hashtextextended('access_request:' || CAST(@object_id::uuid AS text) || ':' || @requester_kind::text || ':' || CAST(@requester_id::uuid AS text), 0));

-- name: OpenAccessRequestFor :one
SELECT r.id, r.created_at, r.note
FROM access_requests r
WHERE coalesce(r.dashboard_id, r.observation_id) = @object_id::uuid
  AND r.requester_kind = @requester_kind::text AND r.requester_id = @requester_id::uuid
  AND access_request_open(r)
ORDER BY r.created_at DESC
LIMIT 1;

-- name: AccessGranterCandidates :many
SELECT u.id, u.role, f.admins_only,
  EXISTS (SELECT 1 FROM folder_permissions p WHERE p.folder_id = f.id)::boolean AS listed,
  ARRAY(
    SELECT p.level FROM folder_permissions p
    WHERE p.folder_id = f.id AND (p.user_id = u.id OR p.group_name = ANY(u.groups))
  )::text[] AS grants,
  coalesce(f.created_by_kind = 'user' AND f.created_by_id = u.id, false)::boolean AS creator
FROM folders f
JOIN users u ON u.deactivated_at IS NULL AND (
  (f.created_by_kind = 'user' AND f.created_by_id = u.id)
  OR EXISTS (
    SELECT 1 FROM folder_permissions p
    WHERE p.folder_id = f.id AND p.level = 'admin' AND (p.user_id = u.id OR p.group_name = ANY(u.groups))
  )
)
WHERE f.id = @folder_id AND NOT f.personal
ORDER BY u.id;

-- name: ActiveAdminIDs :many
SELECT u.id FROM users u WHERE u.role = 'admin' AND u.deactivated_at IS NULL ORDER BY u.id;

-- name: InsertAccessRequest :one
INSERT INTO access_requests (id, dashboard_id, observation_id, requester_kind, requester_id, requester_name, note, granters, activity_seq)
VALUES (@id, sqlc.narg('dashboard_id')::uuid, sqlc.narg('observation_id')::uuid, @requester_kind::text, @requester_id::uuid, @requester_name::text, @note::text, @granters::uuid[], @activity_seq::bigint)
RETURNING created_at;

-- name: ListOpenAccessRequests :many
SELECT r.id, r.dashboard_id, r.observation_id, coalesce(d.title, o.title)::text AS title,
  coalesce(d.folder_id, o.folder_id)::uuid AS folder_id,
  r.requester_kind, r.requester_id, r.requester_name, r.note, r.created_at
FROM access_requests r
LEFT JOIN dashboards d ON d.id = r.dashboard_id
LEFT JOIN observations o ON o.id = r.observation_id
WHERE access_request_open(r) AND coalesce(d.folder_id, o.folder_id) = ANY(@folders::uuid[])
ORDER BY r.created_at DESC, r.id DESC
LIMIT @row_limit;

-- name: AccessRequestsToSettle :many
SELECT r.id, r.dashboard_id, r.observation_id, coalesce(d.title, o.title)::text AS title,
  r.requester_kind, r.requester_id, r.requester_name,
  coalesce(u.role, sa.role, '')::text AS role,
  f.personal, f.admins_only,
  coalesce(f.owner_id = u.id, false)::boolean AS owner,
  EXISTS (SELECT 1 FROM folder_permissions p WHERE p.folder_id = f.id)::boolean AS listed,
  ARRAY(
    SELECT p.level FROM folder_permissions p
    WHERE p.folder_id = f.id AND (p.user_id = u.id OR p.group_name = ANY(coalesce(u.groups, '{}'::text[])))
  )::text[] AS grants,
  coalesce(f.created_by_kind = r.requester_kind AND f.created_by_id = r.requester_id, false)::boolean AS creator
FROM access_requests r
LEFT JOIN dashboards d ON d.id = r.dashboard_id
LEFT JOIN observations o ON o.id = r.observation_id
JOIN folders f ON f.id = coalesce(d.folder_id, o.folder_id)
LEFT JOIN users u ON r.requester_kind = 'user' AND u.id = r.requester_id AND u.deactivated_at IS NULL
LEFT JOIN service_accounts sa ON r.requester_kind = 'service_account' AND sa.id = r.requester_id
WHERE access_request_open(r)
  AND (sqlc.narg('folder_id')::uuid IS NULL OR f.id = sqlc.narg('folder_id')::uuid)
  AND (sqlc.narg('requester_id')::uuid IS NULL OR (r.requester_kind = @requester_kind::text AND r.requester_id = sqlc.narg('requester_id')::uuid))
ORDER BY r.created_at, r.id;

-- name: GrantAccessRequest :execrows
UPDATE access_requests
SET granted_at = now(), granted_by_kind = @granted_by_kind::text, granted_by_id = sqlc.narg('granted_by_id')::uuid
WHERE id = @id AND granted_at IS NULL;

-- name: ListOwnAccessRequests :many
SELECT r.id, r.dashboard_id, r.observation_id, coalesce(d.title, o.title)::text AS title,
  coalesce(d.folder_id, o.folder_id)::uuid AS folder_id,
  r.requester_kind, r.requester_id, r.requester_name, r.note, r.created_at,
  (r.granted_at IS NOT NULL)::boolean AS granted,
  EXISTS (
    SELECT 1 FROM inbox_item_states s
    WHERE s.seq = r.activity_seq AND s.done AND s.user_id = ANY(r.granters)
  )::boolean AS dismissed,
  EXISTS (
    SELECT 1 FROM access_requests n
    WHERE coalesce(n.dashboard_id, n.observation_id) = coalesce(r.dashboard_id, r.observation_id)
      AND n.requester_kind = r.requester_kind AND n.requester_id = r.requester_id
      AND (n.created_at, n.id) > (r.created_at, r.id)
  )::boolean AS superseded,
  now()::timestamptz AS read_at
FROM access_requests r
LEFT JOIN dashboards d ON d.id = r.dashboard_id
LEFT JOIN observations o ON o.id = r.observation_id
WHERE r.requester_kind = @requester_kind::text AND r.requester_id = @requester_id::uuid
  AND coalesce(d.folder_id, o.folder_id) = ANY(@visible::uuid[])
ORDER BY r.created_at DESC, r.id DESC
LIMIT @row_limit;
