-- +goose Up
ALTER TABLE users
  ADD COLUMN deactivated_at TIMESTAMPTZ,
  ADD COLUMN deactivated_by_id UUID REFERENCES users (id) ON DELETE SET NULL,
  ADD COLUMN deactivated_by_name TEXT,
  ADD COLUMN first_user BOOLEAN NOT NULL DEFAULT false;
UPDATE users SET first_user = true WHERE id = (SELECT id FROM users ORDER BY created_at, id LIMIT 1);
CREATE UNIQUE INDEX users_first_user_idx ON users (first_user) WHERE first_user;

CREATE TABLE user_invites (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL CHECK (octet_length(email) BETWEEN 3 AND 254),
  role TEXT NOT NULL CHECK (role IN ('admin', 'editor', 'commenter', 'viewer')),
  created_by_kind TEXT NOT NULL CHECK (created_by_kind IN ('user', 'service_account')),
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX user_invites_email_idx ON user_invites (lower(email));
CREATE INDEX user_invites_created_idx ON user_invites (created_at DESC, id DESC);

-- +goose Down
DROP TABLE user_invites;
DROP INDEX users_first_user_idx;
ALTER TABLE users DROP COLUMN first_user, DROP COLUMN deactivated_by_name, DROP COLUMN deactivated_by_id, DROP COLUMN deactivated_at;
