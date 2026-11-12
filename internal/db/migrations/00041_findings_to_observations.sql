-- +goose Up
ALTER TABLE findings ADD COLUMN observation_id UUID REFERENCES observations (id) ON DELETE CASCADE;

UPDATE findings f SET observation_id = r.observation_id
FROM agent_requests r
WHERE r.id = f.agent_request_id AND r.observation_id IS NOT NULL
  AND EXISTS (SELECT 1 FROM observation_blocks b
              WHERE b.observation_id = r.observation_id AND b.kind = 'text' AND b.run_id = f.agent_request_id);

INSERT INTO observations (id, folder_id, title, state, summary, time_from, time_to,
  created_by_kind, created_by_id, created_by_name, origin, origin_client, run_id, resolved_at, created_at, updated_at)
SELECT f.id, coalesce(f.folder_id, d.folder_id, p.id, general_folder_id()), f.title, 'resolved',
  left(coalesce(nullif(btrim(ltrim(btrim(split_part(btrim(f.markdown, E' \t\r\n'), E'\n', 1), E' \t\r'), '#>-* ')), ''), f.title), 500),
  f.time_from, f.time_to, f.created_by_kind, f.created_by_id, f.created_by_name, f.origin, f.origin_client,
  f.agent_request_id, f.created_at, f.created_at, f.created_at
FROM findings f
LEFT JOIN dashboards d ON d.id = f.dashboard_id
LEFT JOIN folders p ON p.personal AND p.owner_id = f.created_by_id AND f.created_by_kind = 'user'
WHERE f.observation_id IS NULL;

UPDATE findings SET observation_id = id WHERE observation_id IS NULL;

INSERT INTO observation_blocks (observation_id, kind, state, position, width, body,
  created_by_kind, created_by_id, created_by_name, origin, origin_client, run_id, created_at, updated_at)
SELECT f.id, 'text', 'live', 0, 'full', jsonb_build_object('markdown', f.markdown),
  f.created_by_kind, f.created_by_id, f.created_by_name, f.origin, f.origin_client, f.agent_request_id, f.created_at, f.created_at
FROM findings f
WHERE f.observation_id = f.id AND NOT EXISTS (SELECT 1 FROM observation_blocks b WHERE b.observation_id = f.id);

INSERT INTO observation_blocks (observation_id, kind, state, position, width, body,
  created_by_kind, created_by_id, created_by_name, origin, origin_client, run_id, created_at, updated_at)
SELECT f.id, 'panel', 'live', e.n, 'half',
  jsonb_build_object('panel', e.panel - 'id' - 'gridPos',
    'from', to_char(f.time_from AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
    'to', to_char(f.time_to AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')),
  f.created_by_kind, f.created_by_id, f.created_by_name, f.origin, f.origin_client, f.agent_request_id, f.created_at, f.created_at
FROM findings f
CROSS JOIN LATERAL jsonb_array_elements(f.evidence) WITH ORDINALITY AS e(panel, n)
WHERE f.observation_id = f.id
  AND NOT EXISTS (SELECT 1 FROM observation_blocks b WHERE b.observation_id = f.id AND b.kind = 'panel');

ALTER TABLE findings ALTER COLUMN observation_id SET NOT NULL;
CREATE INDEX findings_observation_idx ON findings (observation_id);

UPDATE agent_requests r SET observation_id = f.observation_id
FROM findings f
WHERE f.agent_request_id = r.id AND r.observation_id IS NULL;

-- +goose StatementBegin
DO $$
BEGIN
  CREATE TEMPORARY TABLE observation_thread_target ON COMMIT DROP AS
  SELECT o.observation_id,
    coalesce((SELECT t.id FROM threads t WHERE t.observation_id = o.observation_id),
             (SELECT t.id FROM threads t JOIN findings f2 ON f2.id = t.finding_id
              WHERE f2.observation_id = o.observation_id ORDER BY t.created_at, t.id LIMIT 1)) AS target_id,
    EXISTS (SELECT 1 FROM threads t LEFT JOIN findings f3 ON f3.id = t.finding_id
            WHERE (f3.observation_id = o.observation_id OR t.observation_id = o.observation_id) AND t.status = 'open') AS any_open
  FROM (SELECT DISTINCT f.observation_id FROM findings f JOIN threads t ON t.finding_id = f.id) o;

  CREATE TEMPORARY TABLE finding_thread_target ON COMMIT DROP AS
  SELECT f.id AS finding_id, x.observation_id, x.target_id, x.any_open
  FROM findings f JOIN observation_thread_target x ON x.observation_id = f.observation_id;

  UPDATE thread_messages m SET thread_id = x.target_id
  FROM threads t JOIN finding_thread_target x ON t.finding_id = x.finding_id
  WHERE m.thread_id = t.id AND t.id <> x.target_id;

  UPDATE activity a SET target_id = x.target_id
  FROM threads t JOIN finding_thread_target x ON t.finding_id = x.finding_id
  WHERE a.target_kind = 'thread' AND a.target_id = t.id AND t.id <> x.target_id;

  UPDATE agent_requests r SET comment_id = x.target_id
  FROM threads t JOIN finding_thread_target x ON t.finding_id = x.finding_id
  WHERE r.comment_id = t.id AND t.id <> x.target_id;

  DELETE FROM threads t USING finding_thread_target x
  WHERE t.finding_id = x.finding_id AND t.id <> x.target_id;

  UPDATE threads t SET observation_id = x.observation_id, finding_id = NULL, updated_at = clock_timestamp()
  FROM observation_thread_target x
  WHERE t.id = x.target_id;

  UPDATE threads t SET status = 'open', resolved_at = NULL, resolved_by_kind = NULL, resolved_by_id = NULL,
    resolved_by_name = NULL, resolved_origin = NULL, resolved_origin_client = NULL, resolved_run_id = NULL,
    resolved_by_version = NULL
  FROM observation_thread_target x
  WHERE t.id = x.target_id AND x.any_open AND t.status = 'resolved';
END
$$;
-- +goose StatementEnd

UPDATE activity a SET observation_id = f.observation_id, folder_id = o.folder_id
FROM findings f JOIN observations o ON o.id = f.observation_id
WHERE a.finding_id = f.id;

UPDATE findings SET folder_id = NULL WHERE folder_id IS NOT NULL;

-- +goose Down
UPDATE findings f SET folder_id = o.folder_id
FROM observations o
WHERE o.id = f.observation_id AND f.dashboard_id IS NULL;

UPDATE activity a SET folder_id = f.folder_id, observation_id = NULL
FROM findings f
WHERE a.finding_id = f.id;

UPDATE threads t SET finding_id = f.id, observation_id = NULL
FROM findings f
WHERE t.observation_id = f.observation_id AND f.observation_id = f.id;

DROP INDEX findings_observation_idx;
ALTER TABLE findings DROP COLUMN observation_id;

DELETE FROM observations o USING findings f WHERE o.id = f.id;
