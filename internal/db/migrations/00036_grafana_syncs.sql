-- +goose Up
CREATE TABLE grafana_syncs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id UUID NOT NULL REFERENCES grafana_connections (id) ON DELETE CASCADE,
  state TEXT NOT NULL CONSTRAINT grafana_syncs_state_check CHECK (state IN ('running', 'finished', 'failed', 'canceled')),
  request JSONB CONSTRAINT grafana_syncs_request_check CHECK (request IS NULL OR jsonb_typeof(request) = 'object'),
  started_by_kind TEXT CONSTRAINT grafana_syncs_started_by_kind_check CHECK (started_by_kind IN ('user', 'service_account')),
  started_by_id UUID,
  total INTEGER NOT NULL DEFAULT 0 CONSTRAINT grafana_syncs_total_check CHECK (total >= 0),
  done INTEGER NOT NULL DEFAULT 0 CONSTRAINT grafana_syncs_done_check CHECK (done >= 0),
  failed INTEGER NOT NULL DEFAULT 0 CONSTRAINT grafana_syncs_failed_check CHECK (failed >= 0),
  skipped INTEGER NOT NULL DEFAULT 0 CONSTRAINT grafana_syncs_skipped_check CHECK (skipped >= 0),
  error TEXT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  finished_at TIMESTAMPTZ,
  lease_id UUID,
  leased_by TEXT,
  lease_until TIMESTAMPTZ,
  lease_losses INTEGER NOT NULL DEFAULT 0 CONSTRAINT grafana_syncs_lease_losses_check CHECK (lease_losses BETWEEN 0 AND 2),
  CONSTRAINT grafana_syncs_counts_check CHECK (failed + skipped <= done),
  CONSTRAINT grafana_syncs_finished_check CHECK ((state = 'running') = (finished_at IS NULL)),
  CONSTRAINT grafana_syncs_error_check CHECK (error IS NULL OR state = 'failed'),
  CONSTRAINT grafana_syncs_lease_check CHECK ((lease_id IS NULL) = (lease_until IS NULL) AND (lease_id IS NULL) = (leased_by IS NULL)),
  CONSTRAINT grafana_syncs_lease_state_check CHECK (state = 'running' OR lease_id IS NULL),
  CONSTRAINT grafana_syncs_resumable_check CHECK (state <> 'running' OR (request IS NOT NULL AND started_by_kind IS NOT NULL AND started_by_id IS NOT NULL))
);
CREATE UNIQUE INDEX grafana_syncs_running_idx ON grafana_syncs (connection_id) WHERE state = 'running';
CREATE INDEX grafana_syncs_connection_idx ON grafana_syncs (connection_id, started_at DESC, id DESC);

CREATE TABLE grafana_sync_items (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  sync_id UUID NOT NULL REFERENCES grafana_syncs (id) ON DELETE CASCADE,
  uid TEXT NOT NULL,
  title TEXT NOT NULL,
  outcome TEXT NOT NULL CONSTRAINT grafana_sync_items_outcome_check CHECK (outcome IN ('imported', 'updated', 'skipped', 'failed')),
  dashboard_id UUID,
  unverified_panels INTEGER NOT NULL DEFAULT 0,
  placeholder_panels INTEGER NOT NULL DEFAULT 0,
  error TEXT,
  CONSTRAINT grafana_sync_items_error_check CHECK ((outcome = 'failed') = (error IS NOT NULL)),
  CONSTRAINT grafana_sync_items_uid_key UNIQUE (sync_id, uid)
);
CREATE INDEX grafana_sync_items_sync_idx ON grafana_sync_items (sync_id, id);

INSERT INTO grafana_syncs (id, connection_id, state, total, done, failed, skipped, error, started_at, finished_at)
SELECT (c.last_sync->>'id')::uuid,
  c.id,
  c.last_sync->>'status',
  coalesce((c.last_sync->>'total')::integer, 0),
  coalesce((c.last_sync->>'done')::integer, 0),
  coalesce((c.last_sync->>'failed')::integer, 0),
  coalesce((c.last_sync->>'skipped')::integer, 0),
  CASE WHEN c.last_sync->>'status' = 'failed' THEN nullif(c.last_sync->>'error', '') END,
  coalesce((c.last_sync->>'started_at')::timestamptz, c.updated_at),
  coalesce((c.last_sync->>'finished_at')::timestamptz, (c.last_sync->>'started_at')::timestamptz, c.updated_at)
FROM grafana_connections c
WHERE c.last_sync->>'status' IN ('finished', 'failed', 'canceled')
  AND c.last_sync->>'id' ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';

-- +goose Down
UPDATE grafana_connections c
SET last_sync = jsonb_strip_nulls(jsonb_build_object(
  'id', s.id,
  'status', s.state,
  'total', s.total,
  'done', s.done,
  'failed', s.failed,
  'skipped', s.skipped,
  'error', s.error,
  'started_at', to_char(s.started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
  'finished_at', to_char(s.finished_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')))
FROM (
  SELECT DISTINCT ON (connection_id) * FROM grafana_syncs
  WHERE finished_at IS NOT NULL
  ORDER BY connection_id, finished_at DESC, id DESC
) s
WHERE s.connection_id = c.id;
DROP TABLE grafana_sync_items;
DROP TABLE grafana_syncs;
