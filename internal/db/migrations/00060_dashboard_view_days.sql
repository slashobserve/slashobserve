-- +goose Up
CREATE TABLE dashboard_view_days (
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  day DATE NOT NULL,
  PRIMARY KEY (dashboard_id, user_id, day)
);
CREATE INDEX dashboard_view_days_day_idx ON dashboard_view_days (day);

-- +goose Down
DROP TABLE dashboard_view_days;
