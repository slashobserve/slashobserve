-- +goose Up
ALTER TABLE users
  ADD COLUMN role TEXT NOT NULL DEFAULT 'editor' CHECK (role IN ('admin', 'editor', 'commenter', 'viewer')),
  ADD COLUMN role_source TEXT NOT NULL DEFAULT 'default' CHECK (role_source IN ('admin_emails', 'first_user', 'group', 'manual', 'default')),
  ADD COLUMN role_group TEXT,
  ADD COLUMN manual_role TEXT CHECK (manual_role IN ('admin', 'editor', 'commenter', 'viewer')),
  ADD COLUMN manual_role_by_id UUID REFERENCES users (id) ON DELETE SET NULL,
  ADD COLUMN manual_role_by_name TEXT,
  ADD COLUMN groups TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX users_groups_idx ON users USING GIN (groups);
ALTER TABLE service_accounts
  ADD COLUMN role TEXT NOT NULL DEFAULT 'editor' CHECK (role IN ('admin', 'editor', 'commenter', 'viewer'));

-- +goose Down
ALTER TABLE service_accounts DROP COLUMN role;
DROP INDEX users_groups_idx;
ALTER TABLE users
  DROP COLUMN groups, DROP COLUMN manual_role_by_name, DROP COLUMN manual_role_by_id,
  DROP COLUMN manual_role, DROP COLUMN role_group, DROP COLUMN role_source, DROP COLUMN role;
