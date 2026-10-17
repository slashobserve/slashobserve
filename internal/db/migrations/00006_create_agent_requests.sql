-- +goose Up
CREATE TABLE agent_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('comment', 'prompt')),
  comment_id UUID REFERENCES dashboard_comments (id) ON DELETE SET NULL,
  prompt TEXT NOT NULL CHECK (length(prompt) BETWEEN 1 AND 4000),
  panel_id TEXT CHECK (panel_id IS NULL OR length(panel_id) BETWEEN 1 AND 64),
  base_version INTEGER NOT NULL CHECK (base_version >= 1),
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  lease_until TIMESTAMPTZ,
  result_version INTEGER,
  error TEXT CHECK (error IS NULL OR length(error) <= 500),
  requested_by_kind TEXT NOT NULL CHECK (requested_by_kind IN ('user', 'service_account')),
  requested_by_id UUID NOT NULL,
  requested_by_name TEXT NOT NULL,
  requested_by_email TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  CHECK (source = 'comment' OR comment_id IS NULL)
);
CREATE INDEX agent_requests_claim_idx ON agent_requests (created_at) WHERE status IN ('queued', 'running');
CREATE INDEX agent_requests_dashboard_idx ON agent_requests (dashboard_id, created_at DESC, id DESC);
CREATE INDEX agent_requests_comment_idx ON agent_requests (comment_id, created_at DESC) WHERE comment_id IS NOT NULL;
ALTER TABLE dashboard_versions ADD COLUMN agent_request_id UUID UNIQUE REFERENCES agent_requests (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE dashboard_versions DROP COLUMN agent_request_id;
DROP TABLE agent_requests;
