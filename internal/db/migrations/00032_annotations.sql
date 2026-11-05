-- +goose Up
CREATE TABLE annotations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  panel_id TEXT NOT NULL DEFAULT '',
  time_from TIMESTAMPTZ NOT NULL,
  time_to TIMESTAMPTZ NULL,
  text TEXT NOT NULL CHECK (length(text) <= 2000),
  tags TEXT[] NOT NULL DEFAULT '{}',
  created_by UUID NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX annotations_dashboard_time_idx ON annotations (dashboard_id, time_from);
CREATE INDEX annotations_tags_idx ON annotations USING GIN (tags);

-- +goose Down
DROP TABLE annotations;
