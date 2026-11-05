-- +goose Up
CREATE TABLE jobs (
  name TEXT PRIMARY KEY,
  every_seconds INT NOT NULL,
  next_run_at TIMESTAMPTZ NOT NULL,
  leased_by TEXT NOT NULL DEFAULT '',
  lease_until TIMESTAMPTZ NULL,
  last_started_at TIMESTAMPTZ NULL,
  last_finished_at TIMESTAMPTZ NULL,
  last_outcome TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE jobs;
