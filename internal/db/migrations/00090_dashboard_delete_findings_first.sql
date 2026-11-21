-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION delete_dashboard_findings() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM findings WHERE dashboard_id = OLD.id AND observation_id IS NULL;
  DELETE FROM findings f USING agent_requests r
  WHERE r.dashboard_id = OLD.id AND f.agent_request_id = r.id AND f.observation_id IS NULL;
  RETURN OLD;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER dashboards_delete_findings BEFORE DELETE ON dashboards
FOR EACH ROW EXECUTE FUNCTION delete_dashboard_findings();

-- +goose Down
DROP TRIGGER dashboards_delete_findings ON dashboards;
DROP FUNCTION delete_dashboard_findings();
