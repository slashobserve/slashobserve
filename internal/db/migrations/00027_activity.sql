-- +goose Up
CREATE TABLE activity (
  seq BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  xid XID8 NOT NULL DEFAULT pg_current_xact_id(),
  type TEXT NOT NULL CONSTRAINT activity_type_check CHECK (type ~ '^[a-z_]+\.[a-z_]+$'),
  target_kind TEXT NOT NULL CONSTRAINT activity_target_kind_check CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run')),
  target_id UUID NOT NULL,
  dashboard_id UUID,
  folder_id UUID REFERENCES folders (id) ON DELETE CASCADE,
  actor_kind TEXT NOT NULL CONSTRAINT activity_actor_kind_check CHECK (actor_kind IN ('user', 'service_account')),
  actor_id UUID NOT NULL,
  actor_name TEXT NOT NULL,
  origin TEXT CONSTRAINT activity_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  origin_client TEXT CONSTRAINT activity_origin_client_check
    CHECK (origin_client IS NULL OR (origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  run_id UUID,
  data JSONB NOT NULL DEFAULT '{}'::jsonb CONSTRAINT activity_data_check CHECK (jsonb_typeof(data) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT activity_type_target_check CHECK (split_part(type, '.', 1) = target_kind)
);
CREATE INDEX activity_position_idx ON activity (xid, seq);
CREATE INDEX activity_dashboard_idx ON activity (dashboard_id, xid, seq) WHERE dashboard_id IS NOT NULL;
CREATE INDEX activity_folder_idx ON activity (folder_id, xid, seq);
CREATE INDEX activity_created_idx ON activity (created_at);

-- +goose Down
DROP TABLE activity;
