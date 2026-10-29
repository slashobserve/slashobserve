-- +goose Up
CREATE TABLE folders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 100 AND title = btrim(title)),
  general BOOLEAN NOT NULL DEFAULT false,
  created_by_kind TEXT CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID,
  created_by_name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX folders_title_idx ON folders (lower(title));
CREATE UNIQUE INDEX folders_general_idx ON folders (general) WHERE general;

CREATE TABLE folder_permissions (
  folder_id UUID NOT NULL REFERENCES folders (id) ON DELETE CASCADE,
  user_id UUID REFERENCES users (id) ON DELETE CASCADE,
  group_name TEXT CHECK (octet_length(group_name) BETWEEN 1 AND 256),
  level TEXT NOT NULL CHECK (level IN ('view', 'edit', 'admin')),
  created_by_id UUID REFERENCES users (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((user_id IS NULL) <> (group_name IS NULL))
);
CREATE UNIQUE INDEX folder_permissions_user_idx ON folder_permissions (folder_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX folder_permissions_group_idx ON folder_permissions (folder_id, group_name) WHERE group_name IS NOT NULL;

INSERT INTO folders (title, general) VALUES ('General', true);
-- +goose StatementBegin
CREATE FUNCTION general_folder_id() RETURNS uuid
LANGUAGE sql STABLE
AS $$ SELECT id FROM folders WHERE general $$;
-- +goose StatementEnd
ALTER TABLE dashboards ADD COLUMN folder_id UUID REFERENCES folders (id) ON DELETE RESTRICT;
UPDATE dashboards SET folder_id = general_folder_id();
ALTER TABLE dashboards ALTER COLUMN folder_id SET NOT NULL, ALTER COLUMN folder_id SET DEFAULT general_folder_id();
CREATE INDEX dashboards_folder_idx ON dashboards (folder_id);

-- +goose Down
DROP INDEX dashboards_folder_idx;
ALTER TABLE dashboards DROP COLUMN folder_id;
DROP FUNCTION general_folder_id();
DROP TABLE folder_permissions;
DROP TABLE folders;
