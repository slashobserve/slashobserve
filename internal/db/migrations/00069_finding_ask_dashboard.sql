-- +goose Up
ALTER TABLE findings
  ADD COLUMN ask_dashboard_id UUID REFERENCES dashboards (id) ON DELETE SET NULL,
  ADD CONSTRAINT findings_ask_dashboard_check CHECK (ask_dashboard_id IS NULL OR dashboard_id IS NULL);

-- +goose Down
ALTER TABLE findings
  DROP CONSTRAINT findings_ask_dashboard_check,
  DROP COLUMN ask_dashboard_id;
