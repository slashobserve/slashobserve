-- +goose Up
ALTER TABLE findings ADD COLUMN folder_id UUID REFERENCES folders (id) ON DELETE RESTRICT;
UPDATE findings f SET folder_id = p.id
FROM folders p
WHERE f.dashboard_id IS NULL AND f.created_by_kind = 'user' AND p.personal AND p.owner_id = f.created_by_id;
ALTER TABLE findings ADD CONSTRAINT findings_placement_check CHECK (dashboard_id IS NULL OR folder_id IS NULL);
CREATE INDEX findings_folder_idx ON findings (folder_id, created_at DESC, id DESC) WHERE folder_id IS NOT NULL;

CREATE TABLE saved_searches (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind TEXT NOT NULL CHECK (kind IN ('logs', 'traces')),
  title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
  datasource_uid TEXT NOT NULL CHECK (datasource_uid ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$'),
  query TEXT CHECK (char_length(query) BETWEEN 1 AND 8000),
  filters JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(filters) = 'object'),
  time_from TEXT NOT NULL CHECK (char_length(time_from) BETWEEN 1 AND 64),
  time_to TEXT NOT NULL CHECK (char_length(time_to) BETWEEN 1 AND 64),
  folder_id UUID NOT NULL REFERENCES folders (id) ON DELETE RESTRICT,
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  origin TEXT CONSTRAINT saved_searches_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT saved_searches_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT saved_searches_logs_query_check CHECK (kind <> 'logs' OR query IS NOT NULL)
);
CREATE INDEX saved_searches_folder_idx ON saved_searches (folder_id, updated_at DESC, id DESC);
CREATE INDEX saved_searches_updated_idx ON saved_searches (updated_at DESC, id DESC);

DROP TRIGGER users_personal_dashboards ON users;
DROP FUNCTION delete_personal_dashboards();
-- +goose StatementBegin
CREATE FUNCTION delete_personal_folder_contents() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM saved_searches s USING folders f WHERE s.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM findings x USING folders f WHERE x.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM dashboards d USING folders f WHERE d.folder_id = f.id AND f.owner_id = OLD.id;
  RETURN OLD;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER users_personal_folder_contents BEFORE DELETE ON users
FOR EACH ROW EXECUTE FUNCTION delete_personal_folder_contents();

ALTER TABLE activity ADD COLUMN finding_id UUID, ADD COLUMN saved_search_id UUID;
UPDATE activity a SET finding_id = f.id, folder_id = f.folder_id
FROM findings f
WHERE a.target_kind = 'finding' AND a.target_id = f.id AND a.dashboard_id IS NULL AND f.dashboard_id IS NULL;
UPDATE activity a SET finding_id = f.id, folder_id = f.folder_id
FROM threads t JOIN findings f ON f.id = t.finding_id
WHERE a.target_kind = 'thread' AND a.target_id = t.id AND a.dashboard_id IS NULL AND f.dashboard_id IS NULL;
CREATE INDEX activity_finding_idx ON activity (finding_id) WHERE finding_id IS NOT NULL;
CREATE INDEX activity_saved_search_idx ON activity (saved_search_id) WHERE saved_search_id IS NOT NULL;
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search'));

-- +goose Down
DELETE FROM activity WHERE target_kind = 'saved_search' OR type = 'finding.moved';
UPDATE activity SET folder_id = NULL WHERE finding_id IS NOT NULL;
ALTER TABLE activity DROP COLUMN finding_id, DROP COLUMN saved_search_id;
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread'));

DROP TRIGGER users_personal_folder_contents ON users;
DROP FUNCTION delete_personal_folder_contents();
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

DROP TABLE saved_searches;
ALTER TABLE findings DROP COLUMN folder_id;
