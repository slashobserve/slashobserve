-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION health_problem_matches(kind text, query_failed boolean, no_data boolean, variable_failed boolean,
  duplicate_title boolean, last_viewed_at timestamptz, created_at timestamptz, unopened_before timestamptz) RETURNS boolean
LANGUAGE sql IMMUTABLE
AS $$
  SELECT CASE kind
    WHEN 'any' THEN query_failed OR no_data OR variable_failed OR duplicate_title
      OR coalesce(last_viewed_at, created_at) < unopened_before
    WHEN 'query_failed' THEN query_failed
    WHEN 'no_data' THEN no_data
    WHEN 'variable_failed' THEN variable_failed
    WHEN 'duplicate_title' THEN duplicate_title
    WHEN 'unopened' THEN coalesce(last_viewed_at, created_at) < unopened_before
    ELSE false END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION health_problem_matches(text, boolean, boolean, boolean, boolean, timestamptz, timestamptz, timestamptz);
