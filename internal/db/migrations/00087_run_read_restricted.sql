-- +goose Up
ALTER TABLE agent_requests ADD COLUMN read_restricted BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN read_restricted;
