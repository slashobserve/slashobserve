-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION observespec_datasource_uids(spec jsonb) RETURNS text[]
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $$
SELECT coalesce(array_agg(DISTINCT uid ORDER BY uid), '{}'::text[]) FROM (
  SELECT jsonb_path_query(spec, 'lax $.panels[*].datasource ? (@.type() == "string")') #>> '{}' AS uid
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].datasource.uid ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.variables[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.variables[*].datasource.uid ? (@.type() == "string")') #>> '{}'
) found
WHERE uid <> ''
$$;
-- +goose StatementEnd

ALTER TABLE dashboard_versions ADD COLUMN datasource_uids TEXT[] NOT NULL
  GENERATED ALWAYS AS (observespec_datasource_uids(spec)) STORED;

ALTER TABLE dashboards
  ADD COLUMN created_by_kind TEXT,
  ADD COLUMN created_by_id UUID,
  ADD COLUMN created_by_name TEXT,
  ADD COLUMN created_by_email TEXT,
  ADD COLUMN created_origin TEXT,
  ADD COLUMN created_origin_client TEXT,
  ADD COLUMN created_via TEXT NOT NULL DEFAULT 'direct';
UPDATE dashboards d SET
  created_by_kind = v.author_kind, created_by_id = v.author_id, created_by_name = v.author_name,
  created_by_email = v.author_email, created_origin = v.origin, created_origin_client = v.origin_client,
  created_via = CASE WHEN v.agent_request_id IS NOT NULL THEN 'agent' ELSE 'direct' END
FROM dashboard_versions v
WHERE v.dashboard_id = d.id AND v.version = 1;
ALTER TABLE dashboards
  ALTER COLUMN created_by_kind SET NOT NULL,
  ALTER COLUMN created_by_id SET NOT NULL,
  ALTER COLUMN created_by_name SET NOT NULL,
  ALTER COLUMN created_via DROP DEFAULT,
  ADD CONSTRAINT dashboards_created_by_kind_check CHECK (created_by_kind IN ('user', 'service_account')),
  ADD CONSTRAINT dashboards_created_origin_check CHECK (created_origin IN ('web', 'cli', 'mcp', 'api')),
  ADD CONSTRAINT dashboards_created_via_check CHECK (created_via IN ('direct', 'agent'));
CREATE INDEX dashboards_created_by_idx ON dashboards (created_by_id);

-- +goose Down
DROP INDEX dashboards_created_by_idx;
ALTER TABLE dashboards
  DROP COLUMN created_via, DROP COLUMN created_origin_client, DROP COLUMN created_origin,
  DROP COLUMN created_by_email, DROP COLUMN created_by_name, DROP COLUMN created_by_id, DROP COLUMN created_by_kind;
ALTER TABLE dashboard_versions DROP COLUMN datasource_uids;
DROP FUNCTION observespec_datasource_uids(jsonb);
