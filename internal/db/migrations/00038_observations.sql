-- +goose Up
CREATE TABLE observations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  folder_id UUID NOT NULL REFERENCES folders (id) ON DELETE RESTRICT,
  title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'resolved')),
  summary TEXT CHECK (summary IS NULL OR char_length(summary) BETWEEN 1 AND 500),
  time_from TIMESTAMPTZ,
  time_to TIMESTAMPTZ,
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  origin TEXT CONSTRAINT observations_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT observations_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  run_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT observations_resolved_check CHECK (state = 'open' OR summary IS NOT NULL),
  CONSTRAINT observations_resolved_at_check CHECK ((state = 'resolved') = (resolved_at IS NOT NULL)),
  CONSTRAINT observations_time_check CHECK ((time_from IS NULL) = (time_to IS NULL) AND (time_from IS NULL OR time_from < time_to))
);
CREATE INDEX observations_folder_idx ON observations (folder_id, updated_at DESC, id DESC);
CREATE INDEX observations_updated_idx ON observations (updated_at DESC, id DESC);
CREATE INDEX observations_run_idx ON observations (run_id) WHERE run_id IS NOT NULL;

CREATE TABLE snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  observation_id UUID NOT NULL REFERENCES observations (id) ON DELETE CASCADE,
  panel JSONB NOT NULL CHECK (jsonb_typeof(panel) = 'object'),
  query JSONB NOT NULL CHECK (jsonb_typeof(query) = 'array'),
  frames JSONB NOT NULL CHECK (jsonb_typeof(frames) = 'object'),
  user_scoped BOOLEAN NOT NULL DEFAULT false,
  time_from TIMESTAMPTZ NOT NULL,
  time_to TIMESTAMPTZ NOT NULL,
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  origin TEXT CONSTRAINT snapshots_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT snapshots_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  run_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT snapshots_size_check CHECK (pg_column_size(frames) <= 2097152),
  CONSTRAINT snapshots_time_check CHECK (time_from < time_to)
);
CREATE INDEX snapshots_observation_idx ON snapshots (observation_id);

CREATE TABLE observation_blocks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  observation_id UUID NOT NULL REFERENCES observations (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('text', 'panel', 'snapshot', 'run')),
  state TEXT NOT NULL DEFAULT 'live' CHECK (state IN ('live', 'requested')),
  position INTEGER NOT NULL CHECK (position >= 0),
  width TEXT NOT NULL DEFAULT 'full' CHECK (width IN ('full', 'half')),
  version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
  body JSONB NOT NULL CHECK (jsonb_typeof(body) = 'object'),
  snapshot_id UUID REFERENCES snapshots (id),
  run_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  origin TEXT CONSTRAINT observation_blocks_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT observation_blocks_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT observation_blocks_snapshot_check CHECK ((kind = 'snapshot') = (snapshot_id IS NOT NULL)),
  CONSTRAINT observation_blocks_position_key UNIQUE (observation_id, position) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX observation_blocks_run_idx ON observation_blocks (run_id) WHERE run_id IS NOT NULL;
CREATE INDEX observation_blocks_snapshot_idx ON observation_blocks (snapshot_id) WHERE snapshot_id IS NOT NULL;

CREATE TABLE observation_rcas (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  observation_id UUID NOT NULL REFERENCES observations (id) ON DELETE CASCADE,
  revision INTEGER NOT NULL CHECK (revision >= 1),
  title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  summary TEXT CHECK (summary IS NULL OR char_length(summary) BETWEEN 1 AND 500),
  observation_state TEXT NOT NULL CHECK (observation_state IN ('open', 'resolved')),
  time_from TIMESTAMPTZ,
  time_to TIMESTAMPTZ,
  content JSONB NOT NULL CHECK (jsonb_typeof(content) = 'object'),
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  origin TEXT CONSTRAINT observation_rcas_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT observation_rcas_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  run_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT observation_rcas_revision_key UNIQUE (observation_id, revision)
);

-- +goose StatementBegin
CREATE FUNCTION observation_rcas_immutable() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.run_id IS NOT NULL OR (to_jsonb(NEW) - 'run_id') <> (to_jsonb(OLD) - 'run_id') THEN
    RAISE EXCEPTION 'an RCA revision cannot be changed' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER observation_rcas_immutable BEFORE UPDATE ON observation_rcas
FOR EACH ROW EXECUTE FUNCTION observation_rcas_immutable();

-- +goose StatementBegin
CREATE FUNCTION snapshots_immutable() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.run_id IS NOT NULL OR (to_jsonb(NEW) - 'run_id') <> (to_jsonb(OLD) - 'run_id') THEN
    RAISE EXCEPTION 'a snapshot cannot be changed' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER snapshots_immutable BEFORE UPDATE ON snapshots
FOR EACH ROW EXECUTE FUNCTION snapshots_immutable();

ALTER TABLE threads ADD COLUMN observation_id UUID REFERENCES observations (id) ON DELETE CASCADE;
ALTER TABLE threads DROP CONSTRAINT threads_target_check;
ALTER TABLE threads ADD CONSTRAINT threads_target_check
  CHECK (num_nonnulls(dashboard_id, finding_id, annotation_id, observation_id) = 1);
CREATE UNIQUE INDEX threads_observation_idx ON threads (observation_id) WHERE observation_id IS NOT NULL;

ALTER TABLE activity ADD COLUMN observation_id UUID;
CREATE INDEX activity_observation_idx ON activity (observation_id) WHERE observation_id IS NOT NULL;
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'observation'));

ALTER TABLE agent_requests ADD COLUMN observation_id UUID REFERENCES observations (id) ON DELETE SET NULL;
CREATE INDEX agent_requests_observation_idx ON agent_requests (observation_id) WHERE observation_id IS NOT NULL;

ALTER TABLE public_links ADD COLUMN rca_id UUID REFERENCES observation_rcas (id) ON DELETE CASCADE;
ALTER TABLE public_links ALTER COLUMN dashboard_id DROP NOT NULL;
ALTER TABLE public_links ADD CONSTRAINT public_links_subject_check CHECK (num_nonnulls(dashboard_id, rca_id) = 1);
CREATE INDEX public_links_rca_idx ON public_links (rca_id) WHERE rca_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION observation_search_vector(title text, summary text) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE COST 1000 SET search_path FROM CURRENT
AS $$
SELECT search_vector(title, '', coalesce(summary, ''), '')
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_block_search_vector(body jsonb) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE COST 1000 SET search_path FROM CURRENT
AS $$
SELECT search_vector('', '', coalesce(body ->> 'markdown', ''), '')
$$;
-- +goose StatementEnd

CREATE INDEX observations_search_idx ON observations USING GIN (observation_search_vector(title, summary));
CREATE INDEX observation_blocks_search_idx ON observation_blocks USING GIN (observation_block_search_vector(body))
  WHERE kind = 'text' AND state = 'live';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_personal_folder_contents() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM saved_searches s USING folders f WHERE s.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM findings x USING folders f WHERE x.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM observations o USING folders f WHERE o.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM dashboards d USING folders f WHERE d.folder_id = f.id AND f.owner_id = OLD.id;
  RETURN OLD;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_personal_folder_contents() RETURNS trigger
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

DROP INDEX observation_blocks_search_idx;
DROP INDEX observations_search_idx;
DROP FUNCTION observation_block_search_vector(jsonb);
DROP FUNCTION observation_search_vector(text, text);

DELETE FROM public_links WHERE rca_id IS NOT NULL;
ALTER TABLE public_links DROP CONSTRAINT public_links_subject_check;
DROP INDEX public_links_rca_idx;
ALTER TABLE public_links DROP COLUMN rca_id;
ALTER TABLE public_links ALTER COLUMN dashboard_id SET NOT NULL;

DROP INDEX agent_requests_observation_idx;
ALTER TABLE agent_requests DROP COLUMN observation_id;

DELETE FROM activity WHERE target_kind = 'observation' OR observation_id IS NOT NULL;
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search'));
DROP INDEX activity_observation_idx;
ALTER TABLE activity DROP COLUMN observation_id;

DELETE FROM threads WHERE observation_id IS NOT NULL;
DROP INDEX threads_observation_idx;
ALTER TABLE threads DROP CONSTRAINT threads_target_check;
ALTER TABLE threads ADD CONSTRAINT threads_target_check
  CHECK (num_nonnulls(dashboard_id, finding_id, annotation_id) = 1);
ALTER TABLE threads DROP COLUMN observation_id;

DROP TRIGGER snapshots_immutable ON snapshots;
DROP FUNCTION snapshots_immutable();
DROP TRIGGER observation_rcas_immutable ON observation_rcas;
DROP FUNCTION observation_rcas_immutable();
DROP TABLE observation_rcas, observation_blocks, snapshots, observations;
