-- +goose Up
ALTER TABLE agent_requests
  ADD COLUMN assigned_kind TEXT NOT NULL DEFAULT 'builtin'
    CONSTRAINT agent_requests_assigned_kind_check CHECK (assigned_kind IN ('builtin', 'grant', 'service_account')),
  ADD COLUMN assigned_id UUID,
  ADD COLUMN claimed_by TEXT CONSTRAINT agent_requests_claimed_by_check CHECK (length(claimed_by) BETWEEN 1 AND 300),
  ADD CONSTRAINT agent_requests_assigned_check CHECK ((assigned_kind = 'builtin') = (assigned_id IS NULL)),
  ADD CONSTRAINT agent_requests_assigned_source_check CHECK (assigned_kind = 'builtin' OR source <> 'suggest');

CREATE INDEX agent_requests_assigned_idx ON agent_requests (assigned_kind, assigned_id, created_at)
  WHERE status IN ('queued', 'running') AND assigned_kind <> 'builtin';
CREATE INDEX agent_requests_claimed_by_idx ON agent_requests (claimed_by) WHERE status = 'running';

CREATE TABLE run_tokens (
  token_hash BYTEA PRIMARY KEY CONSTRAINT run_tokens_token_hash_check CHECK (length(token_hash) = 32),
  agent_request_id UUID NOT NULL REFERENCES agent_requests (id) ON DELETE CASCADE,
  attempt INTEGER NOT NULL CONSTRAINT run_tokens_attempt_check CHECK (attempt >= 1),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX run_tokens_request_idx ON run_tokens (agent_request_id);
CREATE INDEX run_tokens_expires_idx ON run_tokens (expires_at);

CREATE TABLE agent_workers (
  id TEXT PRIMARY KEY CONSTRAINT agent_workers_id_check CHECK (length(id) BETWEEN 1 AND 300),
  principal_kind TEXT NOT NULL CONSTRAINT agent_workers_principal_kind_check CHECK (principal_kind IN ('builtin', 'grant', 'service_account')),
  principal_id UUID,
  client TEXT NOT NULL DEFAULT '' CONSTRAINT agent_workers_client_check CHECK (length(client) <= 200),
  seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT agent_workers_principal_check CHECK ((principal_kind = 'builtin') = (principal_id IS NULL))
);

ALTER TABLE oauth_grants
  ADD COLUMN mode TEXT NOT NULL DEFAULT 'direct' CONSTRAINT oauth_grants_mode_check CHECK (mode IN ('direct', 'propose'));

ALTER TABLE service_accounts ADD COLUMN takes_runs BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE service_accounts DROP COLUMN takes_runs;
ALTER TABLE oauth_grants DROP COLUMN mode;
DROP TABLE agent_workers;
DROP TABLE run_tokens;
DROP INDEX agent_requests_claimed_by_idx;
DROP INDEX agent_requests_assigned_idx;
ALTER TABLE agent_requests
  DROP CONSTRAINT agent_requests_assigned_source_check,
  DROP CONSTRAINT agent_requests_assigned_check,
  DROP COLUMN claimed_by,
  DROP COLUMN assigned_id,
  DROP COLUMN assigned_kind;
