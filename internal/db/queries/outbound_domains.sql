-- name: ListOutboundDomains :many
SELECT domain, created_at, created_by_name, (created_by_id IS NOT NULL)::boolean AS has_author
FROM outbound_domains
ORDER BY domain COLLATE "C";

-- name: LockOutboundDomains :exec
SELECT pg_advisory_xact_lock(hashtextextended('outbound_domains', 0));

-- name: CountOutboundDomains :one
SELECT count(*) FROM outbound_domains;

-- name: InsertOutboundDomain :one
INSERT INTO outbound_domains (domain, created_by_id, created_by_name)
VALUES (@domain, @created_by_id, @created_by_name)
ON CONFLICT (domain) DO NOTHING
RETURNING domain, created_at, created_by_name;

-- name: DeleteOutboundDomain :execrows
DELETE FROM outbound_domains WHERE domain = @domain;
