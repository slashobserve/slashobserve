-- +goose Up
ALTER TABLE context_sources ADD COLUMN allow_private BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE context_sources DROP COLUMN allow_private;
