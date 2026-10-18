-- +goose Up
DROP INDEX users_last_seen_idx;
CREATE INDEX users_created_idx ON users (created_at DESC, id DESC);

-- +goose Down
DROP INDEX users_created_idx;
CREATE INDEX users_last_seen_idx ON users (last_seen_at DESC, id DESC);
