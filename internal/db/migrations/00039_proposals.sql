-- +goose Up
CREATE TABLE proposals (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  state TEXT NOT NULL DEFAULT 'open' CONSTRAINT proposals_state_check CHECK (state IN ('open', 'merged', 'discarded')),
  title TEXT NOT NULL CONSTRAINT proposals_title_check CHECK (char_length(title) BETWEEN 1 AND 200),
  revision INTEGER NOT NULL DEFAULT 1 CONSTRAINT proposals_revision_check CHECK (revision >= 1),
  author_kind TEXT NOT NULL CONSTRAINT proposals_author_kind_check CHECK (author_kind IN ('user', 'service_account')),
  author_id UUID NOT NULL,
  author_name TEXT NOT NULL,
  origin TEXT CONSTRAINT proposals_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT proposals_origin_client_check
    CHECK (origin_client IS NULL OR (origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  run_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  closed_at TIMESTAMPTZ,
  closed_by_kind TEXT CONSTRAINT proposals_closed_by_kind_check CHECK (closed_by_kind IN ('user', 'service_account')),
  closed_by_id UUID,
  closed_by_name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT proposals_closed_check CHECK (
    (state = 'open') = (closed_at IS NULL)
    AND (state = 'open') = (closed_by_kind IS NULL)
    AND (closed_by_kind IS NULL) = (closed_by_id IS NULL)
    AND (closed_by_kind IS NULL) = (closed_by_name IS NULL))
);
CREATE INDEX proposals_dashboard_idx ON proposals (dashboard_id, created_at);
CREATE INDEX proposals_run_idx ON proposals (run_id) WHERE run_id IS NOT NULL;
CREATE UNIQUE INDEX proposals_open_author_key ON proposals (dashboard_id, author_kind, author_id)
  WHERE state = 'open' AND run_id IS NULL;
CREATE UNIQUE INDEX proposals_open_run_key ON proposals (dashboard_id, run_id)
  WHERE state = 'open' AND run_id IS NOT NULL;

CREATE TABLE proposal_revisions (
  proposal_id UUID NOT NULL REFERENCES proposals (id) ON DELETE CASCADE,
  revision INTEGER NOT NULL CONSTRAINT proposal_revisions_revision_check CHECK (revision >= 1),
  base_version INTEGER NOT NULL CONSTRAINT proposal_revisions_base_version_check CHECK (base_version >= 1),
  spec JSONB NOT NULL CONSTRAINT proposal_revisions_spec_check CHECK (jsonb_typeof(spec) = 'object'),
  summary TEXT NOT NULL CONSTRAINT proposal_revisions_summary_check CHECK (char_length(summary) BETWEEN 1 AND 500),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (proposal_id, revision)
);

ALTER TABLE dashboard_versions ADD COLUMN merged_proposal_id UUID REFERENCES proposals (id) ON DELETE SET NULL;
CREATE UNIQUE INDEX dashboard_versions_merged_proposal_key ON dashboard_versions (merged_proposal_id)
  WHERE merged_proposal_id IS NOT NULL;

ALTER TABLE threads ADD COLUMN proposal_id UUID REFERENCES proposals (id) ON DELETE CASCADE;
ALTER TABLE threads DROP CONSTRAINT threads_target_check;
ALTER TABLE threads ADD CONSTRAINT threads_target_check
  CHECK (num_nonnulls(dashboard_id, finding_id, annotation_id, proposal_id) = 1);
CREATE INDEX threads_proposal_idx ON threads (proposal_id) WHERE proposal_id IS NOT NULL;

ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'proposal'));

ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR source = 'investigate'
    OR (source = 'suggest' AND suggestion IS NOT NULL)
    OR (source = 'observe' AND dashboard_id IS NULL AND result_version IS NULL)
    OR (source IN ('prompt', 'comment') AND dashboard_id IS NOT NULL AND result_version IS NULL)
    OR (source NOT IN ('suggest', 'investigate') AND dashboard_id IS NOT NULL AND result_version IS NOT NULL)
    OR (source NOT IN ('suggest', 'investigate') AND dashboard_id IS NOT NULL AND result_version IS NULL AND result_kind = 'proposal'));

-- +goose Down
UPDATE agent_requests SET status = 'failed', state = 'failed', failure_code = 'internal', error = 'proposal results were removed by a downgrade'
  WHERE status = 'succeeded' AND result_kind = 'proposal' AND source = 'observe' AND dashboard_id IS NOT NULL AND result_version IS NULL;
UPDATE agent_requests SET result_kind = 'none' WHERE result_kind = 'proposal';
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_succeeded_check;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_succeeded_check
  CHECK (status <> 'succeeded' OR source = 'investigate'
    OR (source = 'suggest' AND suggestion IS NOT NULL)
    OR (source = 'observe' AND dashboard_id IS NULL AND result_version IS NULL)
    OR (source IN ('prompt', 'comment') AND dashboard_id IS NOT NULL AND result_version IS NULL)
    OR (source NOT IN ('suggest', 'investigate') AND dashboard_id IS NOT NULL AND result_version IS NOT NULL));
DELETE FROM activity WHERE target_kind = 'proposal';
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search'));
DELETE FROM threads WHERE proposal_id IS NOT NULL;
ALTER TABLE threads DROP CONSTRAINT threads_target_check;
ALTER TABLE threads DROP COLUMN proposal_id;
ALTER TABLE threads ADD CONSTRAINT threads_target_check CHECK (num_nonnulls(dashboard_id, finding_id, annotation_id) = 1);
ALTER TABLE dashboard_versions DROP COLUMN merged_proposal_id;
DROP TABLE proposal_revisions;
DROP TABLE proposals;
