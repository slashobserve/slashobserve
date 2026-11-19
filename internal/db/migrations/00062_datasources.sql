-- +goose Up
CREATE TABLE datasources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT NOT NULL CONSTRAINT datasources_uid_key UNIQUE CONSTRAINT datasources_uid_check CHECK (uid ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$'),
  type TEXT NOT NULL CONSTRAINT datasources_type_check CHECK (type IN ('prometheus', 'clickhouse', 'loki', 'tempo', 'elasticsearch', 'opensearch')),
  name TEXT NOT NULL CONSTRAINT datasources_name_check CHECK (length(name) BETWEEN 1 AND 100),
  config JSONB NOT NULL CONSTRAINT datasources_config_check CHECK (jsonb_typeof(config) = 'object'),
  secrets BYTEA CONSTRAINT datasources_secrets_check CHECK (secrets IS NULL OR octet_length(secrets) BETWEEN 1 AND 65536),
  secret_names TEXT[] NOT NULL DEFAULT '{}' CONSTRAINT datasources_secret_names_check CHECK ((secrets IS NULL) = (cardinality(secret_names) = 0)),
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'observation', 'proposal',
    'user', 'service_account', 'webhook', 'mcp_client', 'outbound_domain', 'grafana_connection', 'public_link', 'datasource'));

-- +goose Down
DELETE FROM activity WHERE target_kind = 'datasource';
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'observation', 'proposal',
    'user', 'service_account', 'webhook', 'mcp_client', 'outbound_domain', 'grafana_connection', 'public_link'));
DROP TABLE datasources;
