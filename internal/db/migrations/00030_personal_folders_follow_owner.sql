-- +goose Up
DELETE FROM dashboards d USING folders f
WHERE d.folder_id = f.id AND f.personal AND f.owner_id IS NULL;
DELETE FROM folders WHERE personal AND owner_id IS NULL;
ALTER TABLE folders
  DROP CONSTRAINT folders_owner_check,
  DROP CONSTRAINT folders_owner_id_fkey,
  ADD CONSTRAINT folders_owner_check CHECK ((owner_id IS NOT NULL) = personal),
  ADD CONSTRAINT folders_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE;
-- +goose StatementBegin
CREATE FUNCTION delete_personal_dashboards() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM dashboards d USING folders f WHERE d.folder_id = f.id AND f.owner_id = OLD.id;
  RETURN OLD;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER users_personal_dashboards BEFORE DELETE ON users
FOR EACH ROW EXECUTE FUNCTION delete_personal_dashboards();

-- +goose Down
DROP TRIGGER users_personal_dashboards ON users;
DROP FUNCTION delete_personal_dashboards();
ALTER TABLE folders
  DROP CONSTRAINT folders_owner_id_fkey,
  DROP CONSTRAINT folders_owner_check,
  ADD CONSTRAINT folders_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE SET NULL,
  ADD CONSTRAINT folders_owner_check CHECK (owner_id IS NULL OR personal);
