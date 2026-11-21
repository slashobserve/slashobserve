-- name: InsertRunSession :execrows
INSERT INTO run_sessions (agent_request_id, attempt, state)
VALUES (@agent_request_id, @attempt, @state)
ON CONFLICT (agent_request_id, attempt) DO NOTHING;

-- name: GetRunSessionForUpdate :one
SELECT state FROM run_sessions WHERE agent_request_id = @agent_request_id AND attempt = @attempt FOR UPDATE;

-- name: UpsertRunSession :exec
INSERT INTO run_sessions (agent_request_id, attempt, state)
VALUES (@agent_request_id, @attempt, @state)
ON CONFLICT (agent_request_id, attempt) DO UPDATE SET state = EXCLUDED.state, updated_at = now();

-- name: DeleteRunSession :exec
DELETE FROM run_sessions WHERE agent_request_id = @agent_request_id AND attempt = @attempt;

-- name: DeleteOlderRunSessions :exec
DELETE FROM run_sessions WHERE agent_request_id = @agent_request_id AND attempt < @attempt;

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
    LEFT JOIN dashboards d ON d.id = COALESCE(t.dashboard_id, p.dashboard_id)
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
  ), false)::boolean AS build_admins_only
FROM agent_requests r
LEFT JOIN agent_request_builds b ON b.agent_request_id = r.id
WHERE r.id = @id;

-- name: MarkRunReadRestricted :exec
UPDATE agent_requests SET read_restricted = true WHERE id = @id AND NOT read_restricted;

-- name: RunInheritsReadRestricted :one
SELECT (EXISTS (SELECT 1 FROM thread_messages asked JOIN thread_messages m ON m.thread_id = asked.thread_id JOIN agent_requests a ON a.id = m.run_id WHERE asked.id = r.thread_message_id AND a.read_restricted)
  OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests a ON a.id = f.agent_request_id WHERE f.id = r.applies_finding_id AND a.read_restricted)
  OR EXISTS (SELECT 1 FROM agent_requests a WHERE a.id = r.continues_id AND a.read_restricted))::boolean
FROM agent_requests r WHERE r.id = @id;
