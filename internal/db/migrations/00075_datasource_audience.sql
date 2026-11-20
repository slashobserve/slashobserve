-- +goose Up
ALTER TABLE datasources ADD COLUMN audience TEXT NOT NULL DEFAULT 'everyone' CHECK (audience IN ('everyone', 'admins'));

INSERT INTO audit_activity_actions (type, action, category, keys, extra) VALUES
  ('datasource.audience_changed', 'datasource.audience_change', 'admin', '{uid,type,name,host,from,to}', '{}');

-- +goose Down
DELETE FROM audit_activity_actions WHERE type = 'datasource.audience_changed';

ALTER TABLE datasources DROP COLUMN audience;
