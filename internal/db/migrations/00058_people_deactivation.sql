-- +goose Up
ALTER TABLE users
  ADD COLUMN deactivated_at TIMESTAMPTZ,
  ADD COLUMN deactivated_by_id UUID REFERENCES users (id) ON DELETE SET NULL,
  ADD COLUMN deactivated_by_name TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN deactivated_by_name, DROP COLUMN deactivated_by_id, DROP COLUMN deactivated_at;
