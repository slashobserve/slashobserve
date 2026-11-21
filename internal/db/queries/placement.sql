-- name: ShareDatasourceUIDs :exec
SELECT pg_advisory_xact_lock_shared(hashtextextended('datasource_uid:' || u.uid, 0)) FROM unnest(@uids::text[]) AS u(uid);

-- name: LockDatasourceUID :exec
SELECT pg_advisory_xact_lock(hashtextextended('datasource_uid:' || @uid::text, 0));

-- name: ShareDatasourceAudiences :many
SELECT uid, audience FROM datasources WHERE uid = ANY(@uids::text[]) ORDER BY uid FOR SHARE;

-- name: AdminsOnlyDatasourceUIDs :many
SELECT uid FROM datasources WHERE audience = 'admins' ORDER BY uid;

-- name: ShareFolderPlacement :one
SELECT id, title, admins_only FROM folders WHERE id = @id FOR SHARE;

-- name: RunReadRestricted :one
SELECT read_restricted FROM agent_requests WHERE id = @id;

-- name: FindingRun :one
SELECT agent_request_id FROM findings WHERE id = @id;

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

-- name: PlacementObservations :many
SELECT o.id, o.title, o.folder_id, f.title AS folder_title, f.personal AS folder_personal
FROM observations o
JOIN folders f ON f.id = o.folder_id
WHERE (sqlc.narg('folder_id')::uuid IS NULL OR o.folder_id = sqlc.narg('folder_id')::uuid)
  AND (NOT @shared_only::boolean OR NOT f.admins_only)
  AND o.id > @after_id::uuid
  AND (EXISTS (SELECT 1 FROM observation_blocks b, unnest(@uids::text[]) AS u(uid)
      WHERE b.observation_id = o.id AND b.kind = 'panel' AND position('"' || u.uid || '"' IN b.body::text) > 0)
    OR EXISTS (SELECT 1 FROM snapshots s WHERE s.observation_id = o.id
      AND (s.datasource_uids && @uids::text[]
        OR EXISTS (SELECT 1 FROM unnest(@uids::text[]) AS u(uid)
          WHERE position('"' || u.uid || '"' IN s.panel::text) > 0 OR position('"' || u.uid || '"' IN s.query::text) > 0))))
ORDER BY o.id
LIMIT @page_size;

-- name: PlacementSavedSearches :many
SELECT s.id, s.title, s.folder_id, f.title AS folder_title, f.personal AS folder_personal, s.datasource_uid
FROM saved_searches s
JOIN folders f ON f.id = s.folder_id
WHERE s.datasource_uid = ANY(@uids::text[])
  AND (sqlc.narg('folder_id')::uuid IS NULL OR s.folder_id = sqlc.narg('folder_id')::uuid)
  AND (NOT @shared_only::boolean OR NOT f.admins_only)
  AND s.id > @after_id::uuid
ORDER BY s.id
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

-- name: DashboardHoldsRestrictedAnswer :one
SELECT (EXISTS (SELECT 1 FROM dashboard_versions v JOIN agent_requests r ON r.id = v.agent_request_id WHERE v.dashboard_id = @id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.dashboard_id = @id AND f.annotation_id IS NOT NULL AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.dashboard_id = @id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM proposals p JOIN threads t ON t.proposal_id = p.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE p.dashboard_id = @id AND r.read_restricted))::boolean;

-- name: ObservationHoldsRestrictedAnswer :one
SELECT (EXISTS (SELECT 1 FROM observation_blocks b JOIN agent_requests r ON r.id = b.run_id WHERE b.observation_id = @id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.observation_id = @id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.observation_id = @id AND r.read_restricted))::boolean;

-- name: RestrictedAnswerHolders :many
SELECT 'dashboard'::text AS kind, d.id, d.title FROM dashboards d
WHERE d.folder_id = @folder_id AND (EXISTS (SELECT 1 FROM dashboard_versions v JOIN agent_requests r ON r.id = v.agent_request_id WHERE v.dashboard_id = d.id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.dashboard_id = d.id AND f.annotation_id IS NOT NULL AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.dashboard_id = d.id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM proposals p JOIN threads t ON t.proposal_id = p.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE p.dashboard_id = d.id AND r.read_restricted))
UNION ALL
SELECT 'observation'::text AS kind, o.id, o.title FROM observations o
WHERE o.folder_id = @folder_id AND (EXISTS (SELECT 1 FROM observation_blocks b JOIN agent_requests r ON r.id = b.run_id WHERE b.observation_id = o.id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.observation_id = o.id AND r.read_restricted)
  OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.observation_id = o.id AND r.read_restricted))
ORDER BY kind, title, id
LIMIT @row_limit;
