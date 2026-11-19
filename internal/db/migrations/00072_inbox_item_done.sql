-- +goose Up
ALTER TABLE inbox_item_states ADD COLUMN done BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE inbox_item_states DROP COLUMN done;
