-- name: StarterContent :one
SELECT ref FROM starter_content WHERE key = @key;

-- name: RecordStarterContent :exec
INSERT INTO starter_content (key, ref) VALUES (@key, @ref) ON CONFLICT (key) DO NOTHING;

-- name: ReplaceStarterContent :exec
INSERT INTO starter_content (key, ref) VALUES (@key, @ref)
ON CONFLICT (key) DO UPDATE SET ref = EXCLUDED.ref, seeded_at = now();

-- name: LatestVersionIsNotBy :one
SELECT EXISTS (
  SELECT 1 FROM dashboard_versions v JOIN dashboards d ON d.id = v.dashboard_id AND d.latest_version = v.version
  WHERE v.dashboard_id = @dashboard_id AND NOT (v.author_kind = @author_kind AND v.author_id = @author_id)
)::boolean;
