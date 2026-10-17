-- +goose Up
CREATE TABLE service_accounts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE CHECK (name ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
  description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 256),
  created_by UUID REFERENCES users (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE service_account_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_account_id UUID NOT NULL REFERENCES service_accounts (id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  display_prefix TEXT NOT NULL,
  token_hash BYTEA NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  created_by UUID REFERENCES users (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ
);
CREATE INDEX service_account_keys_service_account_id_idx ON service_account_keys (service_account_id);

-- +goose Down
DROP TABLE service_account_keys;
DROP TABLE service_accounts;
