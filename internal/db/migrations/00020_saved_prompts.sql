-- +goose Up
CREATE TABLE saved_prompts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
  body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000),
  shared BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX saved_prompts_owner_name_key ON saved_prompts (owner_id, lower(name));
CREATE UNIQUE INDEX saved_prompts_shared_name_key ON saved_prompts (lower(name)) WHERE shared;
CREATE INDEX saved_prompts_shared_idx ON saved_prompts (shared) WHERE shared;

-- +goose Down
DROP TABLE saved_prompts;
