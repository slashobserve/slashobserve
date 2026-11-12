-- name: RunObservation :one
SELECT observation_id FROM agent_requests WHERE id = @id;

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
