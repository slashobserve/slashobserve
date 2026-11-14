-- +goose Up
CREATE INDEX agent_requests_listed_recent_idx ON agent_requests (created_at DESC, id DESC) WHERE source <> 'suggest' AND archived_at IS NULL;

-- +goose Down
DROP INDEX agent_requests_listed_recent_idx;
