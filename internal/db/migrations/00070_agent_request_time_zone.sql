-- +goose Up
ALTER TABLE agent_requests
  ADD COLUMN time_zone TEXT,
  ADD CONSTRAINT agent_requests_time_zone_check CHECK (time_zone IS NULL OR char_length(time_zone) BETWEEN 1 AND 64);

-- +goose Down
ALTER TABLE agent_requests
  DROP CONSTRAINT agent_requests_time_zone_check,
  DROP COLUMN time_zone;
