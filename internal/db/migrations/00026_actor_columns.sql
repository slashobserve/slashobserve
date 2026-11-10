-- +goose Up
ALTER TABLE dashboards ADD COLUMN created_run_id UUID REFERENCES agent_requests (id) ON DELETE SET NULL;
UPDATE dashboards d SET created_run_id = v.agent_request_id
FROM dashboard_versions v
WHERE v.dashboard_id = d.id AND v.version = 1 AND v.agent_request_id IS NOT NULL;

ALTER TABLE findings
  ADD COLUMN origin TEXT CONSTRAINT findings_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  ADD COLUMN origin_client TEXT CONSTRAINT findings_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64));

ALTER TABLE annotations
  ADD COLUMN created_by_kind TEXT CONSTRAINT annotations_created_by_kind_check CHECK (created_by_kind IN ('user', 'service_account')),
  ADD COLUMN origin TEXT CONSTRAINT annotations_origin_check CHECK (origin IN ('web', 'cli', 'mcp', 'api')),
  ADD COLUMN origin_client TEXT CONSTRAINT annotations_origin_client_check
    CHECK (origin_client IS NULL OR (origin IS NOT NULL AND origin = 'mcp' AND length(origin_client) BETWEEN 1 AND 64)),
  ADD CONSTRAINT annotations_created_by_kind_id_check CHECK (created_by_kind IS NULL OR created_by IS NOT NULL);
UPDATE annotations a SET created_by_kind = CASE
    WHEN EXISTS (SELECT 1 FROM users u WHERE u.id = a.created_by) THEN 'user'
    WHEN EXISTS (SELECT 1 FROM service_accounts s WHERE s.id = a.created_by) THEN 'service_account'
  END
WHERE a.created_by IS NOT NULL;

-- +goose Down
ALTER TABLE annotations
  DROP CONSTRAINT annotations_created_by_kind_id_check,
  DROP COLUMN origin_client,
  DROP COLUMN origin,
  DROP COLUMN created_by_kind;
ALTER TABLE findings DROP COLUMN origin_client, DROP COLUMN origin;
ALTER TABLE dashboards DROP COLUMN created_run_id;
