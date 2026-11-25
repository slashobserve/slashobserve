-- +goose Up
CREATE TABLE context_sources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT NOT NULL CONSTRAINT context_sources_uid_key UNIQUE CONSTRAINT context_sources_uid_check CHECK (uid ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$'),
  name TEXT NOT NULL CONSTRAINT context_sources_name_check CHECK (length(name) BETWEEN 1 AND 100),
  description TEXT CONSTRAINT context_sources_description_check CHECK (description IS NULL OR length(description) BETWEEN 1 AND 300),
  url TEXT NOT NULL CONSTRAINT context_sources_url_check CHECK (length(url) BETWEEN 1 AND 2048),
  audience TEXT NOT NULL DEFAULT 'everyone' CONSTRAINT context_sources_audience_check CHECK (audience IN ('everyone', 'admins')),
  secret_header TEXT CONSTRAINT context_sources_secret_header_check CHECK (secret_header IS NULL OR length(secret_header) BETWEEN 1 AND 100),
  secret BYTEA CONSTRAINT context_sources_secret_check CHECK (secret IS NULL OR octet_length(secret) BETWEEN 1 AND 8192),
  provisioned BOOLEAN NOT NULL DEFAULT false,
  checked_at TIMESTAMPTZ,
  failure TEXT CONSTRAINT context_sources_failure_check CHECK (failure IS NULL OR checked_at IS NOT NULL),
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT context_sources_secret_header_pair_check CHECK (secret IS NULL OR secret_header IS NOT NULL)
);

CREATE TABLE context_source_tools (
  source_id UUID NOT NULL REFERENCES context_sources (id) ON DELETE CASCADE,
  name TEXT NOT NULL CONSTRAINT context_source_tools_name_check CHECK (length(name) BETWEEN 1 AND 128),
  state TEXT NOT NULL CONSTRAINT context_source_tools_state_check CHECK (state IN ('offered', 'approved', 'changed', 'gone', 'refused')),
  refusal TEXT,
  description TEXT NOT NULL,
  input_schema JSONB NOT NULL CONSTRAINT context_source_tools_schema_check CHECK (jsonb_typeof(input_schema) = 'object'),
  hash TEXT NOT NULL CONSTRAINT context_source_tools_hash_check CHECK (hash ~ '^[0-9a-f]{64}$'),
  seen_description TEXT,
  seen_input_schema JSONB,
  seen_hash TEXT,
  approved_by UUID,
  approved_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (source_id, name),
  CONSTRAINT context_source_tools_refusal_check CHECK ((state = 'refused') = (refusal IS NOT NULL)),
  CONSTRAINT context_source_tools_seen_check CHECK ((state = 'changed') = (seen_hash IS NOT NULL AND seen_description IS NOT NULL AND seen_input_schema IS NOT NULL)),
  CONSTRAINT context_source_tools_approved_check CHECK ((state IN ('approved', 'changed', 'gone')) = (approved_at IS NOT NULL))
);

-- +goose Down
DROP TABLE context_source_tools;
DROP TABLE context_sources;
