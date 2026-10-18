-- +goose Up
ALTER TABLE users ADD COLUMN last_seen_at TIMESTAMPTZ;
UPDATE users SET last_seen_at = last_login_at;
ALTER TABLE users ALTER COLUMN last_seen_at SET NOT NULL;
ALTER TABLE users ALTER COLUMN last_seen_at SET DEFAULT now();
CREATE INDEX users_last_seen_idx ON users (last_seen_at DESC, id DESC);

-- +goose Down
DROP INDEX users_last_seen_idx;
ALTER TABLE users DROP COLUMN last_seen_at;
