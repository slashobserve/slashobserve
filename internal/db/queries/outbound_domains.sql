-- name: ListOutboundDomains :many
SELECT domain, created_at, created_by_name, (created_by_id IS NOT NULL)::boolean AS has_author, for_connection
FROM outbound_domains
ORDER BY domain COLLATE "C";

-- name: LockOutboundDomains :exec
SELECT pg_advisory_xact_lock(hashtextextended('outbound_domains', 0));

-- name: CountOutboundDomains :one
SELECT count(*) FROM outbound_domains;

-- name: InsertOutboundDomain :one
INSERT INTO outbound_domains (domain, created_by_id, created_by_name, for_connection)
VALUES (@domain, @created_by_id, @created_by_name, @for_connection)
ON CONFLICT (domain) DO NOTHING
RETURNING domain, created_at, created_by_name, for_connection;

-- name: DeleteOutboundDomain :execrows
DELETE FROM outbound_domains WHERE domain = @domain;

-- name: DeleteConnectionOutboundDomain :execrows
DELETE FROM outbound_domains WHERE domain = @domain AND for_connection;

-- name: ListOutboundUsers :many
SELECT 'webhook'::text AS kind, name, url FROM webhooks
UNION ALL
SELECT 'grafana_connection'::text AS kind, name, url FROM grafana_connections
UNION ALL
SELECT 'datasource'::text AS kind, name, CASE WHEN type = 'clickhouse' THEN 'tcp://' || (config->>'address') ELSE config->>'address' END AS url FROM datasources
UNION ALL
SELECT 'mcp_source'::text AS kind, name, url FROM mcp_sources
ORDER BY kind, name;

-- name: ClaimConnectionOutboundDomain :one
UPDATE outbound_domains SET for_connection = false
WHERE domain = @domain AND for_connection
RETURNING domain, created_at, created_by_name, for_connection;
