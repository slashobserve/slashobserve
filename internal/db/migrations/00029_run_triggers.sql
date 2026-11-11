-- +goose Up
ALTER TABLE agent_requests ADD COLUMN trigger TEXT NOT NULL DEFAULT 'person'
  CONSTRAINT agent_requests_trigger_check CHECK (trigger IN ('person', 'mention', 'schedule', 'webhook', 'slack'));

-- +goose Down
ALTER TABLE agent_requests DROP COLUMN trigger;
