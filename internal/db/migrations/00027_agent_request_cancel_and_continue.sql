-- +goose Up
ALTER TABLE agent_requests
  ADD COLUMN cancel_requested_at TIMESTAMPTZ,
  ADD COLUMN canceled_by_kind TEXT CHECK (canceled_by_kind IN ('user', 'service_account')),
  ADD COLUMN canceled_by_id UUID,
  ADD COLUMN canceled_by_name TEXT,
  ADD COLUMN continues_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL,
  ADD COLUMN continue_note TEXT;
ALTER TABLE agent_requests ADD CONSTRAINT agent_requests_canceled_by_check
  CHECK ((cancel_requested_at IS NULL) = (canceled_by_kind IS NULL)
    AND (canceled_by_kind IS NULL) = (canceled_by_id IS NULL)
    AND (canceled_by_id IS NULL) = (canceled_by_name IS NULL));
CREATE UNIQUE INDEX agent_requests_continues_key ON agent_requests (continues_id) WHERE continues_id IS NOT NULL;

-- +goose Down
DROP INDEX agent_requests_continues_key;
ALTER TABLE agent_requests DROP CONSTRAINT agent_requests_canceled_by_check;
ALTER TABLE agent_requests
  DROP COLUMN continue_note,
  DROP COLUMN continues_id,
  DROP COLUMN canceled_by_name,
  DROP COLUMN canceled_by_id,
  DROP COLUMN canceled_by_kind,
  DROP COLUMN cancel_requested_at;
