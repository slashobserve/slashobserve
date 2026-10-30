-- +goose Up
ALTER TABLE folder_permissions DROP CONSTRAINT folder_permissions_level_check;
ALTER TABLE folder_permissions ADD CONSTRAINT folder_permissions_level_check CHECK (level IN ('view', 'comment', 'edit', 'admin'));

-- +goose Down
UPDATE folder_permissions SET level = 'view' WHERE level = 'comment';
ALTER TABLE folder_permissions DROP CONSTRAINT folder_permissions_level_check;
ALTER TABLE folder_permissions ADD CONSTRAINT folder_permissions_level_check CHECK (level IN ('view', 'edit', 'admin'));
