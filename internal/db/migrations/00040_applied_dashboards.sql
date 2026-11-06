-- +goose Up
CREATE TABLE applied_dashboards (
  path TEXT PRIMARY KEY,
  dashboard_id UUID NOT NULL UNIQUE REFERENCES dashboards (id) ON DELETE CASCADE,
  content_sha256 TEXT NOT NULL,
  applied_by_id UUID NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE applied_dashboards;
