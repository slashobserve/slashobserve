-- +goose Up
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

-- +goose Down
DROP TABLE provision_errors;
DROP TABLE provisioned_dashboards;
