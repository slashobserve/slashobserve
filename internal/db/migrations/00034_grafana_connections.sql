-- +goose Up
CREATE TABLE grafana_connections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  url TEXT NOT NULL CHECK (length(url) BETWEEN 1 AND 2048),
  token_ciphertext BYTEA NOT NULL CHECK (octet_length(token_ciphertext) BETWEEN 1 AND 8192),
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_test_at TIMESTAMPTZ,
  last_test_ok BOOLEAN NOT NULL DEFAULT false,
  last_test_message TEXT NOT NULL DEFAULT '',
  last_sync JSONB CHECK (last_sync IS NULL OR jsonb_typeof(last_sync) = 'object')
);
CREATE INDEX grafana_connections_created_at_id_idx ON grafana_connections (created_at, id);

-- +goose Down
DROP TABLE grafana_connections;
