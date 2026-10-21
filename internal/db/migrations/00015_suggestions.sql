-- +goose Up
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe', 'suggest'));
ALTER TABLE agent_requests ADD COLUMN draft JSONB
  CONSTRAINT agent_requests_draft_check CHECK (draft IS NULL OR (source = 'suggest' AND jsonb_typeof(draft) = 'object'));
ALTER TABLE agent_requests ADD COLUMN suggestion JSONB
  CONSTRAINT agent_requests_suggestion_check CHECK (suggestion IS NULL OR (source = 'suggest' AND jsonb_typeof(suggestion) = 'object'));
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_suggest_shape_check
  CHECK (source <> 'suggest' OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL AND panel_id IS NOT NULL AND comment_id IS NULL AND result_version IS NULL));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR (source = 'suggest' AND suggestion IS NOT NULL) OR (source <> 'suggest' AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));
DROP INDEX agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests (dashboard_id) WHERE status = 'running' AND source <> 'suggest';
CREATE INDEX agent_requests_suggest_active_idx ON agent_requests (requested_by_id) WHERE source = 'suggest' AND status IN ('queued', 'running');

-- +goose Down
DELETE FROM agent_requests WHERE source = 'suggest';
DROP INDEX agent_requests_suggest_active_idx;
DROP INDEX agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests (dashboard_id) WHERE status = 'running';
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check CHECK (status <> 'succeeded' OR (dashboard_id IS NOT NULL AND result_version IS NOT NULL));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_suggest_shape_check;
ALTER TABLE agent_requests DROP COLUMN suggestion;
ALTER TABLE agent_requests DROP COLUMN draft;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe'));
