-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION search_vector(title text, tags text, body text, code text) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
SELECT setweight(to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(tags, '')), 'A') || ''' '':1'::tsvector
    || setweight(to_tsvector('english', coalesce(title, '') || ' ' || coalesce(tags, '')), 'A') || ''' '':1'::tsvector
    || setweight(to_tsvector('simple', coalesce(body, '')), 'B') || ''' '':1'::tsvector
    || setweight(to_tsvector('english', coalesce(body, '')), 'B') || ''' '':1'::tsvector
    || setweight(to_tsvector('simple', coalesce(code, '')), 'C') || ''' '':1'::tsvector
    || setweight(to_tsvector('english', coalesce(code, '')), 'C')
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION search_words(words text[]) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT array_to_string(words, ' ') $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observespec_search_tags(spec jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
SELECT coalesce(string_agg(tag, ' '), '') FROM (
  SELECT jsonb_path_query(spec, 'lax $.tags[*] ? (@.type() == "string")') #>> '{}' AS tag
) found
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observespec_search_text(spec jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
SELECT coalesce(spec ->> 'description', '') || ' ' || coalesce((
  SELECT string_agg(part, ' ') FROM (
    SELECT jsonb_path_query(spec, 'lax $.panels[*].title ? (@.type() == "string")') #>> '{}' AS part
    UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].description ? (@.type() == "string")') #>> '{}'
    UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].title ? (@.type() == "string")') #>> '{}'
    UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].description ? (@.type() == "string")') #>> '{}'
  ) found
), '')
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observespec_search_code(spec jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
SELECT coalesce(string_agg(part, E'\n'), '') FROM (
  SELECT jsonb_path_query(spec, 'lax $.panels[*].queries[*].query ? (@.type() == "string")') #>> '{}' AS part
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].queries[*].query ? (@.type() == "string")') #>> '{}'
) found
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION message_search_text(body text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT regexp_replace(body, '@\[([^\]\n]*)\]\(user:[0-9a-fA-F-]{36}\)', '@\1', 'g') $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION search_query(words text, prefix text) RETURNS tsquery
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
SELECT CASE
  WHEN prefix IS NULL THEN exact.q
  ELSE exact.q || (websearch_to_tsquery('simple', left(words, length(words) - length(prefix))) && to_tsquery('simple', prefix || ':*'))
END
FROM (SELECT websearch_to_tsquery('simple', words) || websearch_to_tsquery('english', words) AS q) exact
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION search_recency(changed_at timestamptz, as_of timestamptz) RETURNS double precision
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT 1 + 0.5 * power(0.5, greatest(0, extract(epoch FROM as_of - changed_at)) / 2592000.0) $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION dashboard_search_vector(spec jsonb) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE COST 1000 SET search_path FROM CURRENT
AS $$
SELECT search_vector(spec ->> 'title', observespec_search_tags(spec), observespec_search_text(spec), observespec_search_code(spec))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION finding_search_vector(title text, markdown text, evidence jsonb) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE COST 1000 SET search_path FROM CURRENT
AS $$
SELECT search_vector(
  title,
  '',
  markdown || ' ' || observespec_search_text(jsonb_build_object('panels', evidence)),
  observespec_search_code(jsonb_build_object('panels', evidence))
)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION annotation_search_vector(body text, tags text[]) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE COST 1000 SET search_path FROM CURRENT
AS $$
SELECT search_vector(split_part(body, E'\n', 1), search_words(tags), body, '')
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION message_search_vector(body text) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE COST 1000 SET search_path FROM CURRENT
AS $$
SELECT search_vector('', '', message_search_text(body), '')
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION saved_search_search_vector(title text, query text, filters jsonb) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE COST 1000 SET search_path FROM CURRENT
AS $$
SELECT search_vector(title, '', coalesce(filters ->> 'service', '') || ' ' || coalesce(filters ->> 'span', ''), query)
$$;
-- +goose StatementEnd

CREATE INDEX dashboard_versions_search_idx ON dashboard_versions USING GIN (dashboard_search_vector(spec));
CREATE INDEX findings_search_idx ON findings USING GIN (finding_search_vector(title, markdown, evidence));
CREATE INDEX annotations_search_idx ON annotations USING GIN (annotation_search_vector(text, tags));
CREATE INDEX thread_messages_search_idx ON thread_messages USING GIN (message_search_vector(body));
CREATE INDEX saved_searches_search_idx ON saved_searches USING GIN (saved_search_search_vector(title, query, filters));

-- +goose Down
DROP INDEX saved_searches_search_idx;
DROP INDEX thread_messages_search_idx;
DROP INDEX annotations_search_idx;
DROP INDEX findings_search_idx;
DROP INDEX dashboard_versions_search_idx;
DROP FUNCTION saved_search_search_vector(text, text, jsonb);
DROP FUNCTION message_search_vector(text);
DROP FUNCTION annotation_search_vector(text, text[]);
DROP FUNCTION finding_search_vector(text, text, jsonb);
DROP FUNCTION dashboard_search_vector(jsonb);
DROP FUNCTION search_recency(timestamptz, timestamptz);
DROP FUNCTION search_query(text, text);
DROP FUNCTION message_search_text(text);
DROP FUNCTION observespec_search_code(jsonb);
DROP FUNCTION observespec_search_text(jsonb);
DROP FUNCTION observespec_search_tags(jsonb);
DROP FUNCTION search_words(text[]);
DROP FUNCTION search_vector(text, text, text, text);
