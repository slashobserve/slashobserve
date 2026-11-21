-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE observation_blocks ADD COLUMN IF NOT EXISTS height INTEGER;

ALTER TABLE observation_blocks
  DROP CONSTRAINT IF EXISTS observation_blocks_width_check,
  DROP CONSTRAINT IF EXISTS observation_blocks_height_check,
  ADD CONSTRAINT observation_blocks_width_check CHECK (width IN ('third', 'half', 'two_thirds', 'full')) NOT VALID,
  ADD CONSTRAINT observation_blocks_height_check CHECK (height IS NULL OR height BETWEEN 120 AND 960) NOT VALID;
ALTER TABLE observation_blocks VALIDATE CONSTRAINT observation_blocks_width_check;
ALTER TABLE observation_blocks VALIDATE CONSTRAINT observation_blocks_height_check;

-- +goose Down
ALTER TABLE observation_blocks
  DROP CONSTRAINT IF EXISTS observation_blocks_height_check,
  DROP COLUMN IF EXISTS height;
UPDATE observation_blocks SET width = 'half' WHERE width IN ('third', 'two_thirds');
ALTER TABLE observation_blocks
  DROP CONSTRAINT IF EXISTS observation_blocks_width_check,
  ADD CONSTRAINT observation_blocks_width_check CHECK (width IN ('full', 'half'));
