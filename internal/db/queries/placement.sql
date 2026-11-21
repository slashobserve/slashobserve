-- name: ShareDatasourceAudiences :many
SELECT uid, audience FROM datasources WHERE uid = ANY(@uids::text[]) ORDER BY uid FOR SHARE;

-- name: AdminsOnlyDatasourceUIDs :many
SELECT uid FROM datasources WHERE audience = 'admins' ORDER BY uid;

-- name: ShareFolderPlacement :one
SELECT id, title, admins_only FROM folders WHERE id = @id FOR SHARE;

-- name: PlacementDashboardVersions :many
SELECT v.dashboard_id AS id, v.version AS seq, d.title, d.folder_id, f.title AS folder_title, f.personal AS folder_personal, v.spec AS payload
FROM dashboard_versions v
JOIN dashboards d ON d.id = v.dashboard_id
JOIN folders f ON f.id = d.folder_id
WHERE (sqlc.narg('folder_id')::uuid IS NULL OR d.folder_id = sqlc.narg('folder_id')::uuid)
  AND (NOT @shared_only::boolean OR NOT f.admins_only)
  AND (v.dashboard_id, v.version) > (@after_id::uuid, @after_seq::int)
  AND EXISTS (SELECT 1 FROM unnest(@uids::text[]) AS u(uid) WHERE position('"' || u.uid || '"' IN v.spec::text) > 0)
ORDER BY v.dashboard_id, v.version
LIMIT @page_size;

-- name: PlacementProposalRevisions :many
SELECT r.proposal_id AS id, r.revision AS seq, p.title, d.folder_id, f.title AS folder_title, f.personal AS folder_personal, r.spec AS payload
FROM proposal_revisions r
JOIN proposals p ON p.id = r.proposal_id
JOIN dashboards d ON d.id = p.dashboard_id
JOIN folders f ON f.id = d.folder_id
WHERE (sqlc.narg('folder_id')::uuid IS NULL OR d.folder_id = sqlc.narg('folder_id')::uuid)
  AND (NOT @shared_only::boolean OR NOT f.admins_only)
  AND (r.proposal_id, r.revision) > (@after_id::uuid, @after_seq::int)
  AND EXISTS (SELECT 1 FROM unnest(@uids::text[]) AS u(uid) WHERE position('"' || u.uid || '"' IN r.spec::text) > 0)
ORDER BY r.proposal_id, r.revision
LIMIT @page_size;

-- name: PlacementPanelBlocks :many
SELECT o.id, b.id AS block_id, o.title, o.folder_id, f.title AS folder_title, f.personal AS folder_personal, b.body AS payload
FROM observation_blocks b
JOIN observations o ON o.id = b.observation_id
JOIN folders f ON f.id = o.folder_id
WHERE b.kind = 'panel'
  AND (sqlc.narg('folder_id')::uuid IS NULL OR o.folder_id = sqlc.narg('folder_id')::uuid)
  AND (NOT @shared_only::boolean OR NOT f.admins_only)
  AND b.id > @after_id::uuid
  AND EXISTS (SELECT 1 FROM unnest(@uids::text[]) AS u(uid) WHERE position('"' || u.uid || '"' IN b.body::text) > 0)
ORDER BY b.id
LIMIT @page_size;

-- name: PlacementSnapshots :many
SELECT o.id, s.id AS snapshot_id, o.title, o.folder_id, f.title AS folder_title, f.personal AS folder_personal, s.panel, s.query, s.datasource_uids
FROM snapshots s
JOIN observations o ON o.id = s.observation_id
JOIN folders f ON f.id = o.folder_id
WHERE (sqlc.narg('folder_id')::uuid IS NULL OR o.folder_id = sqlc.narg('folder_id')::uuid)
  AND (NOT @shared_only::boolean OR NOT f.admins_only)
  AND s.id > @after_id::uuid
  AND (s.datasource_uids && @uids::text[]
    OR EXISTS (SELECT 1 FROM unnest(@uids::text[]) AS u(uid)
      WHERE position('"' || u.uid || '"' IN s.panel::text) > 0 OR position('"' || u.uid || '"' IN s.query::text) > 0))
ORDER BY s.id
LIMIT @page_size;

-- name: PlacementSavedSearches :many
SELECT s.id, s.title, s.folder_id, f.title AS folder_title, f.personal AS folder_personal, s.datasource_uid
FROM saved_searches s
JOIN folders f ON f.id = s.folder_id
WHERE s.datasource_uid = ANY(@uids::text[])
  AND (sqlc.narg('folder_id')::uuid IS NULL OR s.folder_id = sqlc.narg('folder_id')::uuid)
  AND (NOT @shared_only::boolean OR NOT f.admins_only)
ORDER BY s.title, s.id
LIMIT @page_size;

-- name: PlacementDashboardSpecs :many
SELECT v.spec FROM dashboard_versions v WHERE v.dashboard_id = @id
UNION ALL
SELECT r.spec FROM proposal_revisions r JOIN proposals p ON p.id = r.proposal_id WHERE p.dashboard_id = @id;

-- name: PlacementObservationPanels :many
SELECT body FROM observation_blocks WHERE observation_id = @id AND kind = 'panel';

-- name: PlacementObservationSnapshots :many
SELECT panel, query, datasource_uids FROM snapshots WHERE observation_id = @id;

-- name: LockDashboardPlacement :one
SELECT folder_id FROM dashboards WHERE id = @id FOR UPDATE;

-- name: ShareDashboardPlacement :one
SELECT folder_id FROM dashboards WHERE id = @id FOR SHARE;
