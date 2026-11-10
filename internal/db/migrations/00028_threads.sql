-- +goose Up
CREATE TABLE threads (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID,
  dashboard_version INTEGER,
  panel_id TEXT CHECK (length(panel_id) BETWEEN 1 AND 64),
  finding_id UUID REFERENCES findings (id) ON DELETE CASCADE,
  annotation_id UUID REFERENCES annotations (id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
  resolved_at TIMESTAMPTZ,
  resolved_by_kind TEXT CHECK (resolved_by_kind IN ('user', 'service_account')),
  resolved_by_id UUID,
  resolved_by_name TEXT,
  resolved_origin TEXT CONSTRAINT threads_resolved_origin_check CHECK (resolved_origin IN ('web', 'cli', 'mcp', 'api')),
  resolved_origin_client TEXT CONSTRAINT threads_resolved_origin_client_check
    CHECK (resolved_origin_client IS NULL OR (resolved_origin IS NOT NULL AND resolved_origin = 'mcp' AND length(resolved_origin_client) BETWEEN 1 AND 64)),
  resolved_run_id UUID,
  resolved_by_version INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT threads_target_check CHECK (num_nonnulls(dashboard_id, finding_id, annotation_id) = 1),
  CONSTRAINT threads_anchor_check CHECK ((dashboard_id IS NULL) = (dashboard_version IS NULL) AND (panel_id IS NULL OR dashboard_id IS NOT NULL)),
  CONSTRAINT threads_resolved_check CHECK (
    (status = 'resolved') = (resolved_at IS NOT NULL)
    AND (status = 'resolved' OR (resolved_by_kind IS NULL AND resolved_by_id IS NULL AND resolved_by_name IS NULL
      AND resolved_origin IS NULL AND resolved_run_id IS NULL))),
  CONSTRAINT threads_resolved_by_version_check CHECK (
    resolved_by_version IS NULL OR (status = 'resolved' AND dashboard_id IS NOT NULL AND resolved_by_version >= dashboard_version)),
  CONSTRAINT threads_dashboard_version_fkey FOREIGN KEY (dashboard_id, dashboard_version)
    REFERENCES dashboard_versions (dashboard_id, version) ON DELETE CASCADE,
  CONSTRAINT threads_resolved_by_version_fkey FOREIGN KEY (dashboard_id, resolved_by_version)
    REFERENCES dashboard_versions (dashboard_id, version) ON DELETE SET NULL (resolved_by_version)
);
CREATE INDEX threads_dashboard_idx ON threads (dashboard_id) WHERE dashboard_id IS NOT NULL;
CREATE INDEX threads_dashboard_open_idx ON threads (dashboard_id) WHERE dashboard_id IS NOT NULL AND status = 'open';
CREATE INDEX threads_finding_idx ON threads (finding_id) WHERE finding_id IS NOT NULL;
CREATE INDEX threads_annotation_idx ON threads (annotation_id) WHERE annotation_id IS NOT NULL;

CREATE TABLE thread_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  thread_id UUID NOT NULL REFERENCES threads (id) ON DELETE CASCADE,
  body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
  author_kind TEXT NOT NULL CHECK (author_kind IN ('user', 'service_account')),
  author_id UUID NOT NULL,
  author_name TEXT NOT NULL,
  author_email TEXT,
  origin TEXT CONSTRAINT thread_messages_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT thread_messages_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  edited_at TIMESTAMPTZ
);
CREATE INDEX thread_messages_thread_created_idx ON thread_messages (thread_id, created_at, id);

CREATE TABLE thread_mentions (
  message_id UUID NOT NULL REFERENCES thread_messages (id) ON DELETE CASCADE,
  user_id UUID REFERENCES users (id) ON DELETE CASCADE,
  agent TEXT CHECK (agent IN ('observe')),
  name TEXT NOT NULL CHECK (length(name) <= 256),
  CONSTRAINT thread_mentions_one_check CHECK (num_nonnulls(user_id, agent) = 1)
);
CREATE UNIQUE INDEX thread_mentions_user_key ON thread_mentions (message_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX thread_mentions_agent_key ON thread_mentions (message_id, agent) WHERE agent IS NOT NULL;
CREATE INDEX thread_mentions_user_idx ON thread_mentions (user_id) WHERE user_id IS NOT NULL;

INSERT INTO threads (
  id, dashboard_id, dashboard_version, panel_id, status, resolved_at, resolved_by_kind, resolved_by_id, resolved_by_name,
  resolved_by_version, created_at, updated_at
)
SELECT
  c.id, c.dashboard_id, c.version, c.panel_id, c.status, c.resolved_at,
  CASE WHEN c.status = 'resolved' THEN c.resolved_by_kind END,
  CASE WHEN c.status = 'resolved' THEN c.resolved_by_id END,
  CASE WHEN c.status = 'resolved' THEN c.resolved_by_name END,
  c.resolved_by_version, c.created_at, c.updated_at
FROM dashboard_comments c
WHERE c.parent_id IS NULL;

INSERT INTO thread_messages (id, thread_id, body, author_kind, author_id, author_name, author_email, created_at, updated_at)
SELECT c.id, coalesce(c.parent_id, c.id), c.body, c.author_kind, c.author_id, c.author_name, c.author_email, c.created_at, c.created_at
FROM dashboard_comments c;

ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread'));

-- +goose Down
DELETE FROM activity WHERE target_kind = 'thread';
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run'));
DROP TABLE thread_mentions, thread_messages, threads;
