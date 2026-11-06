-- +goose Up
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

-- +goose Down
DROP TABLE dashboard_views;
DROP TABLE dashboard_health;
