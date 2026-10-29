-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION observespec_datasource_uids(spec jsonb) RETURNS text[]
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $$
WITH refs AS (
  SELECT jsonb_path_query(spec, 'lax $.panels[*].datasource ? (@.type() == "string")') #>> '{}' AS ref
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.variables[*].datasource ? (@.type() == "string")') #>> '{}'
), defaults AS (
  SELECT v ->> 'name' AS name, v ->> 'default' AS uid
  FROM jsonb_path_query(spec, 'lax $.variables[*] ? (@."type" == "datasource" && @."default".type() == "string")') AS v
)
SELECT coalesce(array_agg(DISTINCT uid COLLATE "C" ORDER BY uid COLLATE "C"), '{}'::text[]) FROM (
  SELECT ref AS uid FROM refs WHERE left(ref, 2) <> '${'
  UNION ALL SELECT d.uid FROM defaults d WHERE '${' || d.name || '}' IN (SELECT ref FROM refs)
) found
WHERE uid <> ''
$$;
-- +goose StatementEnd

UPDATE dashboard_versions SET spec = spec;

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION observespec_datasource_uids(spec jsonb) RETURNS text[]
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
AS $$
SELECT coalesce(array_agg(DISTINCT uid COLLATE "C" ORDER BY uid COLLATE "C"), '{}'::text[]) FROM (
  SELECT jsonb_path_query(spec, 'lax $.panels[*].datasource ? (@.type() == "string")') #>> '{}' AS uid
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.variables[*].datasource ? (@.type() == "string")') #>> '{}'
) found
WHERE uid <> ''
$$;
-- +goose StatementEnd

UPDATE dashboard_versions SET spec = spec;
