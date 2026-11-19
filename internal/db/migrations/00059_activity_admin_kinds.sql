-- +goose Up
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'observation', 'proposal',
    'user', 'service_account', 'webhook', 'mcp_client', 'outbound_domain', 'grafana_connection', 'public_link'));

-- +goose Down
DELETE FROM activity WHERE target_kind IN ('user', 'service_account', 'webhook', 'mcp_client', 'outbound_domain', 'grafana_connection', 'public_link');
ALTER TABLE activity DROP CONSTRAINT activity_target_kind_check;
ALTER TABLE activity ADD CONSTRAINT activity_target_kind_check
  CHECK (target_kind IN ('dashboard', 'annotation', 'finding', 'run', 'thread', 'saved_search', 'observation', 'proposal'));
