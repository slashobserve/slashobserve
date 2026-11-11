-- +goose Up
CREATE TABLE server_instances (
  id TEXT PRIMARY KEY CONSTRAINT server_instances_id_check CHECK (length(id) BETWEEN 1 AND 200),
  version TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE server_instances;
