-- name: UpsertJob :exec
INSERT INTO jobs (name, every_seconds, next_run_at)
VALUES (@name, @every_seconds, now())
ON CONFLICT (name) DO UPDATE SET every_seconds = EXCLUDED.every_seconds;

-- name: ClaimJob :one
UPDATE jobs
SET leased_by = @instance_id,
    lease_until = now() + make_interval(secs => LEAST(every_seconds, @max_lease_seconds::int)),
    last_started_at = now()
WHERE name = (
  SELECT j.name FROM jobs j
  WHERE j.name = ANY(@names::text[])
    AND (@force::boolean OR j.next_run_at <= now())
    AND (j.lease_until IS NULL OR j.lease_until < now())
  ORDER BY j.next_run_at, j.name
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
RETURNING name, last_started_at::timestamptz AS started_at, LEAST(every_seconds, @max_lease_seconds::int)::int AS lease_seconds;

-- name: FinishJob :execrows
UPDATE jobs
SET leased_by = '',
    lease_until = NULL,
    last_finished_at = now(),
    last_outcome = @outcome,
    last_error = @last_error,
    next_run_at = last_started_at + make_interval(secs => every_seconds)
WHERE name = @name AND leased_by = @instance_id AND last_started_at = @started_at;

-- name: ReleaseJob :execrows
UPDATE jobs
SET leased_by = '', lease_until = NULL
WHERE name = @name AND leased_by = @instance_id AND last_started_at = @started_at;

-- name: GetJob :one
SELECT name, every_seconds, next_run_at, leased_by, lease_until, last_started_at, last_finished_at, last_outcome, last_error
FROM jobs
WHERE name = @name;
