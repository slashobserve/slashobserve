-- +goose Up
CREATE TABLE imports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NOT NULL,
  version INTEGER NOT NULL,
  source TEXT NOT NULL CHECK (source = 'grafana'),
  importer_version INTEGER NOT NULL CHECK (importer_version >= 1),
  original BYTEA NOT NULL CHECK (octet_length(original) BETWEEN 1 AND 8388608),
  mapping JSONB NOT NULL CHECK (jsonb_typeof(mapping) = 'object'),
  summary JSONB NOT NULL CHECK (jsonb_typeof(summary) = 'object'),
  datasources JSONB NOT NULL CHECK (jsonb_typeof(datasources) = 'array'),
  report JSONB NOT NULL CHECK (jsonb_typeof(report) = 'object'),
  title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL,
  created_by_email TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT imports_version_fkey FOREIGN KEY (dashboard_id, version) REFERENCES dashboard_versions (dashboard_id, version) ON DELETE CASCADE,
  CONSTRAINT imports_dashboard_id_key UNIQUE (dashboard_id)
);
CREATE INDEX imports_created_at_id_idx ON imports (created_at DESC, id DESC);

-- +goose Down
DROP TABLE imports;
