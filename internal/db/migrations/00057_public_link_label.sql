-- +goose Up
ALTER TABLE public_links ADD COLUMN label TEXT NOT NULL DEFAULT '' CHECK (char_length(label) <= 80);

-- +goose Down
ALTER TABLE public_links DROP COLUMN label;
