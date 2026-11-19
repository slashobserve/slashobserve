-- +goose Up
ALTER TABLE proposals
  ADD COLUMN close_reason TEXT CONSTRAINT proposals_close_reason_check CHECK (close_reason IS NULL OR (state = 'discarded' AND char_length(close_reason) BETWEEN 1 AND 500));

-- +goose Down
ALTER TABLE proposals DROP COLUMN close_reason;
