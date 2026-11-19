-- +goose Up
ALTER TABLE webhooks ADD COLUMN format TEXT NOT NULL DEFAULT 'generic'
  CONSTRAINT webhooks_format_check CHECK (format IN ('generic', 'slack', 'teams'));

ALTER TABLE webhook_deliveries DROP CONSTRAINT webhook_deliveries_error_check;
ALTER TABLE webhook_deliveries ADD CONSTRAINT webhook_deliveries_error_check
  CHECK (error_code IN ('status', 'unreachable', 'blocked_address', 'domain_not_allowed', 'too_large', 'skipped_by_admin'));

-- +goose Down
DELETE FROM webhook_deliveries WHERE error_code = 'skipped_by_admin';
ALTER TABLE webhook_deliveries DROP CONSTRAINT webhook_deliveries_error_check;
ALTER TABLE webhook_deliveries ADD CONSTRAINT webhook_deliveries_error_check
  CHECK (error_code IN ('status', 'unreachable', 'blocked_address', 'domain_not_allowed', 'too_large'));
ALTER TABLE webhooks DROP COLUMN format;
