-- +goose Up
CREATE TABLE dashboards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  tags TEXT[] NOT NULL DEFAULT '{}',
  latest_version INTEGER NOT NULL CHECK (latest_version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX dashboards_updated_at_id_idx ON dashboards (updated_at DESC, id DESC);

CREATE TABLE dashboard_versions (
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK (version >= 1),
  parent_version INTEGER CHECK ((version = 1 AND parent_version IS NULL) OR parent_version = version - 1),
  restored_from INTEGER CHECK (restored_from IS NULL OR restored_from < version),
  spec JSONB NOT NULL CHECK (jsonb_typeof(spec) = 'object'),
  summary TEXT NOT NULL CHECK (length(summary) BETWEEN 1 AND 500),
  author_kind TEXT NOT NULL CHECK (author_kind IN ('user', 'service_account')),
  author_id UUID NOT NULL,
  author_name TEXT NOT NULL,
  author_email TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (dashboard_id, version)
);

-- +goose Down
DROP TABLE dashboard_versions;
DROP TABLE dashboards;
