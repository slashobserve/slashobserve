-- +goose Up
CREATE TABLE dashboard_stars (
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  starred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, dashboard_id)
);
CREATE INDEX dashboard_stars_dashboard_idx ON dashboard_stars (dashboard_id);

-- +goose Down
DROP TABLE dashboard_stars;
