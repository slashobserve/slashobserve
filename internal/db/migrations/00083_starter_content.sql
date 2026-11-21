-- +goose Up
CREATE TABLE starter_content (
  key TEXT PRIMARY KEY,
  ref UUID NOT NULL,
  seeded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE starter_content;
