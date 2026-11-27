-- name: NextRCARevision :one
SELECT (coalesce(max(revision), 0) + 1)::int AS revision, count(*)::int AS total
FROM observation_rcas
WHERE observation_id = @observation_id;

-- name: InsertRCA :one
INSERT INTO observation_rcas (observation_id, revision, title, summary, observation_state, time_from, time_to, content,
  created_by_kind, created_by_id, created_by_name, origin, origin_client, run_id)
VALUES (@observation_id, @revision, @title, sqlc.narg('summary')::text, @observation_state,
  sqlc.narg('time_from')::timestamptz, sqlc.narg('time_to')::timestamptz, @content::jsonb,
  @created_by_kind, @created_by_id, @created_by_name, sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('run_id')::uuid)
RETURNING *;

-- name: RCAByRevision :one
SELECT * FROM observation_rcas WHERE observation_id = @observation_id AND revision = @revision;

-- name: ListRCAs :many
SELECT r.id, r.observation_id, r.revision, r.title, r.summary, r.observation_state,
  r.created_by_kind, r.created_by_id, r.created_by_name, r.origin, r.origin_client, r.run_id, r.created_at,
  (SELECT count(*) FROM public_links p WHERE p.rca_id = r.id AND (p.expires_at IS NULL OR p.expires_at > now()))::int AS live_links
FROM observation_rcas r
WHERE r.observation_id = @observation_id
ORDER BY r.revision DESC;

-- name: RCASnapshot :one
SELECT s.* FROM snapshots s WHERE s.id = @snapshot_id AND s.observation_id = @observation_id;

-- name: RCAHasUserScopedSnapshot :one
SELECT EXISTS (
  SELECT 1
  FROM observation_rcas r
  CROSS JOIN LATERAL jsonb_array_elements(r.content -> 'blocks') AS item
  JOIN snapshots s ON s.id::text = item ->> 'snapshot_id' AND s.observation_id = r.observation_id
  WHERE r.id = @rca_id AND s.user_scoped
) AS user_scoped;

-- name: LockRCAByRevision :one
SELECT id FROM observation_rcas
WHERE observation_id = @observation_id AND revision = @revision
FOR UPDATE;

-- name: LiveRCALinkCount :one
SELECT count(*)::int AS total FROM public_links
WHERE rca_id = @rca_id AND (expires_at IS NULL OR expires_at > now());

-- name: DeleteRCA :execrows
DELETE FROM observation_rcas WHERE id = @id;
