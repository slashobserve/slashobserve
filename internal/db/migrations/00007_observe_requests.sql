-- +goose Up
ALTER TABLE agent_requests ALTER COLUMN dashboard_id DROP NOT NULL;
ALTER TABLE agent_requests ALTER COLUMN base_version DROP NOT NULL;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe'));
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_target_check
  CHECK (source = 'observe' OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL));
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_observe_shape_check
  CHECK (source <> 'observe' OR (base_version IS NULL AND panel_id IS NULL AND comment_id IS NULL));
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR (dashboard_id IS NOT NULL AND result_version IS NOT NULL));
ALTER TABLE agent_requests ADD COLUMN steps JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_steps_check CHECK (jsonb_typeof(steps) = 'array');
ALTER TABLE agent_requests ADD COLUMN run_usage JSONB;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_run_usage_check CHECK (run_usage IS NULL OR jsonb_typeof(run_usage) = 'object');
CREATE INDEX agent_requests_observe_active_idx ON agent_requests (requested_by_id)
  WHERE source = 'observe' AND status IN ('queued', 'running');

-- +goose Down
DELETE FROM agent_requests WHERE source = 'observe';
DROP INDEX agent_requests_observe_active_idx;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_run_usage_check;
ALTER TABLE agent_requests DROP COLUMN run_usage;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_steps_check;
ALTER TABLE agent_requests DROP COLUMN steps;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_observe_shape_check;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_target_check;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt'));
ALTER TABLE agent_requests ALTER COLUMN base_version SET NOT NULL;
ALTER TABLE agent_requests ALTER COLUMN dashboard_id SET NOT NULL;
