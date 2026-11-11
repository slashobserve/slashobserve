-- +goose Up
ALTER TABLE agent_requests
  ADD COLUMN state TEXT,
  ADD COLUMN result_kind TEXT NOT NULL DEFAULT 'none';

UPDATE agent_requests SET state = CASE
  WHEN status = 'running' AND cancel_requested_at IS NOT NULL THEN 'canceling'
  WHEN status = 'failed' AND failure_code = 'canceled' THEN 'canceled'
  ELSE status END;

UPDATE agent_requests r SET result_kind = CASE
  WHEN r.build_saved_at IS NOT NULL THEN 'partial_version'
  WHEN r.source = 'suggest' AND r.suggestion IS NOT NULL THEN 'suggestion'
  WHEN EXISTS (SELECT 1 FROM findings f WHERE f.agent_request_id = r.id) THEN 'observation'
  WHEN r.result_version IS NOT NULL THEN 'version'
  ELSE 'none' END;

ALTER TABLE agent_requests
  ALTER COLUMN state SET NOT NULL,
  ALTER COLUMN state SET DEFAULT 'queued',
  ADD CONSTRAINT agent_requests_state_check
    CHECK (state IN ('queued', 'running', 'canceling', 'succeeded', 'failed', 'canceled')),
  ADD CONSTRAINT agent_requests_result_kind_check
    CHECK (result_kind IN ('none', 'version', 'partial_version', 'proposal', 'suggestion', 'observation')),
  ADD CONSTRAINT agent_requests_state_status_check CHECK (
    (state = 'queued' AND status = 'queued')
    OR (state IN ('running', 'canceling') AND status = 'running')
    OR (state = 'succeeded' AND status = 'succeeded')
    OR (state = 'failed' AND status = 'failed' AND failure_code IS DISTINCT FROM 'canceled')
    OR (state = 'canceled' AND status = 'failed' AND failure_code IS NOT DISTINCT FROM 'canceled')),
  ADD CONSTRAINT agent_requests_canceling_check
    CHECK (state <> 'canceling' OR cancel_requested_at IS NOT NULL);

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN result_kind, DROP COLUMN state;
