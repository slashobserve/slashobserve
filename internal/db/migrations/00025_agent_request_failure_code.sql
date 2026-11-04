-- +goose Up
ALTER TABLE agent_requests ADD COLUMN failure_code TEXT;

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN failure_code;
