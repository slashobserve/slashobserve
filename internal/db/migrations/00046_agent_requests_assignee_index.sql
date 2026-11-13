-- +goose Up
CREATE INDEX agent_requests_assignee_recent_idx ON agent_requests (assigned_kind, assigned_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX agent_requests_assignee_recent_idx;
