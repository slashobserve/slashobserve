-- +goose Up
ALTER TABLE agent_requests ADD COLUMN claim_epoch INTEGER NOT NULL DEFAULT 0;
UPDATE agent_requests SET claim_epoch = attempts WHERE attempts > 0 AND status IN ('queued', 'running');
ALTER TABLE run_tokens ADD COLUMN claim_epoch INTEGER;

-- +goose Down
ALTER TABLE run_tokens DROP COLUMN claim_epoch;
ALTER TABLE agent_requests DROP COLUMN claim_epoch;
