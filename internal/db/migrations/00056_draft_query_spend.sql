-- +goose Up
CREATE TABLE draft_query_spend (
  principal_key TEXT NOT NULL,
  window_start TIMESTAMPTZ NOT NULL,
  calls INTEGER NOT NULL CHECK (calls > 0),
  PRIMARY KEY (principal_key, window_start)
);

-- +goose Down
DROP TABLE draft_query_spend;
