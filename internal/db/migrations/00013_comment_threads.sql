-- +goose Up
ALTER TABLE dashboard_comments ALTER COLUMN panel_id DROP NOT NULL;
ALTER TABLE dashboard_comments ADD COLUMN parent_id UUID REFERENCES dashboard_comments (id) ON DELETE CASCADE;
ALTER TABLE dashboard_comments ADD CONSTRAINT dashboard_comments_dashboard_id_id_key UNIQUE (dashboard_id, id);
ALTER TABLE dashboard_comments ADD CONSTRAINT dashboard_comments_parent_dashboard_fkey
  FOREIGN KEY (dashboard_id, parent_id) REFERENCES dashboard_comments (dashboard_id, id) ON DELETE CASCADE;
ALTER TABLE dashboard_comments ADD COLUMN resolved_by_version INTEGER;
ALTER TABLE dashboard_comments ADD CONSTRAINT dashboard_comments_resolved_by_version_fkey
  FOREIGN KEY (dashboard_id, resolved_by_version) REFERENCES dashboard_versions (dashboard_id, version) ON DELETE SET NULL (resolved_by_version);
ALTER TABLE dashboard_comments ADD CONSTRAINT dashboard_comments_resolved_by_version_check
  CHECK (resolved_by_version IS NULL OR (status = 'resolved' AND resolved_by_version >= version));
ALTER TABLE dashboard_comments ADD CONSTRAINT dashboard_comments_reply_check
  CHECK (parent_id IS NULL OR (status = 'open' AND resolved_at IS NULL AND resolved_by_kind IS NULL));
CREATE INDEX dashboard_comments_parent_idx ON dashboard_comments (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX dashboard_comments_open_threads_idx ON dashboard_comments (dashboard_id) WHERE parent_id IS NULL AND status = 'open';

-- +goose Down
DELETE FROM dashboard_comments WHERE parent_id IS NOT NULL OR panel_id IS NULL;
DROP INDEX dashboard_comments_open_threads_idx;
DROP INDEX dashboard_comments_parent_idx;
ALTER TABLE dashboard_comments DROP CONSTRAINT dashboard_comments_reply_check;
ALTER TABLE dashboard_comments DROP CONSTRAINT dashboard_comments_resolved_by_version_check;
ALTER TABLE dashboard_comments DROP CONSTRAINT dashboard_comments_resolved_by_version_fkey;
ALTER TABLE dashboard_comments DROP COLUMN resolved_by_version;
ALTER TABLE dashboard_comments DROP CONSTRAINT dashboard_comments_parent_dashboard_fkey;
ALTER TABLE dashboard_comments DROP CONSTRAINT dashboard_comments_dashboard_id_id_key;
ALTER TABLE dashboard_comments DROP COLUMN parent_id;
ALTER TABLE dashboard_comments ALTER COLUMN panel_id SET NOT NULL;
