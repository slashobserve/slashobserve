-- +goose Up
ALTER TABLE findings ADD COLUMN unverified JSONB;

-- +goose Down
ALTER TABLE findings DROP COLUMN unverified;
