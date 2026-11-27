-- +goose Up
ALTER TABLE agent_requests ADD COLUMN claim_epoch INTEGER NOT NULL DEFAULT 0;
UPDATE agent_requests SET claim_epoch = attempts WHERE attempts > 0 AND status IN ('queued', 'running');
ALTER TABLE run_tokens ADD COLUMN claim_epoch INTEGER;

-- +goose StatementBegin
CREATE FUNCTION agent_requests_old_server_claim_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.claim_epoch := OLD.claim_epoch + 1;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER agent_requests_old_server_claim_epoch BEFORE UPDATE ON agent_requests
FOR EACH ROW WHEN (NEW.attempts > OLD.attempts AND NEW.claim_epoch = OLD.claim_epoch)
EXECUTE FUNCTION agent_requests_old_server_claim_epoch();

-- +goose Down
DROP TRIGGER agent_requests_old_server_claim_epoch ON agent_requests;
DROP FUNCTION agent_requests_old_server_claim_epoch();
ALTER TABLE run_tokens DROP COLUMN claim_epoch;
ALTER TABLE agent_requests DROP COLUMN claim_epoch;
