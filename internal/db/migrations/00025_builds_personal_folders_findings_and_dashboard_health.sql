-- +goose Up
ALTER TABLE agent_requests ADD COLUMN failure_code TEXT;

CREATE TABLE agent_request_builds (
  agent_request_id UUID PRIMARY KEY REFERENCES agent_requests (id) ON DELETE CASCADE,
  dashboard_id UUID REFERENCES dashboards (id) ON DELETE CASCADE,
  base_version INTEGER,
  spec JSONB NOT NULL,
  unverified JSONB,
  summary TEXT NOT NULL,
  saves INTEGER NOT NULL DEFAULT 1,
  panels INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  planned_panels INTEGER NOT NULL DEFAULT 0,
  CONSTRAINT agent_request_builds_target_check CHECK ((dashboard_id IS NULL) = (base_version IS NULL)),
  CONSTRAINT agent_request_builds_unverified_check CHECK (unverified IS NULL OR jsonb_typeof(unverified) = 'object'),
  CONSTRAINT agent_request_builds_planned_panels_check CHECK (planned_panels BETWEEN 0 AND 200)
);

ALTER TABLE agent_requests ADD COLUMN build_saved_at TIMESTAMPTZ;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_build_saved_check
  CHECK (build_saved_at IS NULL OR (status = 'failed' AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));

ALTER TABLE agent_requests
  ADD COLUMN cancel_requested_at TIMESTAMPTZ,
  ADD COLUMN canceled_by_kind TEXT CHECK (canceled_by_kind IN ('user', 'service_account')),
  ADD COLUMN canceled_by_id UUID,
  ADD COLUMN canceled_by_name TEXT,
  ADD COLUMN continues_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  ADD COLUMN continue_note TEXT;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_canceled_by_check
  CHECK ((cancel_requested_at IS NULL) = (canceled_by_kind IS NULL)
    AND (canceled_by_kind IS NULL) = (canceled_by_id IS NULL)
    AND (canceled_by_id IS NULL) = (canceled_by_name IS NULL));
CREATE UNIQUE INDEX agent_requests_continues_key ON agent_requests (continues_id) WHERE continues_id IS NOT NULL;

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

CREATE TABLE provisioned_dashboards (
  path TEXT PRIMARY KEY,
  dashboard_id UUID NOT NULL UNIQUE REFERENCES dashboards (id) ON DELETE CASCADE,
  content_sha256 TEXT NOT NULL,
  folder_title TEXT NOT NULL,
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE provision_errors (
  path TEXT PRIMARY KEY,
  message TEXT NOT NULL,
  seen_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE annotations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  panel_id TEXT NOT NULL DEFAULT '',
  time_from TIMESTAMPTZ NOT NULL,
  time_to TIMESTAMPTZ NULL,
  text TEXT NOT NULL CHECK (length(text) <= 2000),
  tags TEXT[] NOT NULL DEFAULT '{}',
  created_by UUID NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX annotations_dashboard_time_idx ON annotations (dashboard_id, time_from);
CREATE INDEX annotations_tags_idx ON annotations USING GIN (tags);

CREATE TABLE jobs (
  name TEXT PRIMARY KEY,
  every_seconds INT NOT NULL,
  next_run_at TIMESTAMPTZ NOT NULL,
  leased_by TEXT NOT NULL DEFAULT '',
  lease_until TIMESTAMPTZ NULL,
  last_started_at TIMESTAMPTZ NULL,
  last_finished_at TIMESTAMPTZ NULL,
  last_outcome TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE grafana_connections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  url TEXT NOT NULL CHECK (length(url) BETWEEN 1 AND 2048),
  token_ciphertext BYTEA NOT NULL CHECK (octet_length(token_ciphertext) BETWEEN 1 AND 8192),
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_test_at TIMESTAMPTZ,
  last_test_ok BOOLEAN NOT NULL DEFAULT false,
  last_test_message TEXT NOT NULL DEFAULT '',
  last_sync JSONB CHECK (last_sync IS NULL OR jsonb_typeof(last_sync) = 'object')
);
CREATE INDEX grafana_connections_created_at_id_idx ON grafana_connections (created_at, id);

ALTER TABLE imports ADD COLUMN grafana_uid TEXT NOT NULL DEFAULT '';
UPDATE imports i
SET grafana_uid = first.uid
FROM (
  SELECT DISTINCT ON (summary ->> 'uid') id, summary ->> 'uid' AS uid
  FROM imports
  WHERE summary ->> 'uid' ~ '^[A-Za-z0-9_-]{1,40}$'
  ORDER BY summary ->> 'uid', created_at, id
) first
WHERE i.id = first.id;
CREATE UNIQUE INDEX imports_grafana_uid_key ON imports (grafana_uid) WHERE grafana_uid <> '';

ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe', 'suggest', 'investigate'));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_target_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_target_check
  CHECK (source IN ('observe', 'investigate') OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL));
ALTER TABLE agent_requests ADD COLUMN context_from TIMESTAMPTZ, ADD COLUMN context_to TIMESTAMPTZ;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_context_check
  CHECK ((context_from IS NULL AND context_to IS NULL)
    OR (source = 'investigate' AND context_from IS NOT NULL AND context_to IS NOT NULL AND context_from < context_to));
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_investigate_shape_check
  CHECK (source <> 'investigate' OR (base_version IS NULL AND comment_id IS NULL AND result_version IS NULL
    AND draft IS NULL AND suggestion IS NULL AND continues_id IS NULL AND (panel_id IS NULL OR dashboard_id IS NOT NULL)));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR source = 'investigate'
    OR (source = 'suggest' AND suggestion IS NOT NULL)
    OR (source = 'observe' AND dashboard_id IS NULL AND result_version IS NULL)
    OR (source IN ('prompt', 'comment') AND dashboard_id IS NOT NULL AND result_version IS NULL)
    OR (source NOT IN ('suggest', 'investigate') AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));
DROP INDEX agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests (dashboard_id)
  WHERE status = 'running' AND source NOT IN ('suggest', 'investigate');
CREATE INDEX agent_requests_investigate_active_idx ON agent_requests (requested_by_id)
  WHERE source = 'investigate' AND status IN ('queued', 'running');
CREATE TABLE findings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_request_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  dashboard_id UUID REFERENCES dashboards (id) ON DELETE CASCADE,
  title TEXT NOT NULL CONSTRAINT findings_title_check CHECK (char_length(title) BETWEEN 1 AND 120),
  markdown TEXT NOT NULL CONSTRAINT findings_markdown_check CHECK (char_length(markdown) BETWEEN 1 AND 4000),
  evidence JSONB NOT NULL DEFAULT '[]'::jsonb
    CONSTRAINT findings_evidence_check CHECK (CASE WHEN jsonb_typeof(evidence) = 'array' THEN jsonb_array_length(evidence) <= 4 ELSE false END),
  time_from TIMESTAMPTZ NOT NULL,
  time_to TIMESTAMPTZ NOT NULL,
  created_by_kind TEXT NOT NULL CONSTRAINT findings_created_by_kind_check CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  annotation_id UUID REFERENCES annotations (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT findings_window_check CHECK (time_from < time_to)
);
CREATE UNIQUE INDEX findings_agent_request_key ON findings (agent_request_id) WHERE agent_request_id IS NOT NULL;
CREATE INDEX findings_dashboard_idx ON findings (dashboard_id, created_at DESC, id DESC) WHERE dashboard_id IS NOT NULL;
CREATE INDEX findings_creator_idx ON findings (created_by_id, created_at DESC, id DESC);

ALTER TABLE agent_requests
  ADD COLUMN title TEXT CONSTRAINT agent_requests_title_check CHECK (title IS NULL OR char_length(title) BETWEEN 1 AND 120),
  ADD COLUMN pinned_at TIMESTAMPTZ,
  ADD COLUMN archived_at TIMESTAMPTZ;

CREATE TABLE public_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  token_hash BYTEA NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  display_prefix TEXT NOT NULL,
  variables TEXT NOT NULL CHECK (variables IN ('frozen', 'free')),
  selections JSONB NOT NULL DEFAULT '{}',
  expires_at TIMESTAMPTZ NULL,
  created_by_kind TEXT NOT NULL,
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_viewed_at TIMESTAMPTZ NULL
);
CREATE INDEX public_links_dashboard_id_idx ON public_links (dashboard_id);

CREATE TABLE dashboard_health (
  dashboard_id UUID PRIMARY KEY REFERENCES dashboards (id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  checked_at TIMESTAMPTZ NOT NULL,
  skipped TEXT NOT NULL DEFAULT '',
  issues JSONB NOT NULL DEFAULT '[]'
);
CREATE TABLE dashboard_views (
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, dashboard_id)
);
CREATE INDEX dashboard_views_dashboard_idx ON dashboard_views (dashboard_id, viewed_at DESC);

CREATE TABLE applied_dashboards (
  path TEXT PRIMARY KEY,
  dashboard_id UUID NOT NULL UNIQUE REFERENCES dashboards (id) ON DELETE CASCADE,
  content_sha256 TEXT NOT NULL,
  applied_by_id UUID NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dashboard_stars (
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  starred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, dashboard_id)
);
CREATE INDEX dashboard_stars_dashboard_idx ON dashboard_stars (dashboard_id);

ALTER TABLE service_accounts ADD COLUMN provisioned BOOLEAN NOT NULL DEFAULT false;

CREATE VIEW dashboard_health_status AS
SELECT d.id, d.title, d.created_at, d.folder_id,
  f.title AS folder_title, f.general AS folder_general, f.personal AS folder_personal,
  h.version AS checked_version, h.checked_at, coalesce(h.skipped, '')::text AS skipped,
  coalesce(h.issues, '[]'::jsonb)::jsonb AS issues,
  (coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"query_failed"}]'::jsonb)::boolean AS query_failed,
  (coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"no_data"}]'::jsonb)::boolean AS no_data,
  (coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"variable_failed"}]'::jsonb)::boolean AS variable_failed,
  (SELECT max(dv.viewed_at) FROM dashboard_views dv WHERE dv.dashboard_id = d.id)::timestamptz AS last_viewed_at,
  EXISTS (
    SELECT 1 FROM dashboards o
    WHERE o.folder_id = d.folder_id AND o.id <> d.id AND lower(o.title) = lower(d.title)
  ) AS duplicate_title
FROM dashboards d
JOIN folders f ON f.id = d.folder_id
LEFT JOIN dashboard_health h ON h.dashboard_id = d.id;

-- +goose Down
DROP VIEW dashboard_health_status;

ALTER TABLE service_accounts DROP COLUMN provisioned;

DROP TABLE dashboard_stars;

DROP TABLE applied_dashboards;

DROP TABLE dashboard_views;
DROP TABLE dashboard_health;

DROP TABLE public_links;

ALTER TABLE agent_requests DROP COLUMN archived_at, DROP COLUMN pinned_at, DROP COLUMN title;

DROP TABLE findings;
DELETE FROM agent_requests WHERE source = 'investigate';
DROP INDEX agent_requests_investigate_active_idx;
DROP INDEX agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests (dashboard_id) WHERE status = 'running' AND source <> 'suggest';
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR (source = 'suggest' AND suggestion IS NOT NULL) OR (source <> 'suggest' AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_investigate_shape_check;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_context_check;
ALTER TABLE agent_requests DROP COLUMN context_to, DROP COLUMN context_from;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_target_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_target_check
  CHECK (source = 'observe' OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL));
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_source_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe', 'suggest'));

DROP INDEX imports_grafana_uid_key;
ALTER TABLE imports DROP COLUMN grafana_uid;

DROP TABLE grafana_connections;

DROP TABLE jobs;

DROP TABLE annotations;

DROP TABLE provision_errors;
DROP TABLE provisioned_dashboards;

DROP TRIGGER users_personal_dashboards ON users;
DROP FUNCTION delete_personal_dashboards();
ALTER TABLE folders
  DROP CONSTRAINT folders_owner_id_fkey,
  DROP CONSTRAINT folders_owner_check,
  ADD CONSTRAINT folders_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE SET NULL,
  ADD CONSTRAINT folders_owner_check CHECK (owner_id IS NULL OR personal);

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

DROP INDEX agent_requests_continues_key;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_canceled_by_check;
ALTER TABLE agent_requests
  DROP COLUMN continue_note,
  DROP COLUMN continues_id,
  DROP COLUMN canceled_by_name,
  DROP COLUMN canceled_by_id,
  DROP COLUMN canceled_by_kind,
  DROP COLUMN cancel_requested_at;

ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_build_saved_check;
ALTER TABLE agent_requests DROP COLUMN build_saved_at;
DROP TABLE agent_request_builds;

ALTER TABLE agent_requests DROP COLUMN failure_code;
