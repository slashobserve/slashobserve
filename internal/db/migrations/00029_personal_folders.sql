-- +goose Up
ALTER TABLE folders
  ADD COLUMN personal BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN owner_id UUID REFERENCES users (id) ON DELETE SET NULL,
  ADD CONSTRAINT folders_kind_check CHECK (NOT (personal AND general)),
  ADD CONSTRAINT folders_owner_check CHECK (owner_id IS NULL OR personal);
CREATE UNIQUE INDEX folders_owner_idx ON folders (owner_id) WHERE owner_id IS NOT NULL;
DROP INDEX folders_title_idx;
CREATE UNIQUE INDEX folders_title_idx ON folders (lower(title)) WHERE NOT personal;
-- +goose StatementBegin
CREATE FUNCTION create_personal_folder() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  INSERT INTO folders (title, personal, owner_id, created_by_kind, created_by_id, created_by_name)
  VALUES ('My Dashboards', true, NEW.id, 'user', NEW.id, coalesce(nullif(NEW.name, ''), NEW.email))
  ON CONFLICT (owner_id) WHERE owner_id IS NOT NULL DO NOTHING;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER users_personal_folder AFTER INSERT ON users
FOR EACH ROW EXECUTE FUNCTION create_personal_folder();
INSERT INTO folders (title, personal, owner_id, created_by_kind, created_by_id, created_by_name)
SELECT 'My Dashboards', true, u.id, 'user', u.id, coalesce(nullif(u.name, ''), u.email) FROM users u;

-- +goose Down
DROP TRIGGER users_personal_folder ON users;
DROP FUNCTION create_personal_folder();
DELETE FROM folders f WHERE f.personal AND NOT EXISTS (SELECT 1 FROM dashboards d WHERE d.folder_id = f.id);
INSERT INTO folder_permissions (folder_id, user_id, level)
SELECT id, owner_id, 'admin' FROM folders WHERE personal AND owner_id IS NOT NULL;
INSERT INTO folder_permissions (folder_id, group_name, level)
SELECT id, 'removed-owner-' || id::text, 'view' FROM folders WHERE personal AND owner_id IS NULL;
UPDATE folders SET title = 'Personal ' || id::text WHERE personal;
DROP INDEX folders_title_idx;
CREATE UNIQUE INDEX folders_title_idx ON folders (lower(title));
DROP INDEX folders_owner_idx;
ALTER TABLE folders
  DROP CONSTRAINT folders_owner_check,
  DROP CONSTRAINT folders_kind_check,
  DROP COLUMN owner_id,
  DROP COLUMN personal;
