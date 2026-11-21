-- +goose Up
ALTER TABLE folders
  ADD COLUMN admins_only BOOLEAN NOT NULL DEFAULT false,
  ADD CONSTRAINT folders_admins_only_check CHECK (NOT (admins_only AND (general OR personal)));

ALTER TABLE snapshots ADD COLUMN datasource_uids TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE snapshots DROP COLUMN datasource_uids;

ALTER TABLE folders DROP COLUMN admins_only;
