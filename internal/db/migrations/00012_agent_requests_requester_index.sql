-- +goose Up
CREATE INDEX agent_requests_requester_idx ON agent_requests (requested_by_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX agent_requests_requester_idx;
