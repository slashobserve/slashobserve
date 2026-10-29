-- +goose Up
CREATE TABLE outbound_domains (
  domain TEXT PRIMARY KEY CHECK (domain = lower(domain) AND domain <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by_id UUID REFERENCES users(id) ON DELETE SET NULL,
  created_by_name TEXT NOT NULL
);

-- +goose Down
DROP TABLE outbound_domains;
