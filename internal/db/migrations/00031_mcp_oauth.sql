-- +goose Up
CREATE TABLE oauth_clients (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text CONSTRAINT oauth_clients_id_check CHECK (length(id) BETWEEN 1 AND 255),
  name TEXT CONSTRAINT oauth_clients_name_check CHECK (name IS NULL OR char_length(name) BETWEEN 1 AND 64),
  redirect_uris TEXT[] NOT NULL CONSTRAINT oauth_clients_redirect_uris_check CHECK (cardinality(redirect_uris) BETWEEN 1 AND 10),
  registered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX oauth_clients_registered_idx ON oauth_clients (registered_at);

CREATE TABLE oauth_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  client_id TEXT NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
  redirect_uri TEXT NOT NULL CONSTRAINT oauth_requests_redirect_uri_check CHECK (length(redirect_uri) BETWEEN 1 AND 2048),
  state TEXT CONSTRAINT oauth_requests_state_check CHECK (state IS NULL OR length(state) BETWEEN 1 AND 1024),
  code_challenge TEXT NOT NULL CONSTRAINT oauth_requests_code_challenge_check CHECK (code_challenge ~ '^[A-Za-z0-9_-]{43}$'),
  resource TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX oauth_requests_expires_idx ON oauth_requests (expires_at);

CREATE TABLE oauth_grants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  client_id TEXT NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  resource TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  last_used_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  revoked_reason TEXT CONSTRAINT oauth_grants_revoked_reason_check
    CHECK (revoked_reason IN ('person', 'admin', 'client', 'replaced', 'refresh_reuse', 'code_reuse')),
  revoked_by UUID REFERENCES users (id) ON DELETE SET NULL,
  CONSTRAINT oauth_grants_revoked_check CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL))
);
CREATE UNIQUE INDEX oauth_grants_live_idx ON oauth_grants (client_id, user_id) WHERE revoked_at IS NULL;
CREATE INDEX oauth_grants_user_idx ON oauth_grants (user_id, created_at DESC, id DESC);
CREATE INDEX oauth_grants_created_idx ON oauth_grants (created_at DESC, id DESC);

CREATE TABLE oauth_codes (
  code_hash BYTEA PRIMARY KEY CONSTRAINT oauth_codes_code_hash_check CHECK (length(code_hash) = 32),
  client_id TEXT NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  redirect_uri TEXT NOT NULL,
  code_challenge TEXT NOT NULL,
  resource TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  grant_id UUID REFERENCES oauth_grants (id) ON DELETE CASCADE,
  CONSTRAINT oauth_codes_used_check CHECK ((used_at IS NULL) = (grant_id IS NULL))
);
CREATE INDEX oauth_codes_expires_idx ON oauth_codes (expires_at);

CREATE TABLE oauth_tokens (
  token_hash BYTEA PRIMARY KEY CONSTRAINT oauth_tokens_token_hash_check CHECK (length(token_hash) = 32),
  grant_id UUID NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CONSTRAINT oauth_tokens_kind_check CHECK (kind IN ('access', 'refresh')),
  expires_at TIMESTAMPTZ NOT NULL,
  rotated_at TIMESTAMPTZ,
  CONSTRAINT oauth_tokens_rotated_check CHECK (kind = 'refresh' OR rotated_at IS NULL)
);
CREATE INDEX oauth_tokens_grant_idx ON oauth_tokens (grant_id);
CREATE INDEX oauth_tokens_expires_idx ON oauth_tokens (expires_at);

-- +goose Down
DROP TABLE oauth_tokens;
DROP TABLE oauth_codes;
DROP TABLE oauth_grants;
DROP TABLE oauth_requests;
DROP TABLE oauth_clients;
