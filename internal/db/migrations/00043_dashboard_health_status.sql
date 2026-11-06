-- +goose Up
CREATE VIEW dashboard_health_status AS
SELECT d.id, d.title, d.created_at, d.folder_id,
  f.title AS folder_title, f.general AS folder_general, f.personal AS folder_personal,
  h.version AS checked_version, h.checked_at, coalesce(h.skipped, '')::text AS skipped,
  coalesce(h.issues, '[]'::jsonb)::jsonb AS issues,
  (coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"query_failed"}]'::jsonb)::boolean AS query_failed,
  (coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"no_data"}]'::jsonb)::boolean AS no_data,
  (coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"variable_failed"}]'::jsonb)::boolean AS variable_failed,
  (SELECT max(dv.viewed_at) FROM dashboard_views dv WHERE dv.dashboard_id = d.id)::timestamptz AS last_viewed_at,
  EXISTS (
    SELECT 1 FROM dashboards o
    WHERE o.folder_id = d.folder_id AND o.id <> d.id AND lower(o.title) = lower(d.title)
  ) AS duplicate_title
FROM dashboards d
JOIN folders f ON f.id = d.folder_id
LEFT JOIN dashboard_health h ON h.dashboard_id = d.id;

-- +goose Down
DROP VIEW dashboard_health_status;
