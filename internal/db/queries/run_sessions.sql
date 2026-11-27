-- name: InsertRunSession :execrows
INSERT INTO run_sessions (agent_request_id, attempt, state)
VALUES (@agent_request_id, @attempt, @state)
ON CONFLICT (agent_request_id, attempt) DO NOTHING;

-- name: GetRunSessionForUpdate :one
SELECT s.state, r.claim_epoch
FROM run_sessions s JOIN agent_requests r ON r.id = s.agent_request_id
WHERE s.agent_request_id = @agent_request_id AND s.attempt = @attempt FOR UPDATE OF s;

-- name: GetRunSessionSettled :one
SELECT COALESCE(s.state->'settled'->>'kind', '')::text AS kind
FROM run_sessions s JOIN agent_requests r ON r.id = s.agent_request_id
WHERE s.agent_request_id = @agent_request_id AND s.attempt = @attempt AND r.claim_epoch = @claim_epoch;

-- name: UpsertRunSession :exec
INSERT INTO run_sessions (agent_request_id, attempt, state)
VALUES (@agent_request_id, @attempt, @state)
ON CONFLICT (agent_request_id, attempt) DO UPDATE SET state = EXCLUDED.state, updated_at = now();

-- name: DeleteRunSession :exec
DELETE FROM run_sessions WHERE agent_request_id = @agent_request_id AND attempt = @attempt;

-- name: DeleteOlderRunSessions :exec
DELETE FROM run_sessions WHERE agent_request_id = @agent_request_id AND attempt < @attempt;

-- name: GetPriorRunSession :one
SELECT state FROM run_sessions WHERE agent_request_id = @agent_request_id AND attempt < @attempt ORDER BY attempt DESC LIMIT 1;

-- name: ShareRunTargets :exec
WITH targets AS (
  SELECT r.dashboard_id, r.observation_id, b.folder_id
  FROM agent_requests r LEFT JOIN agent_request_builds b ON b.agent_request_id = r.id WHERE r.id = @id
  UNION ALL
  SELECT b.dashboard_id, r.subject_observation_id, NULL
  FROM agent_requests r LEFT JOIN agent_request_builds b ON b.agent_request_id = r.id WHERE r.id = @id
  UNION ALL
  SELECT COALESCE(t.dashboard_id, p.dashboard_id, n.dashboard_id), t.observation_id, NULL
  FROM agent_requests r
  JOIN thread_messages m ON m.id = r.thread_message_id
  JOIN threads t ON t.id = m.thread_id
  LEFT JOIN proposals p ON p.id = t.proposal_id
  LEFT JOIN annotations n ON n.id = t.annotation_id
  WHERE r.id = @id
),
locked_dashboards AS (
  SELECT d.folder_id FROM dashboards d WHERE d.id IN (SELECT dashboard_id FROM targets) ORDER BY d.id FOR SHARE
),
locked_observations AS (
  SELECT o.folder_id FROM observations o WHERE o.id IN (SELECT observation_id FROM targets) ORDER BY o.id FOR SHARE
)
SELECT f.id FROM folders f
WHERE f.id IN (SELECT folder_id FROM locked_dashboards UNION SELECT folder_id FROM locked_observations UNION SELECT folder_id FROM targets)
ORDER BY f.id FOR SHARE;

-- name: GetRunWrites :one
SELECT
  r.intent,
  (r.thread_message_id IS NOT NULL)::boolean AS in_thread,
  (r.observation_id IS NOT NULL)::boolean AS into_observation,
  (r.dashboard_id IS NOT NULL)::boolean AS on_dashboard,
  (b.agent_request_id IS NOT NULL)::boolean AS building,
  COALESCE((
    SELECT f.admins_only
    FROM thread_messages m
    JOIN threads t ON t.id = m.thread_id
    LEFT JOIN proposals p ON p.id = t.proposal_id
    LEFT JOIN annotations n ON n.id = t.annotation_id
    LEFT JOIN dashboards d ON d.id = COALESCE(t.dashboard_id, p.dashboard_id, n.dashboard_id)
    LEFT JOIN observations o ON o.id = t.observation_id
    JOIN folders f ON f.id = COALESCE(d.folder_id, o.folder_id)
    WHERE m.id = r.thread_message_id
  ), false)::boolean AS thread_admins_only,
  COALESCE((
    SELECT f.admins_only FROM observations o JOIN folders f ON f.id = o.folder_id WHERE o.id = r.observation_id
  ), false)::boolean AS observation_admins_only,
  COALESCE((
    SELECT f.admins_only FROM dashboards d JOIN folders f ON f.id = d.folder_id WHERE d.id = r.dashboard_id
  ), false)::boolean AS dashboard_admins_only,
  COALESCE((
    SELECT f.admins_only FROM folders f
    WHERE f.id = COALESCE(b.folder_id, (SELECT d.folder_id FROM dashboards d WHERE d.id = b.dashboard_id))
  ), false)::boolean AS build_admins_only,
  (
    SELECT COALESCE(d.folder_id, o.folder_id)
    FROM thread_messages m
    JOIN threads t ON t.id = m.thread_id
    LEFT JOIN proposals p ON p.id = t.proposal_id
    LEFT JOIN annotations n ON n.id = t.annotation_id
    LEFT JOIN dashboards d ON d.id = COALESCE(t.dashboard_id, p.dashboard_id, n.dashboard_id)
    LEFT JOIN observations o ON o.id = t.observation_id
    WHERE m.id = r.thread_message_id
  )::uuid AS thread_folder_id,
  (SELECT o.folder_id FROM observations o WHERE o.id = r.observation_id)::uuid AS observation_folder_id,
  (SELECT d.folder_id FROM dashboards d WHERE d.id = r.dashboard_id)::uuid AS dashboard_folder_id,
  COALESCE(b.folder_id, (SELECT d.folder_id FROM dashboards d WHERE d.id = b.dashboard_id))::uuid AS build_folder_id
FROM agent_requests r
LEFT JOIN agent_request_builds b ON b.agent_request_id = r.id
WHERE r.id = @id;

-- name: MarkRunReadRestricted :exec
UPDATE agent_requests SET read_restricted = true WHERE id = @id AND NOT read_restricted;

-- name: RunInheritsReadRestricted :one
SELECT (EXISTS (SELECT 1 FROM thread_messages asked JOIN thread_messages m ON m.thread_id = asked.thread_id WHERE asked.id = r.thread_message_id AND run_answer_kept_with_admins(m.run_id, ARRAY[r.dashboard_id, r.observation_id, r.subject_observation_id]))
  OR EXISTS (SELECT 1 FROM findings f WHERE f.id = r.applies_finding_id AND run_answer_kept_with_admins(f.agent_request_id, ARRAY[r.dashboard_id, r.observation_id, r.subject_observation_id]))
  OR COALESCE(run_answer_kept_with_admins(r.continues_id, ARRAY[r.dashboard_id, r.observation_id, r.subject_observation_id]), false))::boolean
FROM agent_requests r WHERE r.id = @id;

-- name: TurnsKeptWithAdminsFrom :one
SELECT EXISTS (SELECT 1 FROM unnest(@turn_ids::uuid[]) AS t(id) WHERE run_answer_kept_with_admins(t.id, ARRAY[r.dashboard_id, r.observation_id, r.subject_observation_id]))::boolean
FROM agent_requests r WHERE r.id = @id;
