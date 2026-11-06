-- +goose Up
ALTER TABLE service_accounts ADD COLUMN provisioned BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE service_accounts DROP COLUMN provisioned;
