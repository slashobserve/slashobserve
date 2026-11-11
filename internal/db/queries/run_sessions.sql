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
