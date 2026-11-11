-- name: UpsertServerInstance :exec
INSERT INTO server_instances (id, version) VALUES (@id, @version)
ON CONFLICT (id) DO UPDATE SET seen_at = now(), version = EXCLUDED.version;

-- name: DeleteStaleServerInstances :exec
DELETE FROM server_instances WHERE seen_at < now() - make_interval(secs => @stale_seconds::double precision);

-- name: CountLiveServerInstances :one
SELECT count(*)::int FROM server_instances WHERE seen_at >= now() - make_interval(secs => @stale_seconds::double precision);

-- name: DeleteServerInstance :exec
DELETE FROM server_instances WHERE id = @id;
