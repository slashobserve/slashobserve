-- +goose Up
ALTER TABLE folders
  ADD COLUMN admins_only BOOLEAN NOT NULL DEFAULT false,
  ADD CONSTRAINT folders_admins_only_check CHECK (NOT (admins_only AND (general OR personal)));

-- +goose Down
ALTER TABLE folders DROP COLUMN admins_only;
