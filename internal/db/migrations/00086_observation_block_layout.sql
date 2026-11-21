-- +goose Up
ALTER TABLE observation_blocks
  DROP CONSTRAINT observation_blocks_width_check,
  ADD CONSTRAINT observation_blocks_width_check CHECK (width IN ('third', 'half', 'two_thirds', 'full')),
  ADD COLUMN height INTEGER CONSTRAINT observation_blocks_height_check CHECK (height IS NULL OR height BETWEEN 120 AND 960);

-- +goose Down
ALTER TABLE observation_blocks DROP COLUMN height;
UPDATE observation_blocks SET width = 'half' WHERE width IN ('third', 'two_thirds');
ALTER TABLE observation_blocks
  DROP CONSTRAINT observation_blocks_width_check,
  ADD CONSTRAINT observation_blocks_width_check CHECK (width IN ('full', 'half'));
