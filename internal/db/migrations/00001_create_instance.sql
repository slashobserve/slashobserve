-- +goose Up
CREATE TABLE instance (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO instance DEFAULT VALUES;

-- +goose Down
DROP TABLE instance;
