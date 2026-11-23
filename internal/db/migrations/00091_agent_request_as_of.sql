-- +goose Up
ALTER TABLE agent_requests ADD COLUMN as_of TIMESTAMPTZ;
ALTER TABLE findings ADD COLUMN as_of TIMESTAMPTZ;
-- +goose Down
ALTER TABLE findings DROP COLUMN as_of;
ALTER TABLE agent_requests DROP COLUMN as_of;
