-- +goose Up
CREATE INDEX dashboards_tags_idx ON dashboards USING GIN (tags);

-- +goose Down
DROP INDEX dashboards_tags_idx;
