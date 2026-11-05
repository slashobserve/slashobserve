-- +goose Up
ALTER TABLE agent_requests
  ADD COLUMN title TEXT CONSTRAINT agent_requests_title_check CHECK (title IS NULL OR char_length(title) BETWEEN 1 AND 120),
  ADD COLUMN pinned_at TIMESTAMPTZ,
  ADD COLUMN archived_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN archived_at, DROP COLUMN pinned_at, DROP COLUMN title;
