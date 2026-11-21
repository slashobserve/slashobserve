-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION delete_dashboard_findings() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM findings
  WHERE dashboard_id = OLD.id
    OR (observation_id IS NULL AND agent_request_id IN (SELECT id FROM agent_requests WHERE dashboard_id = OLD.id));
  RETURN OLD;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER dashboards_delete_findings BEFORE DELETE ON dashboards
FOR EACH ROW EXECUTE FUNCTION delete_dashboard_findings();

-- +goose Down
DROP TRIGGER dashboards_delete_findings ON dashboards;
DROP FUNCTION delete_dashboard_findings();
