-- +goose Up
CREATE TABLE home_dashboards (
  user_id UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  dashboard_id UUID NOT NULL,
  set_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE home_dashboards;
