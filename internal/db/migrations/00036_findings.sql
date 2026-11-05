-- +goose Up
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe', 'suggest', 'investigate'));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_target_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_target_check
  CHECK (source IN ('observe', 'investigate') OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL));
ALTER TABLE agent_requests ADD COLUMN context_from TIMESTAMPTZ, ADD COLUMN context_to TIMESTAMPTZ;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_context_check
  CHECK ((context_from IS NULL AND context_to IS NULL)
    OR (source = 'investigate' AND context_from IS NOT NULL AND context_to IS NOT NULL AND context_from < context_to));
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_investigate_shape_check
  CHECK (source <> 'investigate' OR (base_version IS NULL AND comment_id IS NULL AND result_version IS NULL
    AND draft IS NULL AND suggestion IS NULL AND continues_id IS NULL AND (panel_id IS NULL OR dashboard_id IS NOT NULL)));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR source = 'investigate'
    OR (source = 'suggest' AND suggestion IS NOT NULL)
    OR (source NOT IN ('suggest', 'investigate') AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));
DROP INDEX agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests (dashboard_id)
  WHERE status = 'running' AND source NOT IN ('suggest', 'investigate');
CREATE INDEX agent_requests_investigate_active_idx ON agent_requests (requested_by_id)
  WHERE source = 'investigate' AND status IN ('queued', 'running');
CREATE TABLE findings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_request_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  dashboard_id UUID REFERENCES dashboards (id) ON DELETE CASCADE,
  title TEXT NOT NULL CONSTRAINT findings_title_check CHECK (char_length(title) BETWEEN 1 AND 120),
  markdown TEXT NOT NULL CONSTRAINT findings_markdown_check CHECK (char_length(markdown) BETWEEN 1 AND 4000),
  evidence JSONB NOT NULL DEFAULT '[]'::jsonb
    CONSTRAINT findings_evidence_check CHECK (CASE WHEN jsonb_typeof(evidence) = 'array' THEN jsonb_array_length(evidence) <= 4 ELSE false END),
  time_from TIMESTAMPTZ NOT NULL,
  time_to TIMESTAMPTZ NOT NULL,
  created_by_kind TEXT NOT NULL CONSTRAINT findings_created_by_kind_check CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  annotation_id UUID REFERENCES annotations (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT findings_window_check CHECK (time_from < time_to)
);
CREATE UNIQUE INDEX findings_agent_request_key ON findings (agent_request_id) WHERE agent_request_id IS NOT NULL;
CREATE INDEX findings_dashboard_idx ON findings (dashboard_id, created_at DESC, id DESC) WHERE dashboard_id IS NOT NULL;
CREATE INDEX findings_creator_idx ON findings (created_by_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE findings;
DELETE FROM agent_requests WHERE source = 'investigate';
DROP INDEX agent_requests_investigate_active_idx;
DROP INDEX agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests (dashboard_id) WHERE status = 'running' AND source <> 'suggest';
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR (source = 'suggest' AND suggestion IS NOT NULL) OR (source <> 'suggest' AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_investigate_shape_check;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_context_check;
ALTER TABLE agent_requests DROP COLUMN context_to, DROP COLUMN context_from;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_target_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_target_check
  CHECK (source = 'observe' OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe', 'suggest'));
