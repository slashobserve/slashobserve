-- +goose Up
CREATE TABLE public_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dashboard_id UUID NOT NULL REFERENCES dashboards (id) ON DELETE CASCADE,
  token_hash BYTEA NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  display_prefix TEXT NOT NULL,
  variables TEXT NOT NULL CHECK (variables IN ('frozen', 'free')),
  selections JSONB NOT NULL DEFAULT '{}',
  expires_at TIMESTAMPTZ NULL,
  created_by_kind TEXT NOT NULL,
  created_by_id UUID NOT NULL,
  created_by_name TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_viewed_at TIMESTAMPTZ NULL
);
CREATE INDEX public_links_dashboard_id_idx ON public_links (dashboard_id);

-- +goose Down
DROP TABLE public_links;
