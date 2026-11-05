-- +goose Up
ALTER TABLE imports ADD COLUMN grafana_uid TEXT NOT NULL DEFAULT '';
UPDATE imports i
SET grafana_uid = first.uid
FROM (
  SELECT DISTINCT ON (summary ->> 'uid') id, summary ->> 'uid' AS uid
  FROM imports
  WHERE summary ->> 'uid' ~ '^[A-Za-z0-9_-]{1,40}$'
  ORDER BY summary ->> 'uid', created_at, id
) first
WHERE i.id = first.id;
CREATE UNIQUE INDEX imports_grafana_uid_key ON imports (grafana_uid) WHERE grafana_uid <> '';

-- +goose Down
DROP INDEX imports_grafana_uid_key;
ALTER TABLE imports DROP COLUMN grafana_uid;
