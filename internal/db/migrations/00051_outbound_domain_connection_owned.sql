-- +goose Up
ALTER TABLE outbound_domains ADD COLUMN for_connection BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE outbound_domains DROP COLUMN for_connection;
