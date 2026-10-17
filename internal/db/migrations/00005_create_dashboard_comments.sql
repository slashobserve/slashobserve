-- +goose Up
CREATE TABLE dashboard_comments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NOT NULL,
  version INTEGER NOT NULL,
  panel_id TEXT NOT NULL CHECK (length(panel_id) BETWEEN 1 AND 64),
  body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
  author_kind TEXT NOT NULL CHECK (author_kind IN ('user', 'service_account')),
  author_id UUID NOT NULL,
  author_name TEXT NOT NULL,
  author_email TEXT,
  resolved_at TIMESTAMPTZ,
  resolved_by_kind TEXT CHECK (resolved_by_kind IN ('user', 'service_account')),
  resolved_by_id UUID,
  resolved_by_name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY (dashboard_id, version) REFERENCES dashboard_versions (dashboard_id, version) ON DELETE CASCADE,
  CHECK ((status = 'resolved') = (resolved_at IS NOT NULL))
);
CREATE INDEX dashboard_comments_dashboard_created_idx ON dashboard_comments (dashboard_id, created_at, id);

-- +goose Down
DROP TABLE dashboard_comments;
