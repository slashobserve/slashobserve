-- name: RunObservation :one
SELECT observation_id FROM agent_requests WHERE id = @id;

-- name: ObservationHasActiveRun :one
SELECT EXISTS (
  SELECT 1 FROM agent_requests r
  WHERE r.status IN ('queued', 'running')
    AND (r.observation_id = @observation_id OR r.id = (SELECT run_id FROM observations WHERE id = @observation_id))
);

-- name: RunHasTextBlock :one
SELECT EXISTS (
  SELECT 1 FROM observation_blocks
  WHERE observation_id = @observation_id AND run_id = @run_id AND kind = 'text'
);

-- name: RunBlockOfRun :one
SELECT * FROM observation_blocks
WHERE observation_id = @observation_id AND run_id = @run_id AND kind = 'run'
ORDER BY position
LIMIT 1;

-- name: ChatObservations :many
SELECT chat.id AS chat_id, chat.requested_by_kind, chat.requested_by_id, o.id AS observation_id
FROM agent_requests chat
JOIN agent_requests turn ON turn.id = chat.id OR turn.chat_id = chat.id
JOIN observations o ON o.run_id = turn.id
WHERE chat.chat_id IS NULL AND chat.dashboard_id IS NULL AND chat.archived_at IS NULL
  AND chat.id = coalesce(sqlc.narg('chat_id')::uuid, (
    SELECT coalesce(r.chat_id, r.id) FROM observations so JOIN agent_requests r ON r.id = so.run_id
    WHERE so.id = sqlc.narg('observation_id')::uuid))
LIMIT 2;
