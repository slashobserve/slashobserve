-- +goose Up
CREATE TABLE audit_export_cursor (
  only_row BOOLEAN PRIMARY KEY DEFAULT true CHECK (only_row),
  xid XID8 NOT NULL,
  seq BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE audit_export_cursor;
