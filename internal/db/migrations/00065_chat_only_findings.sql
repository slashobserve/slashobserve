-- +goose Up
ALTER TABLE findings ALTER COLUMN observation_id DROP NOT NULL;
ALTER TABLE findings ADD CONSTRAINT findings_unsaved_run_check CHECK (observation_id IS NOT NULL OR agent_request_id IS NOT NULL);

-- +goose Down
ALTER TABLE findings DROP CONSTRAINT findings_unsaved_run_check;
DELETE FROM findings WHERE observation_id IS NULL;
ALTER TABLE findings ALTER COLUMN observation_id SET NOT NULL;
