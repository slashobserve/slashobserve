-- +goose Up
CREATE TABLE agent_request_builds (
  agent_request_id UUID PRIMARY KEY REFERENCES agent_requests (id) ON DELETE CASCADE,
  dashboard_id UUID REFERENCES dashboards (id) ON DELETE CASCADE,
  base_version INTEGER,
  spec JSONB NOT NULL,
  unverified JSONB,
  summary TEXT NOT NULL,
  saves INTEGER NOT NULL DEFAULT 1,
  panels INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  planned_panels INTEGER NOT NULL DEFAULT 0,
  CONSTRAINT agent_request_builds_target_check CHECK ((dashboard_id IS NULL) = (base_version IS NULL)),
  CONSTRAINT agent_request_builds_unverified_check CHECK (unverified IS NULL OR jsonb_typeof(unverified) = 'object'),
  CONSTRAINT agent_request_builds_planned_panels_check CHECK (planned_panels BETWEEN 0 AND 200)
);

ALTER TABLE agent_requests ADD COLUMN build_saved_at TIMESTAMPTZ;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_build_saved_check
  CHECK (build_saved_at IS NULL OR (status = 'failed' AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));

-- +goose Down
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_build_saved_check;
ALTER TABLE agent_requests DROP COLUMN build_saved_at;
DROP TABLE agent_request_builds;
