-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE webhook_deliveries
  DROP CONSTRAINT IF EXISTS webhook_deliveries_error_check,
  ADD CONSTRAINT webhook_deliveries_error_check CHECK (error_code IN ('status', 'unreachable', 'blocked_address', 'domain_not_allowed', 'too_large', 'skipped_by_admin', 'expired')) NOT VALID;
ALTER TABLE webhook_deliveries VALIDATE CONSTRAINT webhook_deliveries_error_check;

-- +goose Down
DELETE FROM webhook_deliveries WHERE error_code = 'expired';
ALTER TABLE webhook_deliveries
  DROP CONSTRAINT IF EXISTS webhook_deliveries_error_check,
  ADD CONSTRAINT webhook_deliveries_error_check CHECK (error_code IN ('status', 'unreachable', 'blocked_address', 'domain_not_allowed', 'too_large', 'skipped_by_admin'));
