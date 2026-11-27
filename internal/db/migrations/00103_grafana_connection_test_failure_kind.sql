-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE grafana_connections
  ADD COLUMN IF NOT EXISTS last_test_failure_kind TEXT;

ALTER TABLE grafana_connections
  DROP CONSTRAINT IF EXISTS grafana_connections_untested_check,
  DROP CONSTRAINT IF EXISTS grafana_connections_failure_kind_check,
  ADD CONSTRAINT grafana_connections_untested_check
    CHECK (last_test_at IS NOT NULL OR (NOT last_test_ok AND last_test_message = '' AND last_test_failure_kind IS NULL)) NOT VALID,
  ADD CONSTRAINT grafana_connections_failure_kind_check
    CHECK (last_test_failure_kind IS NULL
      OR (NOT last_test_ok AND last_test_failure_kind IN ('transient', 'credentials', 'configuration', 'unsupported', 'unknown'))) NOT VALID;

ALTER TABLE grafana_connections VALIDATE CONSTRAINT grafana_connections_untested_check;
ALTER TABLE grafana_connections VALIDATE CONSTRAINT grafana_connections_failure_kind_check;

-- +goose Down
ALTER TABLE grafana_connections
  DROP CONSTRAINT IF EXISTS grafana_connections_failure_kind_check,
  DROP CONSTRAINT IF EXISTS grafana_connections_untested_check,
  DROP COLUMN IF EXISTS last_test_failure_kind;
