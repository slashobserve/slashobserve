-- +goose Up
ALTER TABLE agent_request_builds
  ADD COLUMN folder_id UUID REFERENCES folders (id) ON DELETE SET NULL,
  ADD CONSTRAINT agent_request_builds_folder_check CHECK (folder_id IS NULL OR dashboard_id IS NULL);

-- +goose Down
ALTER TABLE agent_request_builds
  DROP CONSTRAINT agent_request_builds_folder_check,
  DROP COLUMN folder_id;
