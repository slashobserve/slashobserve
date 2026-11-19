-- +goose Up
ALTER TABLE users ADD COLUMN threads_seen_since TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE thread_reads (
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  thread_id UUID NOT NULL REFERENCES threads (id) ON DELETE CASCADE,
  read_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, thread_id)
);

CREATE INDEX thread_reads_thread_idx ON thread_reads (thread_id);

CREATE TABLE dashboard_viewers (
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  seen_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (dashboard_id, user_id)
);

CREATE INDEX dashboard_viewers_seen_idx ON dashboard_viewers (seen_at);

-- +goose Down
DROP TABLE dashboard_viewers;
DROP TABLE thread_reads;
ALTER TABLE users DROP COLUMN threads_seen_since;
