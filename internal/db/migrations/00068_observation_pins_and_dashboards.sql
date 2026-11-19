-- +goose Up
CREATE TABLE observation_pins (
  observation_id UUID NOT NULL REFERENCES observations (id) ON DELETE CASCADE,
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  annotation_id UUID NOT NULL REFERENCES annotations (id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (observation_id, dashboard_id)
);
CREATE INDEX observation_pins_annotation_idx ON observation_pins (annotation_id);

CREATE TABLE observation_dashboards (
  observation_id UUID NOT NULL REFERENCES observations (id) ON DELETE CASCADE,
  block_id UUID REFERENCES observation_blocks (id) ON DELETE CASCADE,
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX observation_dashboards_observation_key
  ON observation_dashboards (observation_id, created_by_kind, created_by_id) WHERE block_id IS NULL;
CREATE UNIQUE INDEX observation_dashboards_block_key
  ON observation_dashboards (block_id, created_by_kind, created_by_id) WHERE block_id IS NOT NULL;
CREATE INDEX observation_dashboards_dashboard_idx ON observation_dashboards (dashboard_id);

-- +goose Down
DROP TABLE observation_dashboards;
DROP TABLE observation_pins;
