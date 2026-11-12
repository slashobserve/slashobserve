-- name: InsertRunToken :exec
INSERT INTO run_tokens (token_hash, agent_request_id, attempt, expires_at) VALUES (@token_hash, @agent_request_id, @attempt, @expires_at);

-- name: GetRunTokenRun :one
SELECT sqlc.embed(r), t.expires_at AS token_expires_at
FROM run_tokens t
JOIN agent_requests r ON r.id = t.agent_request_id
WHERE t.token_hash = @token_hash
  AND t.attempt = r.attempts
  AND r.state = 'running'
  AND r.lease_until > clock_timestamp()
  AND t.expires_at > clock_timestamp();

-- name: GetRunTokenDeadline :one
SELECT max(expires_at)::timestamptz FROM run_tokens WHERE agent_request_id = @agent_request_id AND attempt = @attempt;

-- name: DeleteRunTokens :exec
DELETE FROM run_tokens WHERE agent_request_id = @agent_request_id;

-- name: PruneRunTokens :execrows
DELETE FROM run_tokens WHERE expires_at < clock_timestamp();

-- name: GetClaimedAgentRequest :one
SELECT * FROM agent_requests WHERE id = @id AND attempts = @attempts AND claimed_by = @claimed_by;

-- name: TouchAgentWorker :exec
INSERT INTO agent_workers (id, principal_kind, principal_id, client, seen_at)
VALUES (@id, @principal_kind, sqlc.narg('principal_id'), @client, clock_timestamp())
ON CONFLICT (id) DO UPDATE SET client = EXCLUDED.client, seen_at = EXCLUDED.seen_at;

-- name: PruneAgentWorkers :execrows
DELETE FROM agent_workers w
WHERE (w.principal_kind = 'grant' AND NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.id = w.principal_id AND g.revoked_at IS NULL))
   OR (w.principal_kind = 'service_account' AND NOT EXISTS (SELECT 1 FROM service_accounts s WHERE s.id = w.principal_id));

-- name: ListHeldAgentRequests :many
SELECT * FROM agent_requests WHERE claimed_by = @claimed_by AND status = 'running';
