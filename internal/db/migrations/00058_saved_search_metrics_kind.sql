-- +goose Up
ALTER TABLE saved_searches DROP CONSTRAINT saved_searches_kind_check;
ALTER TABLE saved_searches ADD CONSTRAINT saved_searches_kind_check CHECK (kind IN ('logs', 'traces', 'metrics'));
ALTER TABLE saved_searches ADD CONSTRAINT saved_searches_metrics_query_check CHECK (kind <> 'metrics' OR query IS NOT NULL);

-- +goose Down
DELETE FROM saved_searches WHERE kind = 'metrics';
ALTER TABLE saved_searches DROP CONSTRAINT saved_searches_metrics_query_check;
ALTER TABLE saved_searches DROP CONSTRAINT saved_searches_kind_check;
ALTER TABLE saved_searches ADD CONSTRAINT saved_searches_kind_check CHECK (kind IN ('logs', 'traces'));
