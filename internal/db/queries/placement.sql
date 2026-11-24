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

-- name: RunAnswerKeptWithAdmins :one
SELECT run_answer_kept_with_admins(r.id, '{}')::boolean FROM agent_requests r WHERE r.id = @id;

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
SELECT dashboard_holds_restricted_answer(@id::uuid)::boolean;

-- name: ObservationHoldsRestrictedAnswer :one
SELECT observation_holds_restricted_answer(@id::uuid)::boolean;

-- name: DashboardKeptWithAdmins :one
SELECT (dashboard_holds_restricted_answer(d.id) OR dashboard_holds_restricted_run(d.id))::boolean AS restricted,
  (f.admins_only AND dashboard_has_run_in_flight(d.id))::boolean AS in_flight
FROM dashboards d JOIN folders f ON f.id = d.folder_id WHERE d.id = @id;

-- name: ObservationKeptWithAdmins :one
SELECT (observation_holds_restricted_answer(o.id) OR observation_holds_restricted_run(o.id))::boolean AS restricted,
  (f.admins_only AND observation_has_run_in_flight(o.id))::boolean AS in_flight
FROM observations o JOIN folders f ON f.id = o.folder_id WHERE o.id = @id;

-- name: RestrictedAnswerHolders :many
SELECT kind, id, title, restricted FROM (
  SELECT 'dashboard'::text AS kind, d.id, d.title, (dashboard_holds_restricted_answer(d.id) OR dashboard_holds_restricted_run(d.id))::boolean AS restricted, dashboard_has_run_in_flight(d.id) AS in_flight
  FROM dashboards d WHERE d.folder_id = @folder_id
  UNION ALL
  SELECT 'observation'::text, o.id, o.title, (observation_holds_restricted_answer(o.id) OR observation_holds_restricted_run(o.id))::boolean, observation_has_run_in_flight(o.id)
  FROM observations o WHERE o.folder_id = @folder_id
) held
WHERE restricted OR in_flight
ORDER BY restricted DESC, kind, title, id
LIMIT @row_limit;

-- name: RestrictedAnswerFolder :one
SELECT COALESCE(
  (SELECT f.id FROM agent_requests r
    CROSS JOIN LATERAL (VALUES
      (1, (SELECT d.folder_id FROM dashboards d WHERE d.id = r.dashboard_id)),
      (2, (SELECT o.folder_id FROM observations o WHERE o.id = r.observation_id)),
      (3, (SELECT o.folder_id FROM observations o WHERE o.id = r.subject_observation_id))) AS subject(rank, folder_id)
    JOIN folders f ON f.id = subject.folder_id
    WHERE r.id = @run_id AND f.admins_only
    ORDER BY subject.rank LIMIT 1),
  (SELECT f.id FROM starter_content s JOIN folders f ON f.id = s.ref WHERE s.key = @audit_folder_key AND f.admins_only)
)::uuid AS folder_id;
