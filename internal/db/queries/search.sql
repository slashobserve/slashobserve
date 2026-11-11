-- Visibility, ranking and the page are decided before any title, folder or text is joined, so a hidden object's details are never read.
-- Each branch spells its vector exactly as its index expression, or the index is not used.
-- name: SearchWorkspace :many
WITH visible_dashboards AS NOT MATERIALIZED (
  SELECT d.id, d.folder_id, d.latest_version, d.updated_at
  FROM dashboards d
  WHERE d.folder_id = ANY(@folders::uuid[])
),
visible_findings AS NOT MATERIALIZED (
  SELECT f.id, f.created_at,
    CASE WHEN f.dashboard_id IS NOT NULL THEN d.folder_id ELSE f.folder_id END AS folder_id
  FROM findings f
  LEFT JOIN dashboards d ON d.id = f.dashboard_id
  WHERE CASE
    WHEN f.dashboard_id IS NOT NULL THEN d.folder_id = ANY(@folders::uuid[])
    WHEN f.folder_id IS NOT NULL THEN f.folder_id = ANY(@folders::uuid[])
    ELSE @admin::boolean OR (f.created_by_kind = @reader_kind::text AND f.created_by_id = @reader_id::uuid)
  END
),
visible_annotations AS NOT MATERIALIZED (
  SELECT a.id, a.created_at, coalesce(d.folder_id, general_folder_id()) AS folder_id
  FROM annotations a
  LEFT JOIN dashboards d ON d.id = a.dashboard_id
  WHERE (a.dashboard_id IS NULL OR d.id IS NOT NULL)
    AND coalesce(d.folder_id, general_folder_id()) = ANY(@folders::uuid[])
),
hits AS (
  (SELECT 'dashboard'::text AS kind, v.dashboard_id AS id, d.updated_at AS changed_at, d.folder_id AS folder_id,
      'dashboard'::text AS subject_kind, v.dashboard_id AS subject_id,
      ts_rank_cd(dashboard_search_vector(v.spec), search_query(@words::text, sqlc.narg('prefix')::text), 1)
        * search_recency(d.updated_at, @as_of::timestamptz) AS score
    FROM dashboard_versions v
    JOIN visible_dashboards d ON d.id = v.dashboard_id AND v.version = d.latest_version
    WHERE 'dashboard' = ANY(@kinds::text[])
      AND dashboard_search_vector(v.spec) @@ search_query(@words::text, sqlc.narg('prefix')::text)
    ORDER BY score DESC, changed_at DESC, id
    LIMIT @row_window::integer)
  UNION ALL
  (SELECT 'finding'::text AS kind, f.id AS id, f.created_at AS changed_at, vf.folder_id AS folder_id, 'finding'::text AS subject_kind, f.id AS subject_id,
      ts_rank_cd(finding_search_vector(f.title, f.markdown, f.evidence), search_query(@words::text, sqlc.narg('prefix')::text), 1)
        * search_recency(f.created_at, @as_of::timestamptz) AS score
    FROM findings f
    JOIN visible_findings vf ON vf.id = f.id
    WHERE 'finding' = ANY(@kinds::text[])
      AND finding_search_vector(f.title, f.markdown, f.evidence) @@ search_query(@words::text, sqlc.narg('prefix')::text)
    ORDER BY score DESC, changed_at DESC, id
    LIMIT @row_window::integer)
  UNION ALL
  (SELECT 'annotation'::text AS kind, a.id AS id, a.created_at AS changed_at, va.folder_id AS folder_id, 'annotation'::text AS subject_kind, a.id AS subject_id,
      ts_rank_cd(annotation_search_vector(a.text, a.tags), search_query(@words::text, sqlc.narg('prefix')::text), 1)
        * search_recency(a.created_at, @as_of::timestamptz) AS score
    FROM annotations a
    JOIN visible_annotations va ON va.id = a.id
    WHERE 'annotation' = ANY(@kinds::text[])
      AND annotation_search_vector(a.text, a.tags) @@ search_query(@words::text, sqlc.narg('prefix')::text)
    ORDER BY score DESC, changed_at DESC, id
    LIMIT @row_window::integer)
  UNION ALL
  (SELECT 'comment'::text AS kind, m.id AS id, coalesce(m.edited_at, m.created_at) AS changed_at,
      coalesce(vd.folder_id, vf.folder_id, va.folder_id) AS folder_id,
      CASE WHEN t.dashboard_id IS NOT NULL THEN 'dashboard' WHEN t.finding_id IS NOT NULL THEN 'finding' ELSE 'annotation' END AS subject_kind,
      coalesce(t.dashboard_id, t.finding_id, t.annotation_id) AS subject_id,
      ts_rank_cd(message_search_vector(m.body), search_query(@words::text, sqlc.narg('prefix')::text), 1)
        * search_recency(coalesce(m.edited_at, m.created_at), @as_of::timestamptz) AS score
    FROM thread_messages m
    JOIN threads t ON t.id = m.thread_id
    LEFT JOIN visible_dashboards vd ON vd.id = t.dashboard_id
    LEFT JOIN visible_findings vf ON vf.id = t.finding_id
    LEFT JOIN visible_annotations va ON va.id = t.annotation_id
    WHERE 'comment' = ANY(@kinds::text[])
      AND message_search_vector(m.body) @@ search_query(@words::text, sqlc.narg('prefix')::text)
      AND (vd.id IS NOT NULL OR vf.id IS NOT NULL OR va.id IS NOT NULL)
    ORDER BY score DESC, changed_at DESC, id
    LIMIT @row_window::integer)
  UNION ALL
  (SELECT 'saved_search'::text AS kind, s.id AS id, s.updated_at AS changed_at, s.folder_id AS folder_id, 'saved_search'::text AS subject_kind, s.id AS subject_id,
      ts_rank_cd(saved_search_search_vector(s.title, s.query, s.filters), search_query(@words::text, sqlc.narg('prefix')::text), 1)
        * search_recency(s.updated_at, @as_of::timestamptz) AS score
    FROM saved_searches s
    WHERE 'saved_search' = ANY(@kinds::text[])
      AND s.folder_id = ANY(@folders::uuid[])
      AND saved_search_search_vector(s.title, s.query, s.filters) @@ search_query(@words::text, sqlc.narg('prefix')::text)
    ORDER BY score DESC, changed_at DESC, id
    LIMIT @row_window::integer)
),
page AS (
  SELECT h.kind, h.id, h.changed_at, h.folder_id, h.subject_kind, h.subject_id, h.score
  FROM hits h
  ORDER BY h.score DESC, h.changed_at DESC, h.kind, h.id
  OFFSET @row_offset::integer
  LIMIT @row_limit::integer
)
SELECT p.kind, p.id, p.score::float8 AS score, p.changed_at, p.folder_id, fo.title AS folder_title,
  p.subject_kind, p.subject_id,
  CASE p.subject_kind
    WHEN 'dashboard' THEN sd.title
    WHEN 'finding' THEN sf.title
    WHEN 'annotation' THEN split_part(sa.text, E'\n', 1)
    ELSE ss.title
  END::text AS title,
  ts_headline('english',
    translate(CASE p.kind
      WHEN 'dashboard' THEN observespec_search_text(sv.spec)
      WHEN 'finding' THEN sf.markdown
      WHEN 'annotation' THEN sa.text
      WHEN 'comment' THEN message_search_text(m.body)
      ELSE coalesce(ss.query, '')
    END, chr(2) || chr(3), ''),
    search_query(@words::text, sqlc.narg('prefix')::text),
    'StartSel=' || chr(2) || ', StopSel=' || chr(3) || ', MaxWords=24, MinWords=8, MaxFragments=2, FragmentDelimiter=" ... "')::text AS headline,
  m.thread_id AS thread_id,
  ad.id AS annotation_dashboard_id,
  ad.title AS annotation_dashboard_title,
  sa.time_from AS annotation_time_from,
  sa.time_to AS annotation_time_to,
  ss.kind AS saved_kind,
  ss.datasource_uid AS saved_datasource_uid,
  ss.query AS saved_query,
  ss.filters AS saved_filters,
  ss.time_from AS saved_time_from,
  ss.time_to AS saved_time_to
FROM page p
LEFT JOIN folders fo ON fo.id = p.folder_id
LEFT JOIN dashboards sd ON p.subject_kind = 'dashboard' AND sd.id = p.subject_id
LEFT JOIN dashboard_versions sv ON p.kind = 'dashboard' AND sv.dashboard_id = sd.id AND sv.version = sd.latest_version
LEFT JOIN findings sf ON p.subject_kind = 'finding' AND sf.id = p.subject_id
LEFT JOIN annotations sa ON p.subject_kind = 'annotation' AND sa.id = p.subject_id
LEFT JOIN dashboards ad ON ad.id = sa.dashboard_id
LEFT JOIN saved_searches ss ON p.subject_kind = 'saved_search' AND ss.id = p.subject_id
LEFT JOIN thread_messages m ON p.kind = 'comment' AND m.id = p.id
ORDER BY p.score DESC, p.changed_at DESC, p.kind, p.id;
