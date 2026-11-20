-- +goose Up
ALTER TABLE datasources ADD COLUMN audience TEXT NOT NULL DEFAULT 'everyone' CHECK (audience IN ('everyone', 'admins'));

-- +goose Down
ALTER TABLE datasources DROP COLUMN audience;
