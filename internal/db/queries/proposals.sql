-- name: InsertProposal :one
INSERT INTO proposals (dashboard_id, title, author_kind, author_id, author_name, origin, origin_client, run_id)
VALUES (@dashboard_id, @title, @author_kind, @author_id, @author_name,
  sqlc.narg('origin')::text, sqlc.narg('origin_client')::text, sqlc.narg('run_id')::uuid)
RETURNING *;

-- name: InsertProposalRevision :exec
INSERT INTO proposal_revisions (proposal_id, revision, base_version, spec, summary)
VALUES (@proposal_id, @revision, @base_version, @spec, @summary);

-- name: LockProposalSlot :exec
SELECT pg_advisory_xact_lock(hashtextextended('proposals:' || CAST(@dashboard_id::uuid AS text) || ':' || @slot::text, 0));

-- name: LockOpenProposalForAuthor :one
SELECT id, revision FROM proposals
WHERE dashboard_id = @dashboard_id AND state = 'open' AND run_id IS NULL
  AND author_kind = @author_kind AND author_id = @author_id
FOR UPDATE;

-- name: LockOpenProposalForRun :one
SELECT id, revision FROM proposals
WHERE dashboard_id = @dashboard_id AND state = 'open' AND run_id = @run_id
FOR UPDATE;

-- name: AdvanceProposalRevision :one
UPDATE proposals SET revision = revision + 1, updated_at = clock_timestamp()
WHERE id = @id AND state = 'open'
RETURNING revision;

-- name: CloseProposal :execrows
UPDATE proposals
SET state = @state, closed_at = clock_timestamp(),
  closed_by_kind = @closed_by_kind, closed_by_id = @closed_by_id, closed_by_name = @closed_by_name,
  updated_at = clock_timestamp()
WHERE id = @id AND state = 'open' AND revision = @revision;

-- name: GetProposal :one
SELECT sqlc.embed(p), r.base_version, r.summary, r.spec, v.version AS merged_version
FROM proposals p
JOIN proposal_revisions r ON r.proposal_id = p.id AND r.revision = p.revision
LEFT JOIN dashboard_versions v ON v.merged_proposal_id = p.id
WHERE p.id = @id;

-- name: ListDashboardProposals :many
SELECT sqlc.embed(p), r.base_version, r.summary, v.version AS merged_version
FROM proposals p
JOIN proposal_revisions r ON r.proposal_id = p.id AND r.revision = p.revision
LEFT JOIN dashboard_versions v ON v.merged_proposal_id = p.id
WHERE p.dashboard_id = @dashboard_id
  AND (sqlc.narg('state')::text IS NULL OR p.state = sqlc.narg('state')::text)
  AND (sqlc.narg('before_at')::timestamptz IS NULL
    OR (p.created_at, p.id) < (sqlc.narg('before_at')::timestamptz, sqlc.narg('before_id')::uuid))
ORDER BY p.created_at DESC, p.id DESC
LIMIT @row_limit;
