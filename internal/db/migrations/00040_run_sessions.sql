-- +goose Up
CREATE TABLE run_sessions (
  agent_request_id UUID NOT NULL REFERENCES agent_requests (id) ON DELETE CASCADE,
  attempt INTEGER NOT NULL CONSTRAINT run_sessions_attempt_check CHECK (attempt >= 1),
  state JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (agent_request_id, attempt)
);

-- +goose Down
DROP TABLE run_sessions;
