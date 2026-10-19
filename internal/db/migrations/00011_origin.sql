-- +goose Up
ALTER TABLE dashboard_versions ADD COLUMN origin TEXT
  CONSTRAINT dashboard_versions_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api'));
ALTER TABLE dashboard_versions ADD COLUMN origin_client TEXT
  CONSTRAINT dashboard_versions_origin_client_check CHECK (origin_client IS NULL OR (origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64));
ALTER TABLE agent_requests ADD COLUMN origin TEXT
  CONSTRAINT agent_requests_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api'));
ALTER TABLE agent_requests ADD COLUMN origin_client TEXT
  CONSTRAINT agent_requests_origin_client_check CHECK (origin_client IS NULL OR (origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64));

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN origin_client;
ALTER TABLE agent_requests DROP COLUMN origin;
ALTER TABLE dashboard_versions DROP COLUMN origin_client;
ALTER TABLE dashboard_versions DROP COLUMN origin;
