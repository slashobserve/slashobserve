-- +goose Up
ALTER TABLE dashboard_versions ADD COLUMN unverified JSONB
  CONSTRAINT dashboard_versions_unverified_check CHECK (unverified IS NULL OR jsonb_typeof(unverified) = 'object');

-- +goose Down
ALTER TABLE dashboard_versions DROP COLUMN unverified;
