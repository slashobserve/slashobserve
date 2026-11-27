-- +goose Up
ALTER TABLE findings ADD COLUMN causes JSONB;

-- +goose Down
ALTER TABLE findings DROP COLUMN causes;
