-- +goose Up
CREATE TABLE webhooks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL CONSTRAINT webhooks_name_check CHECK (char_length(name) BETWEEN 1 AND 100),
  url TEXT NOT NULL CONSTRAINT webhooks_url_check CHECK (octet_length(url) <= 2048 AND url LIKE 'https://%'),
  secret_ciphertext BYTEA NOT NULL,
  event_types TEXT[] NOT NULL CONSTRAINT webhooks_event_types_check CHECK (cardinality(event_types) >= 1),
  owner_kind TEXT NOT NULL CONSTRAINT webhooks_owner_kind_check CHECK (owner_kind IN ('user', 'service_account')),
  owner_id UUID NOT NULL,
  cursor_xid XID8 NOT NULL,
  cursor_seq BIGINT NOT NULL,
  consecutive_failures INTEGER NOT NULL DEFAULT 0 CONSTRAINT webhooks_failures_check CHECK (consecutive_failures >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_id UUID,
  leased_by TEXT,
  lease_until TIMESTAMPTZ,
  disabled_at TIMESTAMPTZ,
  disabled_reason TEXT CONSTRAINT webhooks_disabled_reason_check
    CHECK (disabled_reason IN ('manual', 'gone', 'failing', 'owner', 'secret_unreadable')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT webhooks_disabled_check CHECK ((disabled_at IS NULL) = (disabled_reason IS NULL)),
  CONSTRAINT webhooks_lease_check CHECK ((lease_id IS NULL) = (lease_until IS NULL) AND (lease_id IS NULL) = (leased_by IS NULL))
);
CREATE INDEX webhooks_due_idx ON webhooks (next_attempt_at) WHERE disabled_at IS NULL;

CREATE TABLE webhook_deliveries (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  webhook_id UUID NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
  delivery_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  activity_seq BIGINT,
  attempted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  duration_ms INTEGER NOT NULL CONSTRAINT webhook_deliveries_duration_check CHECK (duration_ms >= 0),
  status_code INTEGER CONSTRAINT webhook_deliveries_status_check CHECK (status_code BETWEEN 100 AND 599),
  outcome TEXT NOT NULL CONSTRAINT webhook_deliveries_outcome_check CHECK (outcome IN ('delivered', 'failed', 'skipped')),
  error_code TEXT CONSTRAINT webhook_deliveries_error_check
    CHECK (error_code IN ('status', 'unreachable', 'blocked_address', 'domain_not_allowed', 'too_large')),
  CONSTRAINT webhook_deliveries_error_presence_check CHECK ((outcome = 'delivered') = (error_code IS NULL)),
  CONSTRAINT webhook_deliveries_test_check CHECK ((event_type = 'webhook.test') = (activity_seq IS NULL))
);
CREATE INDEX webhook_deliveries_webhook_idx ON webhook_deliveries (webhook_id, id DESC);
CREATE INDEX webhook_deliveries_attempted_idx ON webhook_deliveries (attempted_at);

-- +goose Down
DROP TABLE webhook_deliveries;
DROP TABLE webhooks;
